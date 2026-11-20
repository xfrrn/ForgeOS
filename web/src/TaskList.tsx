import { useEffect, useState } from 'react'
import { formatTime, Icon, Status, statusLabels, type ApiRequest } from './shared'
import { EmptyState, MetricCard, PageHeading } from './ui'

type Summary = {
  id: number; project_id: number; kind: string; status: string; title: string; repository: string
  issue_number?: number; pr_number?: number; worker?: string; updated_at: string | null
}
type Page = { items: Summary[]; counts: Record<string, number>; next_before?: number }
const filters = ['PLANNING', 'ACTIVE', 'BLOCKED', 'READY', 'RUNNING', 'PAUSED', 'WAITING_INPUT', 'WAITING_ACCEPTANCE', 'ACCEPTED', 'DONE', 'FAILED', 'SUPERSEDED', 'CANCELLED']
const metrics = [
  { key: 'ALL', name: '全部任务', icon: 'Layers3' },
  { key: 'RUNNING', name: '执行中', icon: 'Activity' },
  { key: 'WAITING_ACCEPTANCE', name: '待验收', icon: 'ClipboardCheck' },
  { key: 'DONE', name: '已完成', icon: 'CheckCircle2' },
] as const

export default function TaskList({ request, queue, running, onCounts }: {
  request: ApiRequest; queue: boolean; running: boolean; onCounts: (counts: Record<string, number>) => void
}) {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('')
  const [cursors, setCursors] = useState<number[]>([])
  const [page, setPage] = useState<Page | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [lastRequest, setLastRequest] = useState('')
  const [synced, setSynced] = useState('')
  const cursor = cursors.at(-1)
  const status = queue ? 'WAITING_ACCEPTANCE' : running ? 'RUNNING' : filter
  const params = new URLSearchParams({ limit: '20' })
  if (query) params.set('q', query)
  if (status) params.set('status', status)
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
      setPage(value); setLastRequest(path); onCounts(value.counts)
      setSynced(new Date().toISOString())
    }).catch(err => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '读取任务记录失败。')
    })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [path, request, revision, onCounts])

  function changeFilter(value: string) { setFilter(value); setCursors([]) }
  const visible = lastRequest === path ? page : null
  const filtered = Boolean(query || filter)
  return <>
    <PageHeading title={queue ? '验收队列' : running ? '执行中任务' : '任务列表'}
      description={synced ? <>最近同步 <time dateTime={synced}>{formatTime(synced)}</time></> : undefined}
      actions={<button className="button refresh" onClick={() => setRevision(n => n + 1)} disabled={loading}>
        <Icon name="RefreshCw" size={15}/>{loading ? '刷新中…' : '刷新'}
      </button>}/>
    {!queue && !running && <div className="metrics" role="group" aria-label="按状态筛选任务">
      {metrics.map(m => <MetricCard key={m.key} label={m.name} value={page?.counts[m.key] ?? '—'} icon={m.icon}
        pressed={(filter || 'ALL') === m.key} onClick={() => changeFilter(m.key === 'ALL' ? '' : m.key)}/>)}
    </div>}
    <section className="panel task-panel" aria-labelledby="task-table-heading" aria-busy={loading}>
      <div className="table-toolbar"><h2 id="task-table-heading">{queue ? '待处理任务' : '任务记录'}</h2>
        <div className="tools">
          <label className="search"><Icon name="Search" size={16} /><input type="search" aria-label="搜索任务、仓库或编号"
            placeholder="搜索任务" maxLength={120} value={input} onChange={e => setInput(e.target.value)} /></label>
          {!queue && !running && <label className="filter"><Icon name="SlidersHorizontal" size={15} />
            <select aria-label="任务状态" value={filter} onChange={e => changeFilter(e.target.value)}>
              <option value="">全部状态</option>{filters.map(key => <option key={key} value={key}>{statusLabels[key]}</option>)}
            </select><Icon name="ChevronDown" size={13} /></label>}
        </div>
      </div>
      {error ? <div role="alert"><EmptyState icon="AlertCircle" title="读取任务记录失败" description={error}>
        <button className="button" onClick={() => setRevision(n => n + 1)}>重试</button>
      </EmptyState></div> :
        visible && !visible.items.length ? <EmptyState icon={queue ? 'ClipboardCheck' : 'Inbox'}
          title={filtered ? '没有匹配的任务' : queue ? '当前没有待验收任务' : running ? '当前没有执行中任务' : '暂无任务'}>
          {filtered && <button className="button" onClick={() => { setInput(''); setQuery(''); changeFilter('') }}>清除筛选</button>}
        </EmptyState> : <div className="table-scroll"><table className="task-table">
          <thead><tr><th scope="col">任务 / 来源</th><th scope="col">状态</th><th scope="col" className="worker-cell">Worker</th><th scope="col" className="time-cell">最近更新</th><th scope="col" aria-label="进入详情" /></tr></thead>
          <tbody>{visible ? visible.items.map(t => <tr key={t.id}>
            <td><a className="task-title" href={'#/tasks/' + t.id}><span className={'task-glyph ' + (t.kind === 'codex_issue' ? 'issue' : '')}>
              <Icon name={t.kind === 'codex_issue' ? 'Circle' : 'Terminal'} size={16}/></span><span>{t.title}</span></a>
              <div className="task-sub"><span className="mono">T-{t.id}</span><span className="sub-divider">/</span>
                <span>{t.repository || '本地项目 ' + t.project_id}{t.issue_number ? ' #' + t.issue_number : ''}</span></div></td>
            <td><Status value={t.status}/></td>
            <td className="worker-cell"><span className="worker-label">{t.worker && <Icon name="Terminal" size={13}/>} {t.worker || '—'}</span></td>
            <td className="time-cell"><time dateTime={t.updated_at || undefined}>{t.updated_at ? formatTime(t.updated_at) : '—'}</time></td>
            <td><a className="row-link" href={'#/tasks/' + t.id} aria-label={'查看任务 ' + t.id}><Icon name="ChevronRight" size={16}/></a></td>
          </tr>) : <tr><td colSpan={5} className="loading-row">正在读取任务…</td></tr>}</tbody>
        </table></div>}
      <div className="table-foot"><span aria-live="polite">{error ? '读取失败' : visible ? '本页 ' + visible.items.length + ' 项' : '正在读取…'}</span>
        <div><button className="pager" aria-label="上一页" disabled={!cursors.length || loading} onClick={() => setCursors(prev => prev.slice(0, -1))}><Icon name="ChevronRight" className="flip" size={15}/></button>
        <span>第 {cursors.length + 1} 页</span><button className="pager" aria-label="下一页" disabled={!visible?.next_before || loading} onClick={() => { if (visible?.next_before) setCursors(prev => [...prev, visible.next_before!]) }}><Icon name="ChevronRight" size={15}/></button></div>
      </div>
    </section>
  </>
}
