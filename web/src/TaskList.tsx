import { ThemedSelect } from './ThemedSelect'
import { useEffect, useState } from 'react'
import { formatTime, Icon, projectHref, Status, statusLabels, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import { systemTaskTypes, workTypes, type TaskCategory } from './workTypes'

type Summary = {
  id: number; project_id: number; kind: string; status: string; title: string; repository: string
  issue_number?: number; pr_number?: number; worker?: string; updated_at: string | null; goal_id?: number
}
type Page = { items: Summary[]; next_before?: number; category_counts?: Record<string, number> }
const filters = ['PLANNING', 'ACTIVE', 'BLOCKED', 'READY', 'RUNNING', 'PAUSED', 'WAITING_INPUT', 'WAITING_ACCEPTANCE', 'ACCEPTED', 'DONE', 'FAILED', 'SUPERSEDED', 'CANCELLED']

function TaskCategoryNav({ projectId, category, status, counts }: {
  projectId: number; category: TaskCategory; status: string; counts?: Page['category_counts']
}) {
  function links(types: Record<string, string>) {
    return Object.entries(types).map(([key, label]) => {
      const params = new URLSearchParams({ category: key })
      if (status) params.set('status', status)
      return <a href={projectHref(projectId, '/tasks?' + params)} key={key} aria-current={category === key ? 'page' : undefined}>
        {label}{counts && <span className="task-category-count">{counts[key] ?? 0}</span>}
      </a>
    })
  }
  return <nav className="task-categories" aria-label="任务类型">
    <div className="project-secondary-links" role="group" aria-label="项目工作">{links(workTypes)}</div>
    <div className="project-secondary-links" role="group" aria-label="系统检查">
      <span className="muted">系统检查</span>{links(systemTaskTypes)}
    </div>
  </nav>
}

export default function TaskList({ projectId, request, queue, category, initialStatus = '' }: {
  projectId: number; request: ApiRequest; queue?: boolean; category?: TaskCategory; initialStatus?: string
}) {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState(() => filters.includes(initialStatus) ? initialStatus : '')
  const [cursors, setCursors] = useState<number[]>([])
  const [page, setPage] = useState<Page | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [lastRequest, setLastRequest] = useState('')
  const [synced, setSynced] = useState('')
  const cursor = cursors.at(-1)
  const status = queue ? 'WAITING_ACCEPTANCE' : filter
  const params = new URLSearchParams({ limit: '20', project_id: String(projectId) })
  if (query) params.set('q', query)
  if (status) params.set('status', status)
  if (category) params.set('category', category)
  if (cursor) params.set('before', String(cursor))
  const path = '/tasks?' + params

  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (query !== input.trim()) { setCursors([]); setQuery(input.trim()) }
    }, 230)
    return () => window.clearTimeout(timer)
  }, [input, query])

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    request<Page>(path, { signal: controller.signal }).then(value => {
      if (controller.signal.aborted) return
      setPage(value); setLastRequest(path)
      setSynced(new Date().toISOString())
    }).catch(err => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '读取任务记录失败。')
    })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [path, request, revision])

  function changeFilter(value: string) { setFilter(value); setCursors([]) }
  const visible = lastRequest === path ? page : null
  const filtered = Boolean(query || filter)
  const title = queue ? '待验收' : '项目任务'
  const taskHref = (id: number) => projectHref(projectId, '/tasks/' + id + (category ? '?category=' + category : ''))
  return <>
    <PageHeading title={title}
      description={synced ? <>最近同步 <time dateTime={synced}>{formatTime(synced)}</time></> : undefined}
      actions={<button className="button refresh" onClick={() => setRevision(n => n + 1)} disabled={loading}>
        <Icon name="RefreshCw" size={15}/>{loading ? '刷新中…' : '刷新'}
      </button>}/>
    {category && <TaskCategoryNav projectId={projectId} category={category} status={filter} counts={visible?.category_counts}/>}
    <section className="panel task-panel" aria-label={title} aria-busy={loading}>
      <div className="table-toolbar">
        <div className="tools">
          <label className="search"><Icon name="Search" size={16} /><input type="search" aria-label="搜索任务、仓库或编号"
            placeholder="搜索任务" maxLength={120} value={input} onChange={e => setInput(e.target.value)} /></label>
          {!queue && <ThemedSelect label="任务状态" hideLabel value={filter} onChange={changeFilter}
            options={[{value:'',label:'全部状态'},...filters.map(value=>({value,label:statusLabels[value]}))]}/>}
        </div>
      </div>
      {error ? <div role="alert"><EmptyState icon="AlertCircle" title="读取任务记录失败" description={error}>
        <button className="button" onClick={() => setRevision(n => n + 1)}>重试</button>
      </EmptyState></div> :
        visible && !visible.items.length ? <EmptyState icon={queue ? 'ClipboardCheck' : 'Inbox'}
          title={filtered ? '没有匹配的任务' : queue ? '当前没有待验收任务' : '该类型暂无任务'}>
          {filtered && <button className="button" onClick={() => { setInput(''); setQuery(''); changeFilter('') }}>清除筛选</button>}
        </EmptyState> : <div className="table-scroll"><table className="task-table">
          <thead><tr><th scope="col">任务 / 来源</th><th scope="col">状态</th><th scope="col" className="worker-cell">Worker</th><th scope="col" className="time-cell">最近更新</th><th scope="col" aria-label="进入详情" /></tr></thead>
          <tbody>{visible ? visible.items.map(t => <tr key={t.id}>
            <td><a className="task-title" href={taskHref(t.id)}><span className={'task-glyph ' + (t.kind === 'codex_issue' ? 'issue' : '')}>
              <Icon name={t.kind === 'codex_issue' ? 'Circle' : 'Terminal'} size={16}/></span><span>{t.title}</span></a>
              <div className="task-sub"><span className="mono">T-{t.id}</span><span className="sub-divider">/</span>
                <span>{t.repository || '本地项目 ' + t.project_id}{t.issue_number ? ' #' + t.issue_number : ''}</span>
                {category && t.goal_id && <a href={taskHref(t.goal_id)}>子任务 · T-{t.goal_id}</a>}
              </div></td>
            <td><Status value={t.status}/></td>
            <td className="worker-cell"><span className="worker-label">{t.worker && <Icon name="Terminal" size={13}/>} {t.worker || '—'}</span></td>
            <td className="time-cell"><time dateTime={t.updated_at || undefined}>{t.updated_at ? formatTime(t.updated_at) : '—'}</time></td>
            <td><a className="row-link" href={taskHref(t.id)} aria-label={'查看任务 ' + t.id}><Icon name="ChevronRight" size={16}/></a></td>
          </tr>) : <tr><td colSpan={5} className="loading-row">正在读取任务…</td></tr>}</tbody>
        </table></div>}
      <div className="table-foot"><span aria-live="polite">{error ? '读取失败' : visible ? '本页 ' + visible.items.length + ' 项' : '正在读取…'}</span>
        <div><button className="pager" aria-label="上一页" disabled={!cursors.length || loading} onClick={() => setCursors(prev => prev.slice(0, -1))}><Icon name="ChevronRight" className="flip" size={15}/></button>
        <span>第 {cursors.length + 1} 页</span><button className="pager" aria-label="下一页" disabled={!visible?.next_before || loading} onClick={() => { if (visible?.next_before) setCursors(prev => [...prev, visible.next_before!]) }}><Icon name="ChevronRight" size={15}/></button></div>
      </div>
    </section>
  </>
}
