import { useEffect, useRef, useState } from 'react'
import { DesignWireframe, GenerateWireframe, type WireElement } from './DesignWireframe'
import { StepEditor, BuildProgress, emptyStep, type StepReceipt, type BuildInput, type Step } from './FeatureBuild'
import { formatTime, projectHref, Status, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'
import type { ChatModel } from './ProjectChatProvider'
import type { Detail, Post } from './FeatureProposals'

type DispatchAction = {kind:string; status:string; repository:string; issue_number?:number; task_id?:number; steps?:Step[]}
export type DesignReceipt = {
  turn_id?:number; action?:DispatchAction; progress?:{status:string}
  result?:{steps?:StepReceipt[];inputs?:BuildInput[];execution?:number; commit?:string; error?:string; exit_code?:number; validation_exit_code?:number; acceptance?:string; acceptance_note?:string; reviews?:{execution_id:number;commit:string;decision:string;note:string}[]; pr_number?:number; merge_sha?:string; delivery?:string; previews?:{id:string;execution_id:number;state:string;commit:string}[]}
  activities?:{key:string;role:string;label:string;path:string;at:string}[]
}
const stages:Record<string,string>={design_draft:'主控设计稿',design_review:'顾问审阅',design_final:'设计定稿'}
const states:Record<string,string>={queued:'等待模型领取',running:'进行中',completed:'已完成',failed:'失败'}
const previewStates:Record<string,string>={active:'可试用',expired:'已到期',stopped:'已停止'}

function ExecutionReceipt({receipt,project,path,request,refresh}:{receipt:DesignReceipt;project:number;path:string;request:ApiRequest;refresh:()=>void}){
  const action=receipt.action
  if(!action)return null
  const task=action.task_id
  const result=receipt.result
  const repo='https://github.com/'+action.repository
  return <section className="proposal-review"><h3>执行回执</h3>
    {action.issue_number && <a href={repo+'/issues/'+action.issue_number} target="_blank" rel="noreferrer">{action.repository} #{action.issue_number}</a>}
    {task && <div className="detail-actions"><a className="button" href={projectHref(project,'/tasks/'+task)}>{result?.steps && receipt.progress?.status==='WAITING_ACCEPTANCE'?'预览与验收 · ':''}任务 T-{task}</a>{receipt.progress && <Status value={receipt.progress.status}/>}</div>}
    {action.status==='pending' && <p>派工已登记，等待核对 Issue 和任务回执。</p>}
    {action.status==='stale' && <p>Issue 内容、状态或关联任务已变化，本次派工已失效。可以在当前轮次修订设计后重新派工。</p>}
    {result?.steps && <BuildProgress steps={result.steps} inputs={result.inputs} project={project} execution={result.execution} failed={receipt.progress?.status==='FAILED'} path={path} request={request} refresh={refresh}/>}
    {result?.execution && <p><a href={projectHref(project,'/tasks/'+task+'?execution='+result.execution)}>执行 #{result.execution}</a>{result.exit_code!=null && ' · 退出码 '+result.exit_code}{result.validation_exit_code!=null && ' · 验证退出码 '+result.validation_exit_code}</p>}
    {result?.commit && <p>{result.steps?'合成候选提交':'候选提交'} <code>{result.commit}</code></p>}
    {result?.error && <p className="detail-problem">{result.error}</p>}
    {result?.previews?.map(p=><p key={p.id}><a href={projectHref(project,'/tasks/'+task+'/previews/'+p.id+'/feedback')}>查看候选预览与反馈</a> · 执行 #{p.execution_id} · {previewStates[p.state]||p.state} · <code>{p.commit.slice(0,12)}</code></p>)}
    {!!result?.reviews?.length && <details open><summary>验收记录（{result.reviews.length}）</summary>{result.reviews.map(r=><p key={r.execution_id}>{r.decision==='approve'?'验收通过':'验收退回'} · <a href={projectHref(project,'/tasks/'+task+'?execution='+r.execution_id)}>执行 #{r.execution_id}</a> · <code>{r.commit.slice(0,12)}</code> · {r.note}</p>)}</details>}
    {result?.pr_number && <p><a href={repo+'/pull/'+result.pr_number} target="_blank" rel="noreferrer">PR #{result.pr_number}</a>{result.merge_sha && <> · 合并提交 <code>{result.merge_sha}</code></>}</p>}
    {!!receipt.activities?.length && <details><summary>任务活动（{receipt.activities.length}）</summary><ol>{receipt.activities.map(a=><li key={a.key}><a href={projectHref(project,a.path)}>{a.label}</a> · {a.role} · {formatTime(a.at)}</li>)}</ol></details>}
  </section>
}

export function FeatureDesign({round,current,receipt,project,models,path,request,post,busy,refresh}:{
  round:Detail;current:boolean;receipt:DesignReceipt;project:number;models:ChatModel[];path:string;request:ApiRequest;post:Post;busy:boolean;refresh:()=>void
}){
  const [feedback,setFeedback]=useState('')
  const [editTurn,setEditTurn]=useState<number>()
  const [element,setElement]=useState<WireElement>()
  const feedbackField=useRef<HTMLTextAreaElement>(null)
  const [mode,setMode]=useState('new')
  const [number,setNumber]=useState('')
  const [split,setSplit]=useState('single')
  const [steps,setSteps]=useState([emptyStep('前端'),emptyStep('后端','claude')])
  const invalidSteps=split==='multi' && (steps.some(s=>!s.name.trim() || !s.scope.trim() || (s.model && !/^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,127}$/.test(s.model))) || new Set(steps.map(s=>s.name.trim())).size!==steps.length)
  const [sending,setSending]=useState(false)
  const [problem,setProblem]=useState('')
  const [model,setModel]=useState('')
  const jobs=round.jobs.filter(j=>j.design_version)
  const last=jobs.at(-1)
  const final=last?.role==='design_final' && last.status==='completed'?last:undefined
  const action=receipt.turn_id===final?.id?receipt.action:undefined
  const canEdit=current && (!action || action.status==='stale')
  const revising=!!final && editTurn===final.id
  function editDesign(turn:number,target?:WireElement){
    if(editTurn!==turn)setFeedback('')
    setEditTurn(turn);setElement(target)
  }
  useEffect(()=>{if(revising)feedbackField.current?.focus()},[revising,element])
  async function dispatch(){
    if(!final || sending)return
    const issue=action?(action.kind==='dispatch_issue'?action.issue_number:0):mode==='existing'?Number(number):0
    setSending(true);setProblem('')
    try{
      await request(path+'/dispatch',{method:'POST',body:JSON.stringify({revision:round.approved_revision,turn_id:final.id,issue_number:issue,steps:!action && split==='multi'?steps:undefined})})
    }catch(e){setProblem(e instanceof Error?e.message:'派工失败，请重试。')}
    finally{setSending(false);refresh()}
  }
  return <>
    <h2>设计与交付 · 第 {round.version} 轮</h2>
    {!round.approved_revision ? <p>确认这一轮的综合方案后，即可开始设计。</p>:<>
      <details><summary>已拍板方案 v{round.approved_revision} · {round.approved_by}</summary><p className="chat-message-text">{round.revisions.find(r=>r.revision===round.approved_revision)?.body}</p></details>
      {!last && current && <button className="button primary" disabled={busy} onClick={()=>void post({operation:'start_design',revision:round.approved_revision},true)}>从已拍板方案生成设计稿</button>}
      {jobs.map(j=><details key={j.id} className="proposal-job" open={j.id===last?.id}>
        <summary>设计 v{j.design_version} · {stages[j.role]} · {states[j.status]}</summary>
        <small>{j.backend==='claude'?'Claude Code':j.backend==='codex'?'Codex':'API'} · {j.model} · 回合 #{j.id}</small>
        {j.role==='design_draft' && j.parent && <p>修订要求 · {j.text}</p>}
        {j.status==='completed' && (j.role==='design_draft'||j.role==='design_final') ? <>
          {j.wireframe ? <DesignWireframe frame={j.wireframe} selected={revising && j.id===final?.id?element?.id:undefined} onSelect={canEdit && j.id===final?.id?e=>editDesign(j.id,e):undefined}/>:
            <GenerateWireframe models={models} turn={j.id} revision={round.approved_revision} visual={round.jobs.filter(v=>v.role==='design_visual' && v.parent===j.id).at(-1)} busy={busy} post={post}/>}
          <details open={!j.wireframe}><summary>设计说明</summary><p className="chat-message-text">{j.wireframe?j.answer.replace(/```forgeos-ui[\s\S]*?```/,'').trim():j.answer}</p></details>
        </>:j.answer && <p className="chat-message-text">{j.answer}</p>}{j.status==='failed' && <p>{j.error||'此回合已超时。'}</p>}
      </details>)}
      {current && last?.status==='failed' && <div className="proposal-form"><ThemedSelect label="重试模型" value={model} options={[{value:'',label:'原模型'},...models.map(m=>({value:m.id,label:m.name||m.model}))]} onChange={setModel}/>
        <button className="button" disabled={busy} onClick={()=>void post({operation:'retry_design',turn_id:last.id,revision:round.approved_revision,profile_id:model||undefined},true)}>仅重试{stages[last.role]}</button></div>}
      {final && canEdit && <section className="proposal-review">{revising?<form className="proposal-form" onSubmit={async e=>{e.preventDefault();if(await post({operation:'revise_design',revision:round.approved_revision,turn_id:final.id,element_id:element?.id,body:feedback.trim()},true)){setEditTurn(undefined);setElement(undefined);setFeedback('')}}}>
        <label>{element?'修订控件「'+element.label+'」 · 设计 v'+final.design_version:'设计修订意见'}<textarea ref={feedbackField} required rows={3} maxLength={800} value={feedback} onChange={e=>setFeedback(e.target.value)}/></label>
        <div className="detail-actions"><button className="button" disabled={busy || !feedback.trim()}>生成新版并送审</button><button type="button" className="button" onClick={()=>{setEditTurn(undefined);setElement(undefined)}}>取消</button></div></form>:
        <button className="button" disabled={sending} onClick={()=>editDesign(final.id)}>提出设计修订</button>}</section>}
      {final && !revising && ((current && !action) || action?.status==='pending') && <section className="proposal-review proposal-form"><h3>正式派工</h3>
        {!action && <ThemedSelect label="派工方式" value={mode} options={[{value:'new',label:'创建本项目 GitHub Issue 并派工'},{value:'existing',label:'派发已有 Issue'}]} onChange={setMode}/>}
        {!action && mode==='existing' && <label>本项目尚未开始执行的开放 Issue 编号<input type="number" min="1" value={number} onChange={e=>setNumber(e.target.value)}/></label>}
        {!action && <ThemedSelect label="任务安排" value={split} options={[{value:'single',label:'单个实现任务'},{value:'multi',label:'拆分任务并合成候选版本'}]} onChange={setSplit}/>}
        {!action && split==='multi' && <StepEditor steps={steps} hints={round.jobs} change={setSteps}/>}
        {action?.steps?.map(s=><p key={s.name}>{s.name} · {s.harness} · {s.model||'客户端默认模型'}</p>)}
        <p>将设计 v{final.design_version} 交给本项目开发 Worker 执行，验收依据为上方定稿。</p>
        <button className="button primary" disabled={sending||busy||(!action && invalidSteps)||(!action && mode==='existing' && !/^[1-9]\d*$/.test(number))} onClick={()=>void dispatch()}>{sending?'正在核对派工…':action?'继续核对并完成派工':'确认派工 · 设计 v'+final.design_version}</button>
      </section>}
      {problem && <p className="detail-problem" role="alert">{problem}</p>}
      <ExecutionReceipt receipt={receipt} project={project} path={path} request={request} refresh={refresh}/>
    </>}
  </>
}
