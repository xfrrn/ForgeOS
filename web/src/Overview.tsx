import { useEffect, useState, type ComponentProps, type ReactNode } from 'react'
import { approvalLabels, approvalPath, type ApprovalItem } from './Approvals'
import { noticeLabels, noticePath, type Notice } from './Notifications'
import { QueueReason, queueTaskHref, RoleQueueList, type ExecutionQueuePage } from './ProjectExecutionQueue'
import { formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { EmptyState, MetricCard, PageHeading } from './ui'

const queues = [
  { name: '全部任务', status: 'ALL', path: '/tasks', icon: 'Layers3' },
  { name: '执行中', status: 'RUNNING', path: '/tasks?status=RUNNING', icon: 'Activity' },
  { name: '待答问题', status: 'WAITING_INPUT', path: '/questions', icon: 'MessageSquare' },
  { name: '待验收', status: 'WAITING_ACCEPTANCE', path: '/reviews?tab=acceptance', icon: 'ClipboardCheck' },
] as const

const breakdown = [
  { label: '待推进', states: ['PLANNING', 'READY', 'BLOCKED', 'PAUSED'], tone: 'neutral' },
  { label: '正在推进', states: ['ACTIVE', 'RUNNING'], tone: 'info' },
  { label: '待人工处理', states: ['WAITING_INPUT', 'WAITING_ACCEPTANCE'], tone: 'warning' },
  { label: '已验收或完成', states: ['ACCEPTED', 'DONE'], tone: 'positive' },
  { label: '失败或停止', states: ['FAILED', 'SUPERSEDED', 'CANCELLED'], tone: 'danger' },
]

const noticeIcons = { question: 'MessageSquare', plan: 'ListChecks', acceptance: 'ClipboardCheck', failure: 'AlertCircle' } as const
const approvalIcons = { plan: 'ListChecks', decision: 'ClipboardCheck', share: 'GitBranch', question: 'MessageSquare', command: 'Terminal', patch: 'FileCode2' } as const

type Snapshot = { counts: Record<string, number>; notices: Notice[]; approvals: ApprovalItem[]; queue?: ExecutionQueuePage; at: string }

function QueueRow({ href, icon, title, children }: {
  href: string; icon: ComponentProps<typeof Icon>['name']; title: string; children: ReactNode
}) {
  return <li><a className="overview-row" href={href}>
    <span className="overview-row-icon"><Icon name={icon} size={16}/></span>
    <span className="overview-row-copy">
      <span className="overview-row-title">{title}</span>
      <span className="overview-row-meta">{children}</span>
    </span>
    <Icon name="ChevronRight" className="overview-row-arrow" size={16}/>
  </a></li>
}

export function Overview({projectId, request}: {projectId: number; request: ApiRequest}) {
  const [data, setData] = useState<Snapshot>()
  const [error, setError] = useState('')
  const [queueError, setQueueError] = useState('')
  const [revision, setRevision] = useState(0)
  const [refreshing, setRefreshing] = useState(true)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const options = {signal: stop.signal}
    async function load() {
      try {
        const [tasks, notices, approvals, queue] = await Promise.all([
          request<{counts: Record<string, number>}>(`/tasks?limit=1&project_id=${projectId}`, options),
          request<{items: Notice[]}>(`/tasks/notifications?scope=pending&project_id=${projectId}`, options),
          request<{items: ApprovalItem[]}>(`/tasks/approvals?include_questions=1&include_commands=1&include_patches=1&project_id=${projectId}`, options),
          request<ExecutionQueuePage>(`/projects/${projectId}/execution-queue`, options).then(result => {
            if (!stop.signal.aborted) setQueueError('')
            return result
          }).catch(cause => {
            if (!stop.signal.aborted) setQueueError(cause instanceof Error ? cause.message : '无法读取执行队列。')
            return undefined
          }),
        ])
        if (stop.signal.aborted) return
        setData({ counts: tasks.counts, notices: notices.items.slice(0, 5), approvals: approvals.items.slice(0, 5), queue, at: new Date().toISOString() })
        setError('')
      } catch (e) {
        if (!stop.signal.aborted) setError(e instanceof Error ? e.message : '读取项目概览失败。')
      }
      if (!stop.signal.aborted) {
        setRefreshing(false)
        timer = setTimeout(load, 8000)
      }
    }
    void load()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [projectId, request, revision])
  function refresh() {
    setRefreshing(true)
    setRevision(n => n + 1)
  }

  const total = data?.counts.ALL ?? 0
  const blockers = data?.queue?.items.filter(item => ['BLOCKED', 'WAITING_INPUT', 'PAUSED'].includes(item.status) || item.lease_expired).slice(0, 4) ?? []
  return <>
    <PageHeading title="项目概览"
      description={data ? <>最近同步 <time dateTime={data.at}>{formatTime(data.at)}</time></> : undefined}
      actions={<button className="button" disabled={refreshing} onClick={refresh}>
        <Icon name="RefreshCw" size={15}/>{refreshing ? '刷新中…' : '刷新'}
      </button>}/>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    <nav className="metrics" aria-label="任务状态">
      {queues.map(q => <MetricCard key={q.status} label={q.name} value={data ? data.counts[q.status] ?? 0 : '—'}
        icon={q.icon} href={projectHref(projectId, q.path)}/>)}
    </nav>
    {!data && !error && <div className="panel table-message" role="status">正在读取项目概览…</div>}
    {data && <div className="overview-grid">
      <div className="overview-stack">
        <section className="panel overview-card" aria-labelledby="breakdown-heading">
          <div className="overview-heading">
            <div><h2 id="breakdown-heading">任务状态分布</h2><p>按当前状态统计，共 {total} 项任务</p></div>
            <a className="overview-link" href={projectHref(projectId, '/tasks')}>任务列表<Icon name="ChevronRight" size={14}/></a>
          </div>
          <ul className="task-breakdown">
            {breakdown.map(group => {
              const count = group.states.reduce((sum, status) => sum + (data.counts[status] ?? 0), 0)
              return <li className="task-breakdown-row" data-tone={group.tone} key={group.label}>
                <span className="task-breakdown-label">{group.label}</span>
                <meter className="task-breakdown-track" min={0} max={Math.max(total, 1)} value={count} aria-hidden="true"/>
                <span className="task-breakdown-value">{count}</span>
              </li>
            })}
          </ul>
        </section>
        <section className="panel overview-card" aria-labelledby="blockers-heading">
          <div className="overview-heading">
            <h2 id="blockers-heading">主要阻塞</h2>
            <a className="overview-link" href={projectHref(projectId, '/tasks?tab=queue')}>执行队列<Icon name="ChevronRight" size={14}/></a>
          </div>
          {queueError ? <p className="detail-problem" role="alert">{queueError}</p> : blockers.length === 0 ?
            <EmptyState icon="Activity" title="暂无等待依赖、人工回答或暂停的任务"/> : <ul className="overview-list">{blockers.map(item =>
              <li key={item.task_id} className="registered-project"><div>
                <a className="task-title" href={queueTaskHref(projectId, item)}>{item.title}</a>
                <p className="muted">T-{item.task_id} · <QueueReason projectId={projectId} item={item}/></p>
              </div></li>)}</ul>}
        </section>
        <section className="panel overview-card" aria-labelledby="notices-heading">
          <div className="overview-heading">
            <h2 id="notices-heading">待处理通知</h2>
            <a className="overview-link" href={projectHref(projectId, '/notifications')}>全部通知<Icon name="ChevronRight" size={14}/></a>
          </div>
          {data.notices.length === 0 ? <EmptyState icon="Inbox" title="当前没有待处理通知"/> :
            <ul className="overview-list">{data.notices.map(n => <QueueRow key={n.id} href={noticePath(n)} icon={noticeIcons[n.kind]} title={n.title}>
              {noticeLabels[n.kind]} · T-{n.task_id} · <time dateTime={n.at}>{formatTime(n.at)}</time>
            </QueueRow>)}</ul>}
        </section>
      </div>
      <div className="overview-stack"><section className="panel overview-card" aria-labelledby="approvals-heading">
        <div className="overview-heading">
          <h2 id="approvals-heading">待你审批</h2>
          <a className="overview-link" href={projectHref(projectId, '/reviews')}>全部审批<Icon name="ChevronRight" size={14}/></a>
        </div>
        {data.approvals.length === 0 ? <EmptyState icon="ListChecks" title="当前没有待审批记录"/> :
          <ul className="overview-list">{data.approvals.map(a => <QueueRow key={a.kind + a.id} href={approvalPath(a)} icon={approvalIcons[a.kind]} title={a.title}>
            {approvalLabels[a.kind]}{a.revision ? ' · 修订 ' + a.revision : ''}
          </QueueRow>)}</ul>}
      </section>
      {data.queue && <section className="panel overview-card" aria-labelledby="roles-heading">
        <div className="overview-heading"><h2 id="roles-heading">Agent 分工进度</h2>
          <a className="overview-link" href={projectHref(projectId, '/tasks?tab=queue')}>查看队列<Icon name="ChevronRight" size={14}/></a>
        </div>
        <RoleQueueList projectId={projectId} roles={data.queue.roles}/>
      </section>}</div>
    </div>}
  </>
}
