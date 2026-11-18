import { useEffect, useState } from 'react'
import { approvalLabels, approvalPath, type ApprovalItem } from './Approvals'
import { noticeLabels, noticePath, type Notice } from './Notifications'
import { formatTime, Icon, type ApiRequest } from './shared'

const queues = [
  { name: '全部任务', status: 'ALL', path: '/tasks', icon: 'Layers3', color: 'blue' },
  { name: '执行中', status: 'RUNNING', path: '/tasks?status=RUNNING', icon: 'Activity', color: 'cyan' },
  { name: '待答问题', status: 'WAITING_INPUT', path: '/questions', icon: 'MessageSquare', color: 'amber' },
  { name: '待验收', status: 'WAITING_ACCEPTANCE', path: '/review', icon: 'ClipboardCheck', color: 'green' },
] as const

type Snapshot = { counts: Record<string, number>; notices: Notice[]; approvals: ApprovalItem[]; at: string }

export function Overview({request, onCounts}: {request: ApiRequest; onCounts: (counts: Record<string, number>) => void}) {
  const [data, setData] = useState<Snapshot>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const options = {signal: stop.signal}
    async function load() {
      try {
        const [tasks, notices, approvals] = await Promise.all([
          request<{counts: Record<string, number>}>('/tasks?limit=1', options),
          request<{items: Notice[]}>('/tasks/notifications?scope=pending', options),
          request<{items: ApprovalItem[]}>('/tasks/approvals?include_questions=1&include_commands=1', options),
        ])
        if (stop.signal.aborted) return
        setData({ counts: tasks.counts, notices: notices.items.slice(0, 5), approvals: approvals.items.slice(0, 5), at: new Date().toISOString() })
        onCounts(tasks.counts); setError('')
      } catch (e) {
        if (!stop.signal.aborted) setError(e instanceof Error ? e.message : '读取工作台总览失败。')
      }
      if (!stop.signal.aborted) timer = setTimeout(load, 8000)
    }
    void load()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request, onCounts, revision])
  return <>
    <div className="page-heading"><div><h1>工作台总览</h1>
      {data && <p>最近同步 <time dateTime={data.at}>{formatTime(data.at)}</time></p>}
    </div><button className="button" onClick={() => setRevision(n => n + 1)}><Icon name="RefreshCw" size={15}/>刷新</button></div>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {!data && !error && <p role="status">正在读取工作台…</p>}
    {data && <>
      <nav className="metrics" aria-label="任务状态">{queues.map(q => <a key={q.status} className={'metric ' + q.color} href={'#' + q.path}>
        <span className="metric-icon"><Icon name={q.icon} size={19}/></span>
        <span className="metric-text"><span>{q.name}</span><strong>{data.counts[q.status]}</strong></span>
      </a>)}</nav>
      <div className="overview-panels">
        <section className="panel detail-card"><div className="overview-heading"><h2>待处理通知</h2><a href="#/notifications">全部通知</a></div>
          {data.notices.length === 0 && <p>当前没有待处理通知。</p>}
          {data.notices.map(n => <article key={n.id} className="notification-row">
            <a href={noticePath(n)}>{n.title}</a>
            <p>{noticeLabels[n.kind]} · 项目 #{n.project_id} · T-{n.task_id} · <time dateTime={n.at}>{formatTime(n.at)}</time></p>
          </article>)}
        </section>
        <section className="panel detail-card"><div className="overview-heading"><h2>待审批记录</h2><a href="#/approvals">审批中心</a></div>
          {data.approvals.length === 0 && <p>当前没有待审批记录。</p>}
          {data.approvals.map(a => <article key={a.kind + a.id} className="notification-row">
            <a href={approvalPath(a)}>{a.title}</a><p>项目 #{a.project_id} · {approvalLabels[a.kind]}{a.revision && ' · 修订 ' + a.revision}</p>
          </article>)}
        </section>
      </div>
    </>}
  </>
}
