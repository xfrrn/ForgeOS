import { ThemedSelect } from './ThemedSelect'
import { useEffect, useState } from 'react'
import { Fact, type ApiRequest } from './shared'
import { ModelSelect } from './ModelSelect'
import { PageHeading } from './ui'
import './agent-settings.css'

import type { Skill } from './SkillLibrary'
type Settings = { prompt: string; harness: string; model: string; skills?: number[] }
type Config = Settings & { key: string; source: 'builtin' | 'global' | 'project'; revision: number; packages?: {name:string}[] }
type Run = { project_id: number; node: string; at: string; session: string; model: string; harness: string; outcome: string; config: Config }
type Entry = { key: string; role: string; title: string; harnesses: string[]; prompt: string; effective: Config; local_revision: number; last?: Run }
const sources = { builtin: '内置默认', global: '全局配置', project: '项目覆盖' }
const harnessNames: Record<string,string> = { codex: 'Codex CLI', claude: 'Claude Code', task: '跟随任务配置' }
function version(config: Config) { return sources[config.source] + (config.revision ? ' · v' + config.revision : ' · v1') }

export function AgentSettings({projectId, request}: {projectId?: number; request: ApiRequest}) {
  const path = projectId ? `/projects/${projectId}/agents` : '/projects/agent-settings'
  const [entries, setEntries] = useState<Entry[]>([])
  const [catalog, setCatalog] = useState<Skill[]>([])
  const [key, setKey] = useState('management-chat')
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [notice, setNotice] = useState('')
  useEffect(() => {
    const c = new AbortController()
    setError('')
    Promise.all([request<Entry[]>(path, {signal:c.signal}), request<Skill[]>('/projects/agent-skills', {signal:c.signal})]).then(([data, skills]) => {
      if (!c.signal.aborted) { setEntries(data); setCatalog(skills) }
    }).catch(e => { if (!c.signal.aborted) setError(e.message) })
    return () => c.abort()
  }, [path,request,reload])
  const selected = entries.find(row => row.key === key)
  const choose = (entry: Entry) => {setKey(entry.key); setNotice('')}
  return <div className="agent-settings">
    <PageHeading title="Agent 管理" description={projectId ? '查看本项目各角色的生效配置与最近一次运行。' : '管理各项目继承的角色默认配置。'}
      actions={<div className="agent-actions"><a className="button" href="#/skills">打开 Skill 库</a>{projectId && <a className="button" href="#/agents">管理全局默认</a>}</div>}/>
    {error && <p className="panel" role="alert">{error} <button className="button" onClick={()=>setReload(n=>n+1)}>重新读取</button></p>}
    {!selected && !error && <p className="panel table-message" role="status">正在读取 Agent 配置…</p>}
    {selected && <div className="agent-layout">
      <nav className="panel agent-roles" aria-label="Agent 角色">{Array.from(new Set(entries.map(e=>e.role))).map(role => {
        const items = entries.filter(e=>e.role===role)
        return <button type="button" key={role} aria-pressed={selected.role===role} onClick={()=>choose(items[0])}>
          <strong>{role}</strong><span>{items.length} 项用途</span></button>
      })}</nav>
      <div className="agent-content">
        <section className="panel agent-panel">
          <h2>{selected.role}</h2>
          <ThemedSelect label="用途" value={key}
            options={entries.filter(e=>e.role===selected.role).map(e=>({value:e.key,label:e.title}))}
            onChange={value=>choose(entries.find(item=>item.key===value)!)}/>
          <AgentForm key={[selected.key,selected.local_revision,selected.effective.revision].join('/')} entry={selected} catalog={catalog}
            project={!!projectId} request={request} path={path}
            onSaved={async(message)=>{const rows=await request<Entry[]>(path);setEntries(rows);setNotice(message)}} onReload={()=>setReload(n=>n+1)}/>
          {notice && <p role="status">{notice}</p>}
        </section>
        <section className="panel agent-panel"><h2>最近一次运行</h2>
        {selected.last ? <RunDetails run={selected.last} catalog={catalog} projectChat={selected.key==='management-chat'}/> : <p>此用途尚无新版 Worker 的运行回执。</p>}</section>
      </div>
    </div>}
  </div>
}
function AgentForm({entry,project,path,request,onSaved,onReload,catalog}: {entry: Entry;catalog:Skill[];project:boolean;path:string;request:ApiRequest;onSaved:(s:string)=>Promise<void>;onReload:()=>void}) {
  const current=entry.effective
  const [prompt,setPrompt]=useState(current.prompt)
  const [harness,setHarness]=useState(current.harness)
  const [model,setModel]=useState(current.model)
  const [skills,setSkills]=useState(current.skills ?? [])
  const [saving,setSaving]=useState(false)
  const [error,setError]=useState('')
  const own=project ? current.source==='project' : current.source==='global'
  const changed=prompt!==current.prompt || harness!==current.harness || model.trim()!==current.model || skills.join()!==(current.skills ?? []).join()
  function selectSkill(skill: Skill, checked: boolean) {
    setSkills(ids=>{
      if(checked && ids.includes(skill.id))return ids
      const retained=ids.filter(id=>id!==skill.id && (!checked || catalog.find(s=>s.id===id)?.name!==skill.name))
      return checked ? [...retained,skill.id] : retained
    })
  }
  const save=async(reset:boolean)=>{
    setSaving(true);setError('')
    try {
      await request(path,{method:'PUT',body:JSON.stringify({key:entry.key,previous:entry.local_revision,effective:current.revision,
        reset,settings:reset?null:{prompt,harness:entry.key==='management-chat'?'codex':harness,model:entry.key==='management-chat'?'':model.trim(),skills}})})
      await onSaved(reset?'已恢复继承。':'已保存，新领取的任务将使用此版本。')
    } catch(e){setError(e instanceof Error?e.message:'保存失败。')} finally{setSaving(false)}
  }
  return <form onSubmit={e=>{e.preventDefault();void save(false)}}>
    <p>当前生效版本　<strong>{version(current)}</strong></p>
    {entry.key==='management-chat' ? <p>项目聊天使用 ForgeOS 内核。{project ? <a href={path.replace(/^\/projects\/(\d+)\/agents$/, '#/settings/credentials?project=$1')}>管理共享聊天模型</a> : <a href="#/settings/credentials">管理共享聊天模型</a>}</p> : <>
    <div className="agent-fields"><ThemedSelect label="执行 Harness" value={harness}
      options={entry.harnesses.map(value=>({value,label:harnessNames[value]}))}
      onChange={value=>{setHarness(value);setModel('')}}/>
    <ModelSelect key={harness} request={request} harness={harness} value={model} onChange={(id,provider)=>{
      setModel(id)
      if(harness==='task' && provider)setHarness(provider)
    }}/></div>
    </>}
    {(entry.key==='planner' || entry.key==='operations') && <p>新派发的规划任务使用此 Harness。子任务使用任务实现的配置。</p>}
    {entry.key==='implementation' && <p>Harness 选择在新任务派发时生效。派发时明确选择的 Harness 优先。</p>}
    <fieldset className="agent-skills" disabled={saving}>
      <legend>分配给此 Agent 的 Skill</legend>
      {catalog.length ? <p>可多选。任务启动时，Agent 会收到所选 Skill 的文件路径。</p> : <p>Skill 库为空。<a href="#/skills">前往导入 Skill 文件夹</a></p>}
      <div className="agent-skill-options">{catalog.map(skill=><label key={skill.id}>
        <input type="checkbox" checked={skills.includes(skill.id)} onChange={e=>selectSkill(skill,e.target.checked)}/>
        <span><strong>{skill.name}{catalog.some(s=>s.name===skill.name && s.id!==skill.id) && ' · #'+skill.id}</strong>
          <small>{skill.description}</small><small>{skill.entry}</small></span>
      </label>)}</div>
    </fieldset>
    <label>启动提示词<textarea value={prompt} rows={8} required onChange={e=>setPrompt(e.target.value)}/></label>
    <p>Worker 会追加本次任务的上下文、输出格式和执行约束。</p>
    <div className="agent-actions"><button className="button primary" disabled={saving || !changed || !prompt.trim()}>{saving?'正在保存…':project?'保存项目配置':'保存全局默认'}</button>
      {own && <button className="button" type="button" disabled={saving} onClick={()=>void save(true)}>{project?'恢复继承全局':'恢复内置默认'}</button>}</div>
    {error && <p role="alert">{error} <button type="button" className="button" onClick={onReload}>重新读取配置</button></p>}
  </form>
}
function RunDetails({run,catalog,projectChat}: {run:Run;catalog:Skill[];projectChat:boolean}) {
  return <><dl className="agent-facts"><Fact label="完成时间">{new Date(run.at).toLocaleString()}</Fact>
    <Fact label="执行结果">{run.outcome==='completed'?'已完成':'失败'}</Fact><Fact label="执行节点">{run.node}</Fact>
    <Fact label="实际 Harness">{projectChat && run.harness==='codex' ? 'Codex 内核' : harnessNames[run.harness]}</Fact><Fact label="实际模型">{run.model||'Harness 未报告'}</Fact>
    <Fact label="本次 Skills">{run.config.packages?.map(s=>s.name).join("、") || run.config.skills?.map(id=>catalog.find(s=>s.id===id)?.name ?? "#"+id).join("、") || "无"}</Fact>
    <Fact label="提示词版本">{version(run.config)}</Fact><Fact label="项目">#{run.project_id}</Fact>
    {run.session && <Fact label="原生会话" code>{run.session}</Fact>}</dl>
    <details><summary>查看本次启动提示词</summary><pre>{run.config.prompt}</pre></details></>
}
