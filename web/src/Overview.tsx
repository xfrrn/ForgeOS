import { useEffect, useState, type ComponentProps, type ReactNode } from 'react'
import { approvalLabels, approvalPath, type ApprovalItem } from './Approvals'
import { noticeLabels, noticePath, type Notice } from './Notifications'
import { ProjectOverviewContext } from './ProjectOverviewContext'
import { ProjectPreview } from './ProjectPreview'
import { QueueReason, queueTaskHref, RoleQueueList, taskRoles, type ExecutionQueuePage } from './ProjectExecutionQueue'
import { formatTime, Icon, projectHref, Status, type ApiRequest } from './shared'
import { MetricCard, PageHeading } from './ui'
import './runtime-status.css'

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

type Snapshot = { counts?: Record<string, number>; notices?: Notice[]; approvals?: ApprovalItem[]; queue?: ExecutionQueuePage; at?: string }
type Source = 'counts' | 'notices' | 'approvals' | 'queue'

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

function approvalCoversNotice(notice: Notice, approvals: ApprovalItem[]) {
  return approvals.some(approval =>
    notice.kind === 'question' && approval.kind === 'question' && approval.task_id === notice.task_id ||
    notice.kind === 'plan' && approval.kind === 'plan' && approval.id === notice.task_id && approval.revision === notice.plan_revision)
}

export function Overview({projectId, request}: {projectId: number; request: ApiRequest}) {
  const [data, setData] = useState<Snapshot>({})
  const [errors, setErrors] = useState<Partial<Record<Source, string>>>({})
  const [revision, setRevision] = useState(0)
  const [refreshing, setRefreshing] = useState(true)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const options = {signal: stop.signal}
    async function load() {
      const [tasks, notices, approvals, queue] = await Promise.allSettled([
        request<{counts: Record<string, number>}>(`/tasks?limit=1&project_id=${projectId}`, options),
        request<{items: Notice[]}>(`/tasks/notifications?scope=pending&project_id=${projectId}`, options),
        request<{items: ApprovalItem[]}>(`/tasks/approvals?include_questions=1&include_commands=1&include_patches=1&project_id=${projectId}`, options),
        request<ExecutionQueuePage>(`/projects/${projectId}/execution-queue`, options),
      ])
      if (stop.signal.aborted) return
      const sources = { counts: tasks, notices, approvals, queue }
      const messages = {counts: '任务计数刷新失败', notices: '通知刷新失败', approvals: '审批刷新失败', queue: '执行队列刷新失败'}
      const failures: Partial<Record<Source, string>> = {}
      for (const source of Object.keys(sources) as Source[]) {
        if (sources[source].status === 'rejected') failures[source] = messages[source]
      }
      setErrors(failures)
      setData(previous => ({
        counts: tasks.status === 'fulfilled' ? tasks.value.counts : previous.counts,
        notices: notices.status === 'fulfilled' ? notices.value.items : previous.notices,
        approvals: approvals.status === 'fulfilled' ? approvals.value.items : previous.approvals,
        queue: queue.status === 'fulfilled' ? queue.value : previous.queue,
        at: Object.keys(failures).length === 0 ? new Date().toISOString() : previous.at,
      }))
      setRefreshing(false)
      timer = setTimeout(load, 8000)
    }
    void load()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [projectId, request, revision])

  const total = data.counts?.ALL ?? 0
  const items = data.queue?.items ?? []
  const running = items.filter(item => item.status === 'RUNNING' && !item.lease_expired).slice(0, 3)
  const blocked = items.filter(item => item.lease_expired || item.status === 'BLOCKED' || item.status === 'PAUSED' ||
    item.status === 'WAITING_INPUT' || (item.status === 'READY' && (item.held_by_task_id || item.goal_on_hold))).slice(0, 3)
  const approvals = data.approvals?.slice(0, 3) ?? []
  const notices = data.notices?.filter(n => !approvalCoversNotice(n, approvals)).slice(0, 5 - approvals.length) ?? []
  const hasTodo = approvals.length > 0 || notices.length > 0
  const queueHref = projectHref(projectId, '/tasks?tab=queue')
  return <>
    <PageHeading title="项目概览"
      description={data.at ? <>最近完整同步 <time dateTime={data.at}>{formatTime(data.at)}</time></> : undefined}
      actions={<button className="button" disabled={refreshing} onClick={() => { setRefreshing(true); setRevision(n => n + 1) }}>
        <Icon name="RefreshCw" size={15}/>{refreshing ? '刷新中…' : '刷新'}
      </button>}/>
    <ProjectPreview key={projectId} projectId={projectId} request={request}/>
    {errors.counts && <p className="detail-problem" role="alert">{errors.counts}{data.counts ? '，显示上次结果。' : '。'}</p>}
    <nav className="metrics" aria-label="任务状态">
      {queues.map(q => <MetricCard key={q.status} label={q.name} value={data.counts ? data.counts[q.status] ?? 0 : '—'}
        icon={q.icon} href={projectHref(projectId, q.path)}/>)}
    </nav>
    <div className="overview-grid project-overview-work">
      <section className="panel overview-card" aria-labelledby="todo-heading">
        <div className="overview-heading">
          <h2 id="todo-heading">需要你处理</h2>
          <a className="overview-link" href={projectHref(projectId, '/reviews')}>审核与验收<Icon name="ChevronRight" size={14}/></a>
        </div>
        {(errors.approvals || errors.notices) && <p className="detail-problem" role="alert">
          {[errors.approvals, errors.notices].filter(Boolean).join('、')}{(errors.approvals && data.approvals || errors.notices && data.notices) ? '，保留上次收到的待办。' : '。'}
        </p>}
        {hasTodo ? <ul className="overview-list">
          {approvals.map(a => <QueueRow key={a.kind + a.id} href={approvalPath(a)} icon={approvalIcons[a.kind]} title={a.title}>
            {approvalLabels[a.kind]}{a.task_id ? ' · T-' + a.task_id : ''}{a.revision ? ' · 修订 ' + a.revision : ''}
          </QueueRow>)}
          {notices.map(n => <QueueRow key={n.id} href={noticePath(n)} icon={noticeIcons[n.kind]} title={n.title}>
            {noticeLabels[n.kind]} · T-{n.task_id} · <time dateTime={n.at}>{formatTime(n.at)}</time>
          </QueueRow>)}
        </ul> : !errors.approvals && !errors.notices && <p className="muted" role={data.approvals && data.notices ? undefined : 'status'}>
          {data.approvals && data.notices ? '当前没有待办' : '正在读取待办…'}
        </p>}
        <a className="overview-link" href={projectHref(projectId, '/notifications')}>项目通知<Icon name="ChevronRight" size={14}/></a>
      </section>
      <section className="panel overview-card" aria-labelledby="execution-heading">
        <div className="overview-heading">
          <h2 id="execution-heading">当前执行与阻塞</h2>
          <a className="overview-link" href={queueHref}>执行队列<Icon name="ChevronRight" size={14}/></a>
        </div>
        {errors.queue && <p className="detail-problem" role="alert">{errors.queue}{data.queue ? '，显示上次结果。' : '。'}</p>}
        {data.queue ? <>
          <p className="muted project-overview-queue-count">{data.queue.counts.RUNNING ?? 0} 项执行中 · {data.queue.counts.READY ?? 0} 项待领取
            {(data.queue.counts.WAITING_INPUT ?? 0) > 0 && <> · {data.queue.counts.WAITING_INPUT} 项等待回答</>}
          </p>
          {running.length + blocked.length > 0 ? <ul className="project-overview-rows">
            {[...blocked, ...running].map(item => <li key={item.task_id}>
              <div className="project-overview-row">
                <a className="task-title" href={queueTaskHref(projectId, item)}>{item.title}</a>
                {item.lease_expired ? <span className="runtime-warning">租约已到期</span> : <Status value={item.status}/>}
              </div>
              <p className="project-overview-row-meta">
                T-{item.task_id} · {taskRoles[item.role] ?? item.role}{item.worker && <> · {item.worker}</>}
                {item.status === 'RUNNING' && !item.lease_expired && item.updated_at &&
                  <> · 更新于 <time dateTime={item.updated_at}>{formatTime(item.updated_at)}</time></>}
              </p>
              {(item.status !== 'RUNNING' || item.lease_expired) &&
                <p className="project-overview-row-meta"><QueueReason projectId={projectId} item={item}/></p>}
            </li>)}
          </ul> : <p className="muted">{data.queue.counts.READY ? '待领取任务正在等待匹配节点与资源。' : '当前没有执行中或需核对的阻塞任务。'}</p>}
        </> : !errors.queue && <p className="muted" role="status">正在读取执行队列…</p>}
      </section>
    </div>
    <ProjectOverviewContext projectId={projectId} request={request} revision={revision}/>
    {(data.counts || data.queue) && <div className="overview-grid">
      {data.counts && <section className="panel overview-card" aria-labelledby="breakdown-heading">
        <div className="overview-heading">
          <div><h2 id="breakdown-heading">任务状态分布</h2><p>按当前状态统计，共 {total} 项任务</p></div>
          <a className="overview-link" href={projectHref(projectId, '/tasks')}>任务列表<Icon name="ChevronRight" size={14}/></a>
        </div>
        <ul className="task-breakdown">
          {breakdown.map(group => {
            const count = group.states.reduce((sum, status) => sum + (data.counts?.[status] ?? 0), 0)
            return <li className="task-breakdown-row" data-tone={group.tone} key={group.label}>
              <span className="task-breakdown-label">{group.label}</span>
              <meter className="task-breakdown-track" min={0} max={Math.max(total, 1)} value={count} aria-hidden="true"/>
              <span className="task-breakdown-value">{count}</span>
            </li>
          })}
        </ul>
      </section>}
      {data.queue && <section className="panel overview-card" aria-labelledby="roles-heading">
        <div className="overview-heading"><h2 id="roles-heading">Agent 分工进度</h2>
          <a className="overview-link" href={queueHref}>查看队列<Icon name="ChevronRight" size={14}/></a>
        </div>
        <RoleQueueList projectId={projectId} roles={data.queue.roles}/>
      </section>}
    </div>}
  </>
}
