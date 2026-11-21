import { useEffect, useRef, useState, type FormEvent } from 'react'
import { KnowledgeBrowser, KnowledgeComparison, KnowledgeSearch, LibraryNotice, useKnowledgeDetail, useKnowledgeList } from './KnowledgeBrowser'
import { MaterialReader } from './MaterialReader'
import { KnowledgeCuration } from './KnowledgeCuration'
import { ShareDraft } from './ShareDraft'
import { ApiError, formatTime, projectHref, type ApiRequest } from './shared'
import { PageHeading } from './ui'
import { ThemedSelect } from './ThemedSelect'

const states = {current:'当前有效', pending:'待处理', withdrawn:'已撤回', stale:'来源已失效', rejected:'未采纳'}
const changes = {pending:'审核中', ready:'待选择审核事实', rejected:'已退回', applied:'已采纳', obsolete:'已有其他修订', discarded:'已放弃'}
const filters = [{value:'current',label:'当前有效'},{value:'pending',label:'待处理'},{value:'history',label:'历史'},{value:'all',label:'全部'}]
type Experience = {
  id:number; current_revision:number; status:keyof typeof states; title:string; statement?:string; quote?:string; source?:string
  validity:string; reviewed_at?:string; updated_at:string; has_pending:boolean
}
type Revision = {revision:number; title:string; statement:string; quote:string; source:string; validity:string; receipt_path:string; actor:string; change_reason:string; created_at:string; is_current:boolean}
type Change = {id:number; expected_revision:number; title:string; source:string; content:string; reason:string; status:keyof typeof changes
  validity:string; facts:{statement:string;quote:string}[]; review?:{rationale?:string}; created_at:string; receipt_path?:string}
type Entry = Experience & {revisions:Revision[]; changes:Change[]}
type EditorState = {entryID?:number; revision:number; title:string; validity:string; source:string; content:string}

function comparisonText(title:string, source:string, validity:string, content:string) {
  return [title,'来源 · '+source,'适用场景 · '+(validity||'未填写'),'',content].join('\n')
}

export function ProjectExperience({project,entry,request}:{project:number; entry?:number; request:ApiRequest}) {
  const api = '/projects/'+project+'/knowledge/experience'
  const href = projectHref(project,'/knowledge/experience')
  const [filter,setFilter] = useState('current'), [text,setText] = useState(''), [query,setQuery] = useState('')
  const [revision,setRevision] = useState(0)
  const [editor,setEditor] = useState<EditorState>()
  const [notice,setNotice] = useState('')
  const params = new URLSearchParams({status:filter})
  if (query) params.set('q',query)
  const list = useKnowledgeList<Experience>(request,api+'?'+params,revision)
  const selected = entry ?? list.rows[0]?.id
  const detail = useKnowledgeDetail<Entry>(request,selected ? api+'/'+selected : undefined,revision)
  const refresh = ()=>setRevision(r=>r+1)
  const leave = ()=>{setEditor(undefined); setNotice(''); location.hash=href}
  const editing = editor && (!editor.entryID || editor.entryID === selected)
  function start(item?:Entry) {
    setNotice('')
    location.hash=item?href+'/'+item.id:href
    const prev = item?.revisions.find(r=>r.revision===item.current_revision)
    const draft = item?.changes[0]
    setEditor({entryID:item?.id,revision:item?.current_revision ?? 0,title:item?.title ?? '',source:prev?.source ?? draft?.source ?? '',validity:item?.validity??'',
      content:prev?.statement ?? draft?.content ?? ''})
  }
  function saved(doc:Entry) {
    setEditor(undefined); setNotice('已提交审核。审核通过后，可在此选择并采纳事实。')
    location.hash=href+'/'+doc.id
    refresh()
  }
  return <div className="project-setup">
    <PageHeading title="项目经验" actions={<button className="button primary" onClick={()=>start()}>记录经验</button>}/>
    <KnowledgeBrowser detailActive={Boolean(entry || editing)} back={leave}
      toolbar={<KnowledgeSearch query={text} setQuery={setText} placeholder="搜索经验、来源或引句" refresh={refresh}
        search={q=>{setQuery(q);leave()}} filter={<ThemedSelect label="经验状态" hideLabel value={filter} options={filters}
        onChange={f=>{setFilter(f);leave()}}/>}/>}
      catalog={<>{list.rows.map(row=><a key={row.id} className="knowledge-row" href={href+'/'+row.id}
        aria-current={selected===row.id && !editing ? 'true' : undefined} onClick={()=>{setEditor(undefined);setNotice('')}}>
        <strong className="knowledge-row-title">{row.statement||row.title}</strong>
        {row.statement && row.statement!==row.title && <small className="knowledge-meta">{row.title}</small>}
        <small className="knowledge-row-meta">#{row.id} · {states[row.status]} {row.has_pending && row.status!=='pending' && ' · 有待处理更正'}</small>
        <small className="knowledge-meta">{formatTime(row.updated_at)}</small>
      </a>)}<LibraryNotice {...list} retry={refresh} empty={list.rows.length?'':query?'没有匹配的项目经验。':'当前筛选下暂无项目经验。'}/></>}>
      {notice && <p role="status">{notice}</p>}
      {editing && editor ? <ExperienceEditor key={String(editor.entryID ?? 'new')} initial={editor} current={detail.data}
        base={api} request={request} onConflict={refresh} onCancel={()=>setEditor(undefined)} onSaved={saved}/> :
        detail.data ? <ExperienceDetails key={detail.data.id} doc={detail.data} project={project} request={request}
          edit={()=>start(detail.data)} refresh={()=>{setNotice('');refresh()}}/> :
        <LibraryNotice error={detail.error} loading={detail.loading} empty="从列表选择一条经验，查看原文和修订。" retry={refresh}/>}
    </KnowledgeBrowser>
  </div>
}

function ExperienceEditor({initial,current,base,request,onConflict,onCancel,onSaved}:{
  initial:EditorState; current?:Entry; base:string; request:ApiRequest; onConflict:()=>void; onCancel:()=>void; onSaved:(doc:Entry)=>void
}) {
  const [title,setTitle] = useState(initial.title), [source,setSource] = useState(initial.source)
  const [content,setContent] = useState(initial.content), [reason,setReason] = useState('')
  const [validity,setValidity] = useState(initial.validity)
  const [expected,setExpected] = useState(initial.revision)
  const [busy,setBusy] = useState(false), [error,setError] = useState('')
  const retry = useRef<{body:string;key:string} | null>(null)
  const lifetime=useRef<AbortController | null>(null)
  useEffect(()=>{const controller=new AbortController();lifetime.current=controller;return()=>controller.abort()},[])
  const changed = Boolean(initial.entryID && current?.id===initial.entryID && current.current_revision!==expected)
  const unavailable=Boolean(initial.entryID && current?.status==='withdrawn')
  async function save(e:FormEvent) {
    e.preventDefault()
    if (busy || changed || unavailable) return
    const payload={expected_revision:expected,title:title.trim(),source:source.trim(),content,reason:reason.trim(),validity:validity.trim()}
    const bytes=new TextEncoder()
    for (const [name,value,limit] of [['标题',payload.title,256],['依据来源',payload.source,2048],['适用场景',payload.validity,1024],['正文',content,256*1024],['更正原因',payload.reason,2048]] as const) {
      if (bytes.encode(value).length>limit) {setError(name+'最多允许 '+limit+' 字节。');return}
    }
    const body=JSON.stringify(payload)
    if (retry.current?.body !== body) retry.current={body,key:crypto.randomUUID()}
    const signal=lifetime.current?.signal
    setBusy(true);setError('')
    try {
      const item=await request<Entry>(initial.entryID ? base+'/'+initial.entryID+'/changes' : base,
        {method:'POST',body:JSON.stringify({...payload,request_key:retry.current.key}),signal})
      if (signal?.aborted) return
      onSaved(item)
    } catch(err) {
      if (signal?.aborted) return
      if (err instanceof ApiError && err.status===409) {setError('当前状态已变化，已保留草稿。请核对最新修订及待处理请求。');onConflict()}
      else setError(err instanceof Error?err.message:'提交失败。')
    } finally {if (!signal?.aborted) setBusy(false)}
  }
  return <form className="knowledge-form" onSubmit={save}>
    <h2>{initial.entryID?'更正本条经验':'记录项目经验'}</h2>
    <fieldset disabled={busy}>
      <label>标题<input required value={title} onChange={e=>setTitle(e.target.value)}/></label>
      <label>依据来源<input required value={source} onChange={e=>setSource(e.target.value)}/></label>
      <label>适用场景（选填）<input value={validity} onChange={e=>setValidity(e.target.value)}/></label>
      <label>经验与原始依据（Markdown）<textarea required rows={7} value={content} onChange={e=>setContent(e.target.value)}/></label>
      {content && <details><summary>预览拟议正文</summary><MaterialReader markdown content={content}/></details>}
      {initial.entryID && <><KnowledgeComparison summary="比较当前版与拟议版" from="当前版本" to="拟议版本"
          previous={comparisonText(current?.title??initial.title,current?.source??initial.source,current?.validity??initial.validity,current?.statement??initial.content)}
          proposed={comparisonText(title,source,validity,content)}/>
        <label>更正原因<input required value={reason} onChange={e=>setReason(e.target.value)}/></label></>}
      {error && <p role="alert">{error}</p>}
      {changed && current && !unavailable && <button className="button" type="button" onClick={()=>{setExpected(current.current_revision);setError('')}}>已核对，基于修订 {current.current_revision} 提交</button>}
      {unavailable && <p>这条经验已撤回。可复制草稿另行记录。</p>}
      <div className="knowledge-actions"><button className="button primary" disabled={unavailable || changed || !title.trim() || !source.trim() || !content.trim()}>提交审核</button>
        <button className="button" type="button" onClick={onCancel}>取消</button></div>
    </fieldset>
  </form>
}

function ExperienceDetails({doc,project,request,edit,refresh}:{doc:Entry;project:number;request:ApiRequest;edit:()=>void;refresh:()=>void}) {
  const [busy,setBusy]=useState(false),[message,setMessage]=useState(''),[error,setError]=useState('')
  const [confirm,setConfirm]=useState(false)
  const active=doc.revisions.find(r=>r.revision===doc.current_revision)
  const path='/projects/'+project+'/knowledge/experience/'+doc.id
  async function update(suffix:string,body:object,ok:string) {
    setBusy(true);setError('');setMessage('')
    try {await request<Entry>(path+suffix,{method:'POST',body:JSON.stringify(body)});setMessage(ok);setConfirm(false);refresh()}
    catch(err){setError(err instanceof ApiError && err.status===409?'条目已变化，请核对最新状态。':err instanceof Error?err.message:'提交失败。');refresh()}
    finally {setBusy(false)}
  }
  return <article>
    <header><p className="knowledge-meta">项目经验 #{doc.id} · {states[doc.status]}{doc.current_revision>0 && ' · 修订 '+doc.current_revision}</p><h2>{doc.title}</h2></header>
    {active && <><MaterialReader markdown content={active.statement}/><div><strong>原文引句</strong><blockquote>{active.quote}</blockquote><p>来源 · {active.source}</p></div>
      {active.validity && <p>适用场景 · {active.validity}</p>}{doc.reviewed_at && <p>审核时间 · {formatTime(doc.reviewed_at)}</p>}
      {active.receipt_path && <KnowledgeCuration project={project} path={active.receipt_path} request={request}/>}</>}
    {doc.status==='stale' && <p>原始依据已失效。可提交更正并补充当前适用的证据。</p>}
    <div className="knowledge-actions">
      {doc.status!=='withdrawn' && <button className="button" onClick={edit}>更正本条</button>}
      {doc.status==='current' && <ShareDraft project={project} experience={doc.id} revision={doc.current_revision} request={request}/>}
      {doc.status!=='withdrawn' && <button className="button" onClick={()=>setConfirm(v=>!v)}>撤回本条</button>}
    </div>
    {confirm && <div className="knowledge-section"><p>撤回后，后续任务将无法检索这条经验。已有执行仍可核对当时采用的版本。</p>
      <div className="knowledge-actions"><button className="button danger" disabled={busy} onClick={()=>void update('/withdraw',{expected_revision:doc.current_revision},'已撤回本条经验。')}>确认撤回</button><button className="button" onClick={()=>setConfirm(false)}>取消</button></div></div>}
    {error && <p role="alert">{error}</p>}{message && <p role="status">{message}</p>}
    {doc.changes.map(change=><details key={change.id} className="knowledge-section" open={change.status==='ready'||change.status==='pending'}>
      <summary>{changes[change.status]} · {change.title}</summary>
      <p>基于{change.expected_revision?'修订 '+change.expected_revision:'新条目'} · {formatTime(change.created_at)}</p>
      {change.reason && <p>更正原因 · {change.reason}</p>}
      {change.validity && <p>适用场景 · {change.validity}</p>}
      <MaterialReader markdown content={change.content}/><p>来源 · {change.source}</p>
      {change.review?.rationale && <p>审核意见 · {change.review.rationale}</p>}
      {change.receipt_path && <KnowledgeCuration project={project} path={change.receipt_path} request={request}/>}
      {(change.facts??[]).map((fact,i)=><section key={i}><h4>事实 {i+1}</h4><p>{fact.statement}</p><blockquote>{fact.quote}</blockquote>
        {change.status==='ready' && doc.status!=='withdrawn' && <button className="button primary" disabled={busy} onClick={()=>void update('/changes/'+change.id+'/adopt',{fact_number:i+1},'已采用审核事实，当前修订已更新。')}>采纳事实 {i+1}</button>}</section>)}
      {(change.status==='ready' || change.status==='pending') && <button className="button" disabled={busy} onClick={()=>void update('/changes/'+change.id+'/discard',{},'已放弃本次更正。')}>放弃本次更正</button>}
    </details>)}
    {doc.revisions.length>0 && <details className="knowledge-section"><summary>版本与依据（{doc.revisions.length}）</summary>
      {doc.revisions.map((v,index)=>{
        const prior=doc.revisions[index+1]
        return <section className="knowledge-section" key={v.revision}><h4>修订 {v.revision} · {v.title}</h4>
        <p className="knowledge-meta">{v.actor} · {formatTime(v.created_at)}</p>
        {v.change_reason && <p>变更原因 · {v.change_reason}</p>}<p>{v.statement}</p><blockquote>{v.quote}</blockquote><p>来源 · {v.source}</p>
        {v.validity && <p>适用场景 · {v.validity}</p>}
        {prior && <KnowledgeComparison summary={'比较修订 '+prior.revision+' 与 '+v.revision} from={'修订 '+prior.revision} to={'修订 '+v.revision}
          previous={comparisonText(prior.title,prior.source,prior.validity,prior.statement)} proposed={comparisonText(v.title,v.source,v.validity,v.statement)}/>}
        {v.receipt_path && <KnowledgeCuration project={project} path={v.receipt_path} request={request}/>}</section>
      })}
    </details>}
  </article>
}
