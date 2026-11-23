import { useState } from 'react'
import { ThemedSelect } from './ThemedSelect'
import { projectHref, Status, type ApiRequest } from './shared'

export type Step = {name:string; scope:string; harness:string; model:string}
export type StepReceipt = Step & {task:number; status:string; execution?:number; commit?:string; error?:string; revision?:{note:string;start:string}}
export type BuildInput = {task:number; execution:number; commit:string; name:string}
export function emptyStep(name='',harness='codex'):Step {
  return {name, scope:'', harness, model:''}
}

type ModelHint = {backend:string;model:string}
function StepModel({step,hints,onChange}:{step:Step;hints:ModelHint[];onChange:(model:string)=>void}){
  const ids=[...new Set(hints.filter(h=>h.backend===step.harness && h.model && h.model!=='default').map(h=>h.model))]
  const manual=!!step.model && !ids.includes(step.model)
  return <>
    <ThemedSelect label="执行模型" value={manual?'@custom':step.model} options={[{value:'',label:'客户端默认模型'},...ids.map(id=>({value:id,label:id})),{value:'@custom',label:'指定其他模型 ID'}]}
      onChange={onChange}/>
    {manual && <label>模型 ID<input value={step.model==='@custom'?'':step.model} placeholder="输入该客户端可用的模型 ID" maxLength={128} onChange={e=>onChange(e.target.value.trim()||'@custom')}/></label>}
  </>
}
export function StepEditor({steps,hints,change}:{steps:Step[];hints:ModelHint[];change:(items:Step[])=>void}) {
  function edit(at:number,patch:Partial<Step>){
    change(steps.map((s,i)=>i===at?{...s,...patch}:s))
  }
  return <div className="feature-build-steps">
    {steps.map((s,i)=><fieldset className="proposal-form" key={i}>
      <legend>实现任务 {i+1}</legend>
      <label>任务名称<input value={s.name} maxLength={60} onChange={e=>edit(i,{name:e.target.value})}/></label>
      <label>实现范围与交付要求<textarea rows={2} value={s.scope} maxLength={1800} onChange={e=>edit(i,{scope:e.target.value})}/></label>
      <ThemedSelect label="执行客户端" value={s.harness} options={[{value:'codex',label:'Codex'},{value:'claude',label:'Claude Code'}]} onChange={harness=>edit(i,{harness,model:''})}/>
      <StepModel key={s.harness} step={s} hints={hints} onChange={model=>edit(i,{model})}/>
      {steps.length>2 && <button type="button" className="button" onClick={()=>change(steps.filter((_,at)=>at!==i))}>移除此任务</button>}
    </fieldset>)}
    {steps.length<8 && <button type="button" className="button" onClick={()=>change([...steps,emptyStep()])}>添加实现任务</button>}
  </div>
}

export function BuildProgress({steps,inputs,project,execution,failed,path,request,refresh}:{steps:StepReceipt[];inputs?:BuildInput[];project:number;execution?:number;failed:boolean;path:string;request:ApiRequest;refresh:()=>void}){
  const [selected,select]=useState<number>()
  const [note,setNote]=useState('')
  const [busy,setBusy]=useState(false)
  const [error,setError]=useState('')
  async function submit(s:StepReceipt,redo:boolean){
    setBusy(true);setError('')
    try {
      await request(redo?path+'/revise-step':'/executions/'+s.execution+'/retry',{method:'POST',body:redo?JSON.stringify({task:s.task,execution,note}):undefined})
      select(undefined);setNote('')
    } catch(e){setError(e instanceof Error?e.message:'操作失败')}
    finally{setBusy(false);refresh()}
  }
  return <section><h3>实现任务</h3>
    {steps.map(s=><details className="proposal-job" key={s.task} open={s.status==='FAILED'||selected===s.task}>
      <summary>{s.name} · <Status value={s.status}/></summary>
      <p>{s.harness==='claude'?'Claude Code':'Codex'} · {s.model||'客户端默认模型'}</p>
      <p className="chat-message-text">{s.scope}</p>
      {s.revision && <p>修订要求 · {s.revision.note}</p>}
      <a href={projectHref(project,'/tasks/'+s.task)}>任务 T-{s.task}</a>
      {s.commit && <p>实现提交 <code>{s.commit.slice(0,12)}</code></p>}
      {s.error && <p className="detail-problem">{s.error}</p>}
      {s.status==='FAILED' && <button type="button" className="button" disabled={busy} onClick={()=>void submit(s,false)}>重试此任务</button>}
      {failed && s.status==='DONE' && (selected===s.task?<div className="proposal-form">
        <label>修订要求<textarea value={note} maxLength={1500} onChange={e=>setNote(e.target.value)}/></label>
        <button className="button" disabled={busy||!note.trim()} onClick={()=>void submit(s,true)}>重新派发此任务</button>
        <button className="button" onClick={()=>select(undefined)}>取消</button></div>:
        <button className="button" onClick={()=>{select(s.task);setNote('')}}>修订此任务并重新合成</button>)}
    </details>)}
    {inputs?.length ? <details><summary>本次合成采用的提交</summary>{inputs.map(i=><p key={i.task}>{i.name} · <a href={projectHref(project,'/tasks/'+i.task+'?execution='+i.execution)}>执行 #{i.execution}</a> · <code>{i.commit.slice(0,12)}</code></p>)}</details>:null}
    {error && <p className="detail-problem" role="alert">{error}</p>}
  </section>
}
