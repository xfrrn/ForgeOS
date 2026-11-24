import { useEffect, useRef, useState } from 'react'
import { ChatMarkdown } from './ChatMarkdown'
import { DesignWireframe, GenerateWireframe, type WireElement, type Wireframe } from './DesignWireframe'
import type { FeatureBriefState } from './FeatureBrief'
import { FeatureDesign, type DesignReceipt } from './FeatureDesign'
import { ApiError, formatTime, Icon, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'
import { useLibraryResource } from './MaterialCatalog'
import type { ChatModel } from './ProjectChatProvider'

type Round = {id:number; version:number; approved_revision:number|null}
type Material = {key:string; revision:number; title:string}
type Draft = {wireframe?:Wireframe|null;visual_revision?:number;design_version?:number; id:number; role:string; parent:number|null; backend:string; model:string; profile:string; status:string; text:string; answer:string; error:string}
type Revision = {wireframe?:Wireframe;revision:number; body:string; author:string}
export type Detail = Round & {
  snapshot:{title:string; brief:string; head:string; notes:{id:number; body:string}[]; discussion:{id:number; text:string; answer:string}[]; materials:(Material & {content:string})[]}
  approved_by:string; approved_at:string; jobs:Draft[]; revisions:Revision[]
}
const roles:Record<string,string> = {lead:'主控原稿',advisor:'顾问原稿',comparison:'逐项比较',followup:'追问'}
const states:Record<string,string> = {queued:'等待领取',running:'撰写中',failed:'失败',completed:'已完成'}
export type Post = (body:Record<string,unknown>, action?:boolean)=>Promise<boolean>

function ProposalForm({project,feature,profile,preferredModel,models,request,expected,busy,post}:{
  project:number; feature:number; profile:string; preferredModel?:string; models:ChatModel[]; request:ApiRequest; expected:number; busy:boolean; post:Post
}) {
  const briefPath = `/projects/${project}/features/${feature}/brief`
  const [brief,setBrief]=useState('')
  const [briefMode,setBriefMode]=useState<'preview'|'edit'>('edit')
  const [savedBrief,setSavedBrief]=useState('')
  const [briefNotice,setBriefNotice]=useState('')
  const [briefError,setBriefError]=useState('')
  const [generatingBrief,setGeneratingBrief]=useState(false)
  const [briefPoll,setBriefPoll]=useState(0)
  const touched=useRef(false)
  const [base,setBase]=useState(expected)
  const [lead,setLead]=useState(profile)
  const [advisor,setAdvisor]=useState('')
  const [query,setQuery]=useState('')
  const [selected,setSelected]=useState<Material[]>([])
  const materials=useLibraryResource<Material[]>(request,'/projects/'+project+'/materials?q='+encodeURIComponent(query))
  const options=models.map(m=>({value:m.id,label:m.name || m.model}))
  const leadID=options.some(m=>m.value===lead)?lead:options[0]?.value || ''
  const advisors=options.filter(m=>m.value!==leadID)
  const advisorID=advisors.some(m=>m.value===advisor)?advisor:advisors[0]?.value || ''

  useEffect(() => {
    const controller = new AbortController()
    let timer: number
    async function readBrief() {
      try {
        const st = await request<FeatureBriefState>(briefPath, { signal: controller.signal })
        if (controller.signal.aborted) return
        setSavedBrief(st.brief || '')
        const running = st.job?.status === 'queued' || st.job?.status === 'running'
        setGeneratingBrief(running)
        if (st.job?.status === 'failed' && briefPoll > 0) {
          setBriefError(st.job.error || '从讨论汇总需求失败，请重试。')
        }
        if (st.brief && !touched.current) {
          setBrief(st.brief)
          setBriefMode('preview')
          setBriefNotice('已自动带入讨论阶段整理的本轮需求终稿，可切换「编辑原文」微调后冻结。')
        }
        if (running) timer = window.setTimeout(readBrief, 2000)
      } catch { /* Brief prefill is optional; manual input always works. */ }
    }
    void readBrief()
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [briefPath, request, briefPoll])

  async function generateFromDiscussion() {
    if (generatingBrief || !leadID) return
    setGeneratingBrief(true)
    setBriefError('')
    setBriefNotice('')
    touched.current = false
    try {
      await request(`${briefPath}/generate`, {
        method: 'POST',
        body: JSON.stringify({
          client_id: crypto.randomUUID(),
          profile_id: leadID,
          model: leadID === profile ? preferredModel : undefined,
        }),
      })
      setBriefPoll(n => n + 1)
    } catch (e) {
      setGeneratingBrief(false)
      setBriefError(e instanceof Error ? e.message : '发起需求汇总失败，请重试。')
    }
  }

  return <form className="proposal-form" onSubmit={e=>{e.preventDefault();if(!brief.trim())return;void post({expected_version:base,brief:brief.trim(),lead:leadID,advisor:advisorID,materials:selected.map(({key,revision})=>({key,revision}))})}}>
    <section className="proposal-brief-field" aria-label="本轮需求">
      <div className="proposal-brief-head">
        <div className="proposal-brief-title">
          <strong>本轮需求</strong>
          {briefNotice && <span className="feature-assist-status" role="status">{briefNotice}</span>}
        </div>
        <div className="proposal-brief-tools">
          <div className="proposal-brief-mode" role="group" aria-label="需求显示模式">
            <button type="button" className="button" aria-pressed={briefMode === 'preview'} disabled={!brief.trim()}
              onClick={() => setBriefMode('preview')}>预览排版</button>
            <button type="button" className="button" aria-pressed={briefMode === 'edit'}
              onClick={() => setBriefMode('edit')}>编辑原文</button>
          </div>
          <button type="button" className="button feature-assist-btn" disabled={busy || generatingBrief || !leadID} onClick={() => void generateFromDiscussion()}>
            <Icon name="Sparkles" size={13}/>{generatingBrief ? '正在从讨论汇总需求…' : '从讨论自动汇总需求'}
          </button>
          {savedBrief && savedBrief.trim() !== brief.trim() && <button type="button" className="button" disabled={busy}
            onClick={() => { setBrief(savedBrief); setBriefMode('preview'); touched.current = true; setBriefNotice('已载入讨论阶段保存的需求终稿。') }}>
            载入已整理终稿
          </button>}
        </div>
      </div>
      {briefError && <p className="detail-problem" role="alert">{briefError}</p>}
      {briefMode === 'preview' && brief.trim() ? <div className="proposal-brief-preview">
        <ChatMarkdown content={brief}/>
      </div> : <textarea className="proposal-brief-textarea" aria-label="本轮需求" required maxLength={4000} rows={14} value={brief}
        placeholder="填写本轮需求（支持 Markdown：背景与目标、功能范围、约束与验收标准），或点击上方「从讨论自动汇总需求」一键生成…"
        onChange={e=>{touched.current=true;setBrief(e.target.value)}}/>}
    </section>
    <div className="proposal-columns">
      <ThemedSelect label="主控模型" value={leadID} options={options} onChange={setLead}/>
      <ThemedSelect label="顾问模型" value={advisorID} options={advisors} onChange={setAdvisor}/>
    </div>
    <details className="proposal-materials-picker"><summary>引用项目资料（{selected.length}）</summary>
      <div className="proposal-materials-body">
        <label className="proposal-materials-search">搜索资料<input placeholder="按标题筛选项目资料…" value={query} onChange={e=>setQuery(e.target.value)}/></label>
        {materials?.error && <p role="alert">{materials.error}</p>}
        <div className="proposal-materials">{[...selected,...(materials?.data||[]).filter(m=>!selected.some(s=>s.key===m.key))].map(m=>
          <label key={m.key}><input type="checkbox" checked={selected.some(s=>s.key===m.key)} onChange={e=>setSelected(e.target.checked?[...selected,m]:selected.filter(s=>s.key!==m.key))}/><span>{m.title} · v{m.revision}</span></label>)}</div>
      </div>
    </details>
    <p>两位模型将收到同一份需求、最近 24 个已完成的讨论回合、当前话题记录、选定资料和仓库提交号。</p>
    {advisors.length===0 && <p>请连接第二个模型以发起独立提案。</p>}
    {base!==expected && <p role="alert">另一个窗口已发起第 {expected} 轮。<button className="button" type="button" onClick={()=>setBase(expected)}>基于最新轮次继续</button></p>}
    <button className="button primary" disabled={busy || generatingBrief || base!==expected || !advisorID || !brief.trim()}>冻结需求并分别出稿</button>
  </form>
}

function ProposalJob({job,jobs,active,models,busy,post}:{job:Draft;jobs:Draft[];active:boolean;models:ChatModel[];busy:boolean;post:Post}){
  const [ask,setAsk]=useState(false)
  const [question,setQuestion]=useState('')
  const [replacement,setReplacement]=useState('')
  const name=job.backend==='claude'?'Claude Code · '+job.model:job.backend==='codex'?'Codex · '+job.model:job.model
  const retried=jobs.some(other=>other.parent===job.id && other.role===job.role)
  return <article className="proposal-job">
    <strong>{roles[job.role]} · {name}</strong> <small>{states[job.status]} · #{job.id}</small>
    {job.role==='followup' && <p>追问 #{job.parent} · {job.text}</p>}
    {job.answer && <details open={job.role==='lead' || job.role==='advisor'}><summary>查看原文</summary><ChatMarkdown content={job.answer}/></details>}
    {job.status==='failed' && <p role="status">{job.error || '模型未在期限内完成本轮。'}</p>}
    {active && job.status==='failed' && !retried && <div className="proposal-form">
      <ThemedSelect label="重试模型" value={replacement} options={[{value:'',label:'使用原模型'},...models.filter(m=>m.id!==job.profile && !jobs.some(j=>((job.role==='lead' && j.role==='advisor') || (job.role==='advisor' && j.role==='lead')) && j.profile===m.id)).map(m=>({value:m.id,label:m.name||m.model}))]} onChange={setReplacement}/>
      <button className="button" disabled={busy} onClick={()=>void post({operation:'retry',turn_id:job.id,profile_id:replacement || undefined},true)}>仅重试此回合</button>
    </div>}
    {active && job.status==='completed' && (ask ? <form className="proposal-form" onSubmit={async e=>{e.preventDefault();if(await post({operation:'ask',turn_id:job.id,body:question.trim()},true)){setAsk(false);setQuestion('')}}}>
      <label>向 {name} 追问<textarea required rows={3} maxLength={2000} value={question} onChange={e=>setQuestion(e.target.value)}/></label>
      <div className="detail-actions"><button className="button" disabled={busy || !question.trim()}>发送追问</button><button type="button" className="button" onClick={()=>setAsk(false)}>取消</button></div>
    </form>:<button className="button" onClick={()=>setAsk(true)}>追问这一稿</button>)}
  </article>
}

function ProposalReview({round,active,busy,post,models}:{models:ChatModel[];round:Detail;active:boolean;busy:boolean;post:Post}){
  const [edit,setEdit]=useState<{base:number;body:string}>()
  const [annotation,setAnnotation]=useState<{version:number;target:WireElement;note:string}>()
  const input=useRef<HTMLTextAreaElement>(null)
  useEffect(()=>{if(annotation)input.current?.focus()},[annotation?.target.id])
  const latest=round.revisions[0]
  if(!latest)return null
  return <section className="proposal-review">
    <h3>综合方案 · v{latest.revision}</h3>
    {latest.wireframe?<DesignWireframe frame={latest.wireframe} selected={annotation?.version===latest.revision?annotation.target.id:undefined} onSelect={active?target=>{setEdit(undefined);setAnnotation({version:latest.revision,target,note:annotation?.version===latest.revision?annotation.note:''})}:undefined}/>:
      active && <GenerateWireframe models={models} revision={latest.revision} visual={round.jobs.filter(j=>j.visual_revision===latest.revision).at(-1)} busy={busy} post={post}/>}
    {active && annotation?.version===latest.revision && <form className="proposal-form" onSubmit={async e=>{
      e.preventDefault();if(await post({operation:'revise',revision:annotation.version,element_id:annotation.target.id,body:annotation.note.trim()},true))setAnnotation(undefined)
    }}>
      <label>修订控件「{annotation.target.label}」 · 方案 v{annotation.version}<textarea ref={input} required rows={3} maxLength={800} value={annotation.note} onChange={e=>setAnnotation({...annotation,note:e.target.value})}/></label>
      <div className="detail-actions"><button className="button primary" disabled={busy||!annotation.note.trim()}>保存新版并更新可视稿</button><button type="button" className="button" onClick={()=>setAnnotation(undefined)}>取消</button></div>
    </form>}
    <ChatMarkdown content={latest.body}/>
    {round.revisions.slice(1).map(r=><details key={r.revision}><summary>历史修订 v{r.revision} · {r.author}</summary>{r.wireframe && <DesignWireframe frame={r.wireframe}/>}<ChatMarkdown content={r.body}/></details>)}
    {round.approved_revision && <p role="status">{round.approved_by} 已确认 v{round.approved_revision} · {formatTime(round.approved_at)} · 已进入设计</p>}
    {active && annotation?.version!==latest.revision && (edit ? <div className="proposal-form">
      <label>编辑 v{edit.base}<textarea className="proposal-brief-textarea" rows={14} value={edit.body} onChange={e=>setEdit({...edit,body:e.target.value})}/></label>
      {edit.base!==latest.revision && <p role="alert">综合稿已更新到 v{latest.revision}。请对照上方新版核对编辑内容。</p>}
      <div className="detail-actions">{edit.base===latest.revision?<button className="button" disabled={busy || !edit.body.trim()} onClick={async()=>{if(await post({operation:'revise',revision:edit.base,body:edit.body.trim()},true))setEdit(undefined)}}>保存为 v{edit.base+1}</button>:
        <button className="button" onClick={()=>setEdit({...edit,base:latest.revision})}>已核对，在新版上继续编辑</button>}
        <button className="button" onClick={()=>setEdit(undefined)}>取消编辑</button></div>
    </div>:<div className="detail-actions"><button className="button" disabled={busy} onClick={()=>setEdit({base:latest.revision,body:latest.body})}>修订综合稿</button>
      <button className="button primary" disabled={busy} onClick={()=>void post({operation:'approve',revision:latest.revision},true)}>确认 v{latest.revision}，进入设计</button></div>)}
  </section>
}

export function FeatureProposals({project,feature,profile,preferredModel,models,request,delivery,compose=false}:{delivery:boolean;compose?:boolean;project:number;feature:number;profile:string;preferredModel?:string;models:ChatModel[];request:ApiRequest}){
  const path='/projects/'+project+'/features/'+feature+'/proposals'
  const [rounds,setRounds]=useState<Round[]>()
  const [selected,setSelected]=useState(0)
  const [receipt,setReceipt]=useState<DesignReceipt>({})
  const [detail,setDetail]=useState<Detail>()
  const [newRound,setNewRound]=useState(compose)
  useEffect(() => {
    if (compose) setNewRound(true)
  }, [compose])
  const [epoch,refresh]=useState(0)
  const [busy,setBusy]=useState(false)
  const [error,setError]=useState('')
  const [readError,setReadError]=useState('')
  const intent=useRef({key:'',id:''})
  useEffect(()=>{
    const controller=new AbortController()
    let timer:number
    async function read(){
      try{
        const items=await request<Round[]>(path,{signal:controller.signal})
        const id=selected || items[0]?.id
        const next=id?await request<Detail>(path+'/'+id,{signal:controller.signal}):undefined
        const deliveryReceipt=delivery && id ? await request<DesignReceipt>(path+'/'+id+'/delivery',{signal:controller.signal}) : {}
        if(!controller.signal.aborted){setRounds(items);setDetail(next);setReceipt(deliveryReceipt);setReadError('')}
      }catch(e){if(!controller.signal.aborted)setReadError(e instanceof Error?e.message:'无法读取提案')}
      if(!controller.signal.aborted)timer=window.setTimeout(read,5000)
    }
    void read()
    return ()=>{controller.abort();window.clearTimeout(timer)}
  },[path,request,selected,epoch,delivery])
  const latest=rounds?.[0]
  const active=!!detail && latest?.id===detail.id && !detail.approved_revision
  const post:Post=async(body,action=false)=>{
    if(busy)return false
    const url=action?path+'/'+detail?.id+'/actions':path
    const key=JSON.stringify([url,body])
    if(key!==intent.current.key)intent.current={key,id:crypto.randomUUID()}
    setBusy(true);setError('')
    try{
      const result=await request<{id:number}|undefined>(url,{method:'POST',body:JSON.stringify({...body,client_id:intent.current.id})})
      if(result?.id){setSelected(result.id);setDetail(undefined);setNewRound(false)}
      window.dispatchEvent(new Event('forgeos-proposal-changed'));intent.current={key:'',id:''}
      return true
    }catch(e){setError(e instanceof ApiError && e.status===409?'当前版本、模型或输入已变化，请核对刷新后的轮次和修订。':e instanceof Error?e.message:'操作失败');return false}
    finally{setBusy(false);refresh(v=>v+1)}
  }
  return <section className="proposal-workspace" aria-label={delivery?"设计与交付":"方案与拍板"}>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {readError && <p className="detail-problem" role="alert">{readError} <button className="button" onClick={()=>refresh(v=>v+1)}>重新读取</button></p>}
    {!rounds && !readError && <p role="status">正在读取提案…</p>}
    {!!rounds?.length && <div className="detail-actions">
      <ThemedSelect label="需求轮次" value={String(selected||latest?.id)} options={rounds.map(r=>({value:String(r.id),label:'第 '+r.version+' 轮'+(r.approved_revision?' · 已确认 v'+r.approved_revision:'')}))} onChange={v=>{setDetail(undefined);setSelected(Number(v));setNewRound(false);setError('')}}/>
      {!delivery && <button className="button" onClick={()=>setNewRound(!newRound)}>{newRound?'返回方案':'修改需求，另开一轮'}</button>}
    </div>}
    {delivery ? (detail ? <FeatureDesign key={detail.id} round={detail} current={latest?.id===detail.id} path={path+'/'+detail.id} {...{request,post,busy,project,models,receipt}} refresh={()=>{refresh(v=>v+1);window.dispatchEvent(new Event('forgeos-proposal-changed'))}}/> : rounds?.length===0 && <p>先在「方案与拍板」中确认一版方案。</p>) : rounds && (newRound || !rounds.length) ? <ProposalForm {...{project,feature,profile,preferredModel,models,request,busy,post}} expected={latest?.version||0}/> : detail && <>
      <h2>第 {detail.version} 轮 · {detail.approved_revision?'已拍板':detail.revisions.length?'待拍板':'方案准备中'}</h2>
      <div className="proposal-snapshot-brief"><ChatMarkdown content={detail.snapshot.brief}/></div>
      <details><summary>冻结的需求与依据</summary>
        <p>仓库提交 <code>{detail.snapshot.head}</code></p>
        {detail.snapshot.notes.map(n=><p key={n.id}>记录 #{n.id} · {n.body}</p>)}
        {detail.snapshot.discussion.map(t=><details key={t.id}><summary>讨论 #{t.id} · {t.text}</summary><ChatMarkdown content={t.answer}/></details>)}
        {detail.snapshot.materials.map(m=><details key={m.key}><summary>{m.title} · v{m.revision}</summary><ChatMarkdown content={m.content}/></details>)}
      </details>
      <div className="proposal-columns">{detail.jobs.filter(j=>j.role==='lead'||j.role==='advisor').map(job=><ProposalJob key={job.id} {...{job,jobs:detail.jobs,active,busy,models,post}}/>)}</div>
      {detail.jobs.filter(j=>j.role==='comparison'||j.role==='followup').map(job=><ProposalJob key={job.id} {...{job,jobs:detail.jobs,active,busy,models,post}}/>)}
      <ProposalReview models={models} key={detail.id} round={detail} {...{active,busy,post}}/>
    </>}
  </section>
}
