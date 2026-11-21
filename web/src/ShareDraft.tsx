import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, projectHref, type ApiRequest } from './shared'
import { MaterialReader } from './MaterialReader'
import { ThemedSelect } from './ThemedSelect'
import { KnowledgeComparison } from './KnowledgeBrowser'

export type SharedCopy = {id:number; series_id:number; share_version:number; title:string; content:string; digest:string; published_at?:string}
export type ShareRevision = SharedCopy & {
  project_id:number; recipient:number; material_key?:string; revision?:number; experience_id?:number; experience_revision?:number
  proposed_at:string; current_share_id?:number; base_share_id?:number; publication_epoch:number; revoked_at?:string; superseded_at?:string; stale?:boolean
}

export function ShareDraft({project,material,experience,revision,request}:{
  project:number; material?:string; experience?:number; revision:number; request:ApiRequest
}) {
  const [open,setOpen]=useState(false)
  return open ? <ShareEditor project={project} material={material} experience={experience} revision={revision} request={request} onCancel={()=>setOpen(false)}/> :
    <button className="button" type="button" onClick={()=>setOpen(true)}>起草脱敏副本</button>
}

export function ShareEditor({project,material,experience,revision,row,current,request,onCancel,onConflict,onSaved}:{
  project:number;material?:string;experience?:number;revision?:number;row?:ShareRevision;current?:SharedCopy;request:ApiRequest
  onCancel:()=>void;onConflict?:()=>void;onSaved?:(doc:ShareRevision)=>void
}) {
  const [title,setTitle]=useState(row?.title??''),[content,setContent]=useState(row?.content??'')
  const [sourceKind,setSourceKind]=useState(experience||row?.experience_id?'experience':'material')
  const [source,setSource]=useState(String(material??experience??(row?.experience_id||row?.material_key)??''))
  const [sourceRevision,setSourceRevision]=useState(String(revision??(row?.experience_id?row.experience_revision:row?.revision)??1))
  const [recipient,setRecipient]=useState(row?.recipient.toString()??'')
  const [expected,setExpected]=useState({id:row?.current_share_id??0,epoch:row?.publication_epoch??0})
  const [busy,setBusy]=useState(false),[error,setError]=useState('')
  const lifetime=useRef<AbortController | null>(null)
  useEffect(()=>{const c=new AbortController();lifetime.current=c;return()=>c.abort()},[])
  const stale=row && (expected.id!==(row.current_share_id??0)||expected.epoch!==row.publication_epoch)
  const fixed=Boolean(material||experience||row)
  async function submit(e:FormEvent) {
    e.preventDefault()
    if (busy||stale) return
    const target=Number(recipient),version=Number(sourceRevision),key=source.trim()
    if (!Number.isSafeInteger(target)||target===project||target<1||!Number.isSafeInteger(version)||version<1||!key||
      (sourceKind==='experience' && (!Number.isSafeInteger(Number(key))||Number(key)<1))) {
      setError('请核对来源、修订及接收项目编号。');return
    }
    if (!title.trim()||!content.trim()||new TextEncoder().encode(title.trim()).length>256||new TextEncoder().encode(content).length>8192) {
      setError('请填写脱敏标题和正文，标题最多 256 字节，正文最多 8192 字节。');return
    }
    const fields=sourceKind==='experience'?{experience_id:Number(key),experience_revision:version}:{material_key:key,revision:version}
    const body=row?{expected_current_id:expected.id,expected_epoch:expected.epoch,source_revision:version,title:title.trim(),content}:
      {...fields,recipient:target,title:title.trim(),content}
    const signal=lifetime.current?.signal
    setError('');setBusy(true)
    try {
      const saved=await request<ShareRevision>('/projects/'+project+'/shares'+(row?'/'+row.id+'/versions':''),
        {method:'POST',body:JSON.stringify(body),signal})
      if (signal?.aborted) return
      if (onSaved) onSaved(saved)
      else location.hash=projectHref(project,'/knowledge/shares/sent/'+saved.id)
    } catch(err) {
      if (signal?.aborted) return
      if (err instanceof ApiError && err.status===409) {
        setError('提交发生冲突，草稿已保留。请核对来源修订、当前发布版本及正文脱敏情况。');onConflict?.()
      } else setError(err instanceof Error?err.message:'无法提交副本。')
    } finally {if (!signal?.aborted) setBusy(false)}
  }
  return <form className="knowledge-form" onSubmit={submit}><h3>{row?'起草共享新版':'起草脱敏副本'}</h3>
    <fieldset disabled={busy}>
      {fixed?<p>来源 · {sourceKind==='experience'?'项目经验 #':'ForgeOS 资料 '}{source}{!row&&' · 修订 '+sourceRevision}</p>:
        <><ThemedSelect label="共享来源" value={sourceKind} onChange={v=>{setSourceKind(v);setSource('')}} options={[{value:'material',label:'ForgeOS 资料'},{value:'experience',label:'已审核项目经验'}]}/>
          <label>{sourceKind==='experience'?'经验编号':'资料标识'}<input required value={source} onChange={e=>setSource(e.target.value)}/></label></>}
      {(!fixed||row) && <label>来源修订<input type="number" min={1} step={1} required value={sourceRevision} onChange={e=>setSourceRevision(e.target.value)}/></label>}
      {row?<p>接收项目 #{row.recipient} · {row.current_share_id?'当前发布副本 #'+row.current_share_id:'尚无当前发布版'}</p>:
        <label>接收项目编号<input type="number" min={1} step={1} required value={recipient} onChange={e=>setRecipient(e.target.value)}/></label>}
      <label>脱敏标题<input required value={title} onChange={e=>setTitle(e.target.value)}/></label>
      <label>脱敏正文（Markdown）<textarea required rows={6} value={content} onChange={e=>setContent(e.target.value)}/></label>
      {content && <details><summary>预览拟发布正文</summary><MaterialReader markdown content={content}/></details>}
      {(current || row) && <KnowledgeComparison summary="比较脱敏副本变更" from={(current?'当前发布版':'起草依据')+' · v'+(current||row)?.share_version} to="拟发布版本"
        previous={[(current||row)?.title,(current||row)?.content].join('\n\n')} proposed={[title,content].join('\n\n')}/>}
      {error && <p role="alert">{error}</p>}
      {stale && <button type="button" className="button" onClick={()=>{setExpected({id:row.current_share_id??0,epoch:row.publication_epoch});setError('')}}>已核对，基于当前发布状态提交</button>}
      <div className="knowledge-actions"><button disabled={Boolean(stale)} className="button primary">提交 Management 审核</button>
        <button className="button" type="button" onClick={onCancel}>取消起草</button></div>
    </fieldset></form>
}
