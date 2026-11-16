import { useEffect, useRef, useState } from 'react'
import { Fact, Status, formatTime, type ApiRequest } from './shared'
import { RepositoryAnalysis } from './RepositoryAnalysis'
import { InitializationReports } from './InitializationReports'

type Source = { path: string; blob: string; kind: string; dependencies?: string[]; scripts?: string[]; problem?: string; unresolved_inputs?: number; dynamic_fields?: string[]
  workspace_dependencies?: string[]; workspace_source?: {path: string; blob: string}
  managed_dependencies?: string[]; bom_imports?: string[]; parent_sources?: {path:string;blob:string}[]
  profile_dependencies?: Record<string,{dependencies?:string[];managed_dependencies?:string[];bom_imports?:string[]}>
  local_environments?: Record<string, {start_script: string; port: number; health_path: string; timeout_seconds: number}>
  secret_requirements?: { environment: string; name: string; risk: string }[] }
type Report = { execution_id: number; commit_sha: string; github_origin: string; repository_id?: number; scanned_at: string; files: Source[] }
type StartupCheck = { id: number; environment: string; status: string }
const kinds: Record<string,string> = { guide: '入口文档', go: 'Go 模块', node: 'Node.js 包', composer: 'PHP Composer', python: 'Python 包', cargo: 'Rust Cargo', maven: 'Maven POM', ruby: 'Ruby Bundler', 'ruby-lock': 'Bundler 锁定文件', manifest: '依赖清单', lock: '依赖锁定文件', runtime: '构建与启动', forgeos: 'ForgeOS 配置' }
const problems: Record<string,string> = { dynamic_ruby: '包含需要 Bundler 求值的 Ruby 代码，声明依赖未解析', size_limit: '清单超过读取上限', analysis_budget: '清单继承或依赖展开达到本次扫描上限', read_budget: '已达到本次清单内容读取上限', invalid_manifest: '清单格式无效', invalid_project: '项目配置无效，请核对 project.yaml 声明。' }

export function RepositoryInventory({ task, request }: { task: number; request: ApiRequest }) {
  const [report, setReport] = useState<Report>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const lifetime = useRef<AbortController | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [])
  const [checks, setChecks] = useState<StartupCheck[]>([])
  const [pending, setPending] = useState(false)
  const [revision, setRevision] = useState(0)
  const checksPath = `/tasks/${task}/startup-checks`
  useEffect(() => {
    if (!report?.files.some(file => file.local_environments)) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const items = await request<StartupCheck[]>(checksPath, { signal: controller.signal })
        if (!controller.signal.aborted) setChecks(items)
      } catch (e) {
        if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '读取启动检查失败。')
      }
      if (!controller.signal.aborted) timer = setTimeout(refresh, 4000)
    }
    void refresh()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [report, checksPath, request, revision])
  async function start(environment: string, previous?: number) {
    const signal = lifetime.current?.signal
    if (!signal || pending) return
    setPending(true); setError('')
    try {
      await request(checksPath, { method: 'POST', signal, body: JSON.stringify({environment, previous_task_id: previous}) })
    } catch (e) { if (!signal.aborted) setError(e instanceof Error ? e.message : '派发启动检查失败。') }
    finally { if (!signal.aborted) { setPending(false); setRevision(value => value + 1) } }
  }
  async function scan() {
    const signal = lifetime.current?.signal
    if (busy || !signal) return
    setBusy(true); setError('')
    try {
      const result = await request<Report>(`/tasks/${task}/repository-inventory`, { method: 'POST', signal })
      if (!signal.aborted) setReport(result)
    } catch (e) { if (!signal.aborted) setError(e instanceof Error ? e.message : '读取仓库清单失败。') }
    finally { if (!signal.aborted) setBusy(false) }
  }
  return <section className="panel detail-card"><h2>仓库初始化清单</h2>
    {!report && <><p>读取本次 HEAD 检查对应的 Git 对象，保存文档、依赖和启动配置的来源。</p>
      <button className="button" disabled={busy} onClick={() => void scan()}>{busy ? '正在读取…' : '读取仓库清单'}</button></>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {report && <><dl className="detail-facts">
      <Fact label="提交" code>{report.commit_sha}</Fact>
      <Fact label="扫描时间">{formatTime(report.scanned_at)}</Fact>
      {report.github_origin && <Fact label="GitHub origin">{report.github_origin}</Fact>}
      {report.repository_id && <Fact label="已绑定仓库 ID">{report.repository_id}</Fact>}
    </dl>
    <RepositoryAnalysis task={task} request={request}/>
    <InitializationReports task={task} request={request}/>
    {report.files.length ? report.files.map(f => <details className="inventory-file" key={f.path}>
      <summary><code>{f.path}</code> · {kinds[f.kind]}</summary>
      <p>Git blob · <code>{f.blob}</code></p>
      {Boolean(f.dependencies?.length) && <p>{f.kind === 'ruby-lock' ? '锁文件中的直接依赖' : '声明依赖'} · {f.dependencies?.join('、')}</p>}
      {f.workspace_dependencies?.length ? <p>工作区共享依赖 · {f.workspace_dependencies.join('、')}</p> : null}
      {f.workspace_source && <p>继承来源 · {f.workspace_source.path} · <code>{f.workspace_source.blob}</code></p>}
      {f.parent_sources?.map(p=><p key={p.path}>父 POM · {p.path} · <code>{p.blob}</code></p>)}
      {f.managed_dependencies?.length ? <p>受管依赖 · {f.managed_dependencies.join('、')}</p> : null}
      {f.bom_imports?.length ? <p>导入 BOM · {f.bom_imports.join('、')}</p> : null}
      {f.profile_dependencies && Object.entries(f.profile_dependencies).map(([name,d])=><details key={name}>
        <summary>条件配置 · {name}</summary>
        {d.dependencies?.length ? <p>声明依赖 · {d.dependencies.join('、')}</p> : null}
        {d.managed_dependencies?.length ? <p>受管依赖 · {d.managed_dependencies.join('、')}</p> : null}
        {d.bom_imports?.length ? <p>导入 BOM · {d.bom_imports.join('、')}</p> : null}
      </details>)}
      {f.scripts?.map(name => <p key={name}><code>{f.kind === 'composer' ? 'composer run-script ' : f.kind === 'node' ? 'npm run ' : ''}{name}</code></p>)}
      {f.unresolved_inputs ? <p>未解析为包名的来源 · {f.unresolved_inputs} 项</p> : null}
      {f.dynamic_fields?.length ? <p>由构建后端动态提供 · {f.dynamic_fields.join('、')}</p> : null}
      {f.local_environments && <><h3>本地环境</h3>{Object.entries(f.local_environments).map(([env, config]) => {
        const history = checks.filter(check => check.environment === env)
        const latest = history[0]
        return <div key={env}><p>{env} · <code>{config.start_script}</code> · <code>127.0.0.1:{config.port}{config.health_path}</code> · {config.timeout_seconds} 秒</p>
          <button className="button" disabled={pending || (latest && !['DONE','FAILED'].includes(latest.status))}
            onClick={() => void start(env, latest?.id)}>{latest ? '重新检查' : '启动检查'} · {env}</button>
          {history.map(check => <p key={check.id}><a href={`#/tasks/${check.id}`}>启动检查 #{check.id}</a> · <Status value={check.status}/></p>)}
        </div>})}</>}
      {f.secret_requirements && <><h3>声明的密钥需求</h3>{f.secret_requirements.length ? <ul>{f.secret_requirements.map(r =>
        <li key={r.environment + '/' + r.name}>{r.environment} · <code>{r.name}</code> · {r.risk}</li>)}</ul> : <p>此版本未声明密钥需求。</p>}</>}
      {f.problem && <p>{problems[f.problem] || f.problem}</p>}
    </details>) : <p>此提交未匹配已支持的入口文件。</p>}</>}
  </section>
}
