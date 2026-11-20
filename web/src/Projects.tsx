import { GlobalRules } from './GlobalRules'
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, Status, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'

type Project = { id: number; path: string; repository_id?: number; check_id?: number; check_status?: string }
type Page = { items: Project[]; next_before?: number }

export function Projects({ request }: { request: ApiRequest }) {
  const [items, setItems] = useState<Project[]>([])
  const [cursor, setCursor] = useState<number>()
  const [path, setPath] = useState('')
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const lifetime = useRef<AbortController | null>(null)

  const load = useCallback(async (signal: AbortSignal, before?: number) => {
    setBusy(true); setError('')
    try {
      const page = await request<Page>('/projects' + (before ? '?before=' + before : ''), { signal })
      if (signal.aborted) return
      setItems(old => before ? old.concat(page.items.filter(p => !old.some(row => row.id === p.id))) : page.items)
      setCursor(page.next_before)
    } catch (e) { if (!signal.aborted) setError(e instanceof Error ? e.message : '无法读取项目。') }
    finally { if (!signal.aborted) setBusy(false) }
  }, [request])

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  async function register(e: FormEvent) {
    e.preventDefault()
    const signal = lifetime.current?.signal
    if (busy || !path.trim() || !signal) return
    setBusy(true); setError(''); setNotice('')
    try {
      const p = await request<Project>('/projects', { method: 'POST', body: JSON.stringify({ path: path.trim() }), signal })
      if (signal.aborted) return
      setItems(old => [{ ...old.find(row => row.id === p.id), ...p }, ...old.filter(row => row.id !== p.id)])
      setPath(''); setNotice('仓库已登记为项目 #' + p.id + '。')
    } catch (e) {
      if (!signal.aborted) setError(e instanceof ApiError && e.status === 400 ? '请填写控制面允许目录中的 Git 仓库根路径。' : e instanceof Error ? e.message : '登记失败。')
    } finally { if (!signal.aborted) setBusy(false) }
  }
  async function check(project: Project) {
    const signal = lifetime.current?.signal
    if (busy || !signal) return
    setBusy(true); setError(''); setNotice('')
    try {
      const task = await request<{ id: number }>(`/projects/${project.id}/check`, {
        method: 'POST', body: JSON.stringify(project.check_id ? { retry_task_id: project.check_id } : {}), signal,
      })
      if (!signal.aborted) location.hash = '/tasks/' + task.id
    } catch (e) { if (!signal.aborted) setError(e instanceof Error ? e.message : '无法派发检查。') }
    finally { if (!signal.aborted) setBusy(false) }
  }
  return <div className="project-setup project-directory">
    <PageHeading title="项目接入" description="登记本地 Git 仓库，进入项目的讨论、资料与决策。"/>
    {error && <p className="detail-problem" role="alert">{error} <button className="button" disabled={busy} onClick={() => lifetime.current && void load(lifetime.current.signal)}>刷新项目</button></p>}
    {notice && <p className="detail-notice" role="status">{notice}</p>}
    <div className="project-entry-grid">
      <div className="project-primary">
        <form className="panel detail-card" onSubmit={register}>
          <h2>登记本地仓库</h2>
          <label htmlFor="repo-path">本地仓库路径</label>
          <p id="repo-hint" className="muted">填写控制面机器上的仓库根目录。可以使用相对于 FORGEOS_REPO_ROOT 的路径。</p>
          <div className="project-input"><input id="repo-path" aria-describedby="repo-hint" autoComplete="off" required value={path} onChange={e => setPath(e.target.value)}/>
            <button className="button primary" disabled={busy || !path.trim()}>登记仓库</button></div>
        </form>
        <section className="panel detail-card"><h2>已登记项目</h2>
          {!items.length && !error && (busy ? <p role="status" className="table-message">正在读取项目…</p> :
            <EmptyState icon="GitBranch" title="尚未登记仓库"/>)}
          {items.map(p => <article key={p.id} className="registered-project"><div>
            <h3>项目 #{p.id}</h3><code>{p.path}</code>{p.repository_id && <p>GitHub 仓库 ID · {p.repository_id}</p>}
            {p.check_status && <p>最近的 HEAD 检查 <Status value={p.check_status}/></p>}</div>
            <div className="detail-actions"><a href={`#/projects/${p.id}/inbox`}>讨论收件箱</a><a href={`#/projects/${p.id}/materials`}>项目资料</a><a href={`#/projects/${p.id}/decisions`}>项目决策</a><a href={`#/projects/${p.id}/shares`}>共享经验</a>{p.check_id && <a href={'#/tasks/' + p.check_id}>查看检查 #{p.check_id}</a>}
              {(!p.check_id || p.check_status === 'FAILED' || p.check_status === 'DONE') && <button className="button" disabled={busy} onClick={() => void check(p)}>{p.check_status === 'DONE' ? '检查当前 HEAD' : p.check_id ? '重新检查' : '运行 HEAD 检查'}</button>}</div>
          </article>)}
          {cursor && <button className="button" disabled={busy} onClick={() => lifetime.current && void load(lifetime.current.signal, cursor)}>更多项目</button>}
        </section>
      </div>
      <GlobalRules request={request}/>
    </div>
  </div>
}
