import { useEffect, useState } from 'react'
import { formatTime, Icon, Status, statusLabels, type ApiRequest } from './shared'

type Summary = {
  id: number; project_id: number; kind: string; status: string; title: string; repository: string
  issue_number?: number; pr_number?: number; worker?: string; updated_at: string | null
}
type Page = { items: Summary[]; counts: Record<string, number>; next_before?: number }
const filters = ['PLANNING', 'ACTIVE', 'BLOCKED', 'READY', 'RUNNING', 'WAITING_INPUT', 'WAITING_ACCEPTANCE', 'ACCEPTED', 'DONE', 'FAILED']
const metrics = [
  { key: 'ALL', name: '全部任务', icon: 'Layers3', color: 'blue' },
  { key: 'RUNNING', name: '执行中', icon: 'Activity', color: 'cyan' },
  { key: 'WAITING_ACCEPTANCE', name: '待验收', icon: 'ClipboardCheck', color: 'amber' },
  { key: 'DONE', name: '已完成', icon: 'CheckCircle2', color: 'green' },
] as const

export default function TaskList({ request, queue, onCounts }: {
  request: ApiRequest; queue: boolean; onCounts: (counts: Record<string, number>) => void
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
  const status = queue ? 'WAITING_ACCEPTANCE' : filter
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
      setSynced(new Date().toLocaleTimeString('zh-CN', { hour12: false }))
    }).catch(err => { if (!controller.signal.aborted) setError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [path, request, revision, onCounts])

  function changeFilter(value: string) { setFilter(value); setCursors([]) }
  const visible = lastRequest === path ? page : null
  return <>
    <div className="page-heading"><div><h1>{queue ? '验收队列' : '任务列表'}</h1>
      <p>{synced ? '最近同步 ' + synced : '正在读取控制面记录…'}</p></div>
      <button className="button refresh" onClick={() => setRevision(n => n + 1)} disabled={loading}>
        <Icon name="RefreshCw" size={15} />刷新
      </button>
    </div>
    {!queue && <div className="metrics" aria-label="任务统计">{metrics.map(m =>
      <button className={'metric ' + m.color} key={m.key} onClick={() => changeFilter(m.key === 'ALL' ? '' : m.key)}
        aria-pressed={(filter || 'ALL') === m.key}>
        <span className="metric-icon"><Icon name={m.icon} size={19} /></span>
        <span className="metric-text"><span>{m.name}</span><strong>{page?.counts[m.key] ?? '—'}</strong></span>
      </button>)}</div>}
    <section className="panel task-panel" aria-label="任务记录" aria-busy={loading}>
      <div className="table-toolbar"><h2>{queue ? '待处理任务' : '任务记录'}</h2>
        <div className="tools">
          <label className="search"><Icon name="Search" size={16} /><input type="search" aria-label="搜索任务"
            placeholder="搜索任务、仓库或编号" maxLength={120} value={input} onChange={e => setInput(e.target.value)} /></label>
          {!queue && <label className="filter"><Icon name="SlidersHorizontal" size={15} />
            <select aria-label="任务状态" value={filter} onChange={e => changeFilter(e.target.value)}>
              <option value="">全部状态</option>{filters.map(key => <option key={key} value={key}>{statusLabels[key]}</option>)}
            </select><Icon name="ChevronDown" size={13} /></label>}
        </div>
      </div>
      {error ? <div role="alert" className="table-message"><Icon name="AlertCircle"/><p>{error}</p><button className="button" onClick={() => setRevision(n => n + 1)}>重试</button></div> :
        visible && !visible.items.length ? <div className="table-message"><Icon name={queue ? 'ClipboardCheck' : 'Inbox'} size={28}/>
          <strong>{query || filter ? '没有匹配的任务' : queue ? '当前没有待验收任务' : '暂无任务'}</strong>
          {(query || filter) && <button className="button" onClick={() => { setInput(''); setQuery(''); changeFilter('') }}>清除筛选</button>}
        </div> : <div className="table-scroll"><table className="task-table">
          <thead><tr><th>任务 / 来源</th><th>状态</th><th className="worker-cell">Worker</th><th className="time-cell">最近更新</th><th aria-label="进入详情" /></tr></thead>
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
      <div className="table-foot"><span aria-live="polite">{visible ? '本页 ' + visible.items.length + ' 项' : '读取任务记录'}</span>
        <div><button className="pager" aria-label="上一页" disabled={!cursors.length || loading} onClick={() => setCursors(prev => prev.slice(0, -1))}><Icon name="ChevronRight" className="flip" size={15}/></button>
        <span>第 {cursors.length + 1} 页</span><button className="pager" aria-label="下一页" disabled={!visible?.next_before || loading} onClick={() => { if (visible?.next_before) setCursors(prev => [...prev, visible.next_before!]) }}><Icon name="ChevronRight" size={15}/></button></div>
      </div>
    </section>
  </>
}
