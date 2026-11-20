import { ThemedSelect } from './ThemedSelect'
import { useEffect, useState } from 'react'
import { Fact, Status, formatTime, projectHref, type ApiRequest } from './shared'

type Revision = { revision: number; generated_at: string }
type File = { path: string; blob: string; problem?: string
  secret_requirements?: {environment: string; name: string; risk: string}[]
  local_environments?: Record<string,{start_script: string; port: number; health_path: string}> }
type Report = Revision & {task_id: number; repository_mapping: {project_id: number; path: string; github_origin: string; repository_id?: number}
  repository_inventory: {commit_sha: string; execution_id: number; files: File[]}
  project_profile: {status: string; failure?: string; omitted?: number; truncated?: number
    analysis?: {session_id: string; facts: {kind: string; value: string; path: string; quote: string}[]}}
  materials: {key: string; revision: number; kind: string; title: string; content_sha256: string}[]
  startup_checks: {task_id: number; environment: string; status: string; execution_id?: number; worker?: string; error?: string; receipt?: {http_status?: number; probes: number}}[] }

export function InitializationReports({projectId,task,request}:{projectId:number;task:number;request:ApiRequest}){
  const [revisions,setRevisions]=useState<Revision[]>([])
  const [report,setReport]=useState<Report>()
  const [busy,setBusy]=useState(true)
  const [error,setError]=useState('')
  const base=`/tasks/${task}/initialization-reports`
  useEffect(()=>{
    const stop=new AbortController()
    async function load(){
      try {
        const items=await request<Revision[]>(base,{signal:stop.signal})
        const latest=items.length ? await request<Report>(base+'/'+items[0].revision,{signal:stop.signal}) : undefined
        if (!stop.signal.aborted) {setRevisions(items);setReport(latest)}
      } catch(e){if(!stop.signal.aborted)setError(e instanceof Error?e.message:'读取报告失败。')}
      finally {if(!stop.signal.aborted)setBusy(false)}
    }
    void load();return ()=>stop.abort()
  },[base,request])
  async function open(revision?:number){
    if(busy)return
    setBusy(true);setError('')
    try {
      const r=await request<Report>(revision?base+'/'+revision:base,revision?{}:{method:'POST'})
      setReport(r)
      if (!revision) setRevisions(await request<Revision[]>(base))
    } catch(e){setError(e instanceof Error?e.message:'读取报告失败。')}
    finally {setBusy(false)}
  }
  function download(r:Report){
    const url=URL.createObjectURL(new Blob([JSON.stringify(r,null,2)+'\n'],{type:'application/json'}))
    const a=document.createElement('a');a.href=url;a.download=`forgeos-initialization-${task}-r${r.revision}.json`;a.click()
    setTimeout(()=>URL.revokeObjectURL(url),1000)
  }
  const cfg=report?.repository_inventory.files.find(f=>f.path==='project.yaml')
  const labels:Record<string,string>={profile:'项目用途',stack:'技术栈',dependency:'依赖',start:'启动命令',test:'测试命令'}
  return <section><h3>初始化报告</h3>
    <button className="button" disabled={busy} onClick={()=>void open()}>归档当前报告</button>
    {error && <p role="alert">{error}</p>}
    {report && <>
      <ThemedSelect label="报告版本" disabled={busy} value={String(report.revision)} onChange={value=>void open(Number(value))}
        options={revisions.map(r=>({value:String(r.revision),label:'第 '+r.revision+' 版 · '+formatTime(r.generated_at)}))}/> <button className="button" disabled={busy} onClick={()=>download(report)}>下载 JSON</button>
      <dl className="detail-facts"><Fact label="仓库映射">{report.repository_mapping.path}</Fact>
        <Fact label="GitHub 仓库">{report.repository_mapping.github_origin || '未记录 origin'} · {report.repository_mapping.repository_id || '未绑定数字 ID'}</Fact>
        <Fact label="来源提交" code>{report.repository_inventory.commit_sha}</Fact>
        <Fact label="HEAD 检查">T-{task} · 执行 #{report.repository_inventory.execution_id}</Fact></dl>
      <h4>项目画像</h4>
      {report.project_profile.analysis ? <>
        <p>归纳会话 · <code>{report.project_profile.analysis.session_id}</code></p>
        {Object.entries(labels).map(([kind,label])=>{
          const facts=report.project_profile.analysis!.facts.filter(f=>f.kind===kind)
          return <div key={kind}><strong>{label}</strong>{facts.length?facts.map((f,i)=><details key={i}>
            <summary>{f.value}</summary><p>{f.path} · <code>{report.repository_inventory.files.find(source=>source.path===f.path)?.blob}</code></p><pre>{f.quote}</pre></details>):<p>未从归档来源确认。</p>}</div>
        })}
        {(Boolean(report.project_profile.omitted)||Boolean(report.project_profile.truncated)) && <p>归纳省略 {report.project_profile.omitted} 项，截断 {report.project_profile.truncated} 项。</p>}
      </> : <p>归档时的 Agent 归纳 · {report.project_profile.status==='NOT_REQUESTED'?'未派发':<Status value={report.project_profile.status}/>}</p>}
      {report.materials.map(m=><p key={m.key}>{m.title} · {m.key} · 修订 {m.revision} · <code>{m.content_sha256}</code></p>)}
      <h4>环境与启动证据</h4>
      {['local-mock','local-integrated'].map(env=>{
        const declared=cfg?.local_environments?.[env]
        const checks=report.startup_checks.filter(c=>c.environment===env)
        return <div key={env}><strong>{env}</strong><p>{declared ? <><code>{declared.start_script}</code> · <code>127.0.0.1:{declared.port}{declared.health_path}</code></> : '清单中未记录此环境的声明'}</p>
          {checks.length ? checks.map(c=><p key={c.task_id}><a href={projectHref(projectId, `/tasks/${c.task_id}`)}>T-{c.task_id}</a> · <Status value={c.status}/> {c.worker} {c.receipt?.http_status && '· HTTP '+c.receipt.http_status} {c.error}</p>) : <p>归档时尚无启动检查。</p>}</div>
      })}
      <h4>密钥需求</h4>{cfg?.secret_requirements ? cfg.secret_requirements.length ? <ul>{cfg.secret_requirements.map(s=><li key={s.environment+'/'+s.name}>{s.environment} · {s.name} · {s.risk}</li>)}</ul> : <p>此版本声明的需求为空。</p> : <p>清单中没有可核对的需求声明。</p>}
      {cfg?.problem && <p>项目配置读取异常 · {cfg.problem}</p>}
    </>}
  </section>
}
