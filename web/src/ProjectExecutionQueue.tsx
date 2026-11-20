import { useEffect, useState } from 'react'
import { formatTime, Icon, projectHref, Status, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import './project-work.css'

type QueueItem = {
  task_id: number; title: string; status: string; role: string; execution_id?: number; worker?: string
  updated_at?: string; lease_expired?: boolean; blocked_by?: number[]; held_by_task_id?: number
  question_id?: number; goal_on_hold?: boolean
}
type RoleQueue = { role: string; pending: number; running: number }
export type ExecutionQueuePage = { items: QueueItem[]; counts: Record<string, number>; roles: RoleQueue[] }

export const taskRoles: Record<string, string> = { planner: 'Planner', worker: 'Worker', ci: '本地 CI Runner' }

const roleDetails: Record<string, { label: string; path: string }> = {
  plan_review: { label: 'Management · 计划审核', path: '/reviews?tab=approvals' },
  code_review: { label: 'Review Agent · 候选代码审查', path: '/reviews?tab=code-reviews' },
  test_plan: { label: 'Test Agent · 生成测试计划', path: '/reviews?tab=checks' },
  test_coverage: { label: 'Management · 测试覆盖审核', path: '/reviews?tab=checks' },
  curator: { label: 'Curator · 提取项目经验', path: '/materials' },
  curation_review: { label: 'Management · 知识审核', path: '/materials' },
}

const lanes = [
  { status: 'RUNNING', title: '执行中' },
  { status: 'READY', title: '待领取' },
  { status: 'BLOCKED', title: '等待依赖' },
  { status: 'WAITING_INPUT', title: '等待人工回答' },
  { status: 'PAUSED', title: '已暂停' },
]

export function queueTaskHref(projectId: number, item: QueueItem) {
  const task = `/tasks/${item.task_id}`
  return projectHref(projectId, item.question_id ? `${task}/questions/${item.question_id}` :
    item.execution_id ? `${task}?execution=${item.execution_id}` : task)
}

export function QueueReason({ projectId, item }: { projectId: number; item: QueueItem }) {
  if (item.status === 'WAITING_INPUT') return <>等待 Owner 回答{item.question_id && `问题 #${item.question_id}`}</>
  if (item.lease_expired) return <>执行租约已到期，待核对</>
  if (item.status === 'PAUSED') return <>等待恢复执行</>
  if (item.status === 'BLOCKED') return item.blocked_by?.length ? <>
    等待前置任务交付 {item.blocked_by.map(id => <a key={id} href={projectHref(projectId, `/tasks/${id}`)}>T-{id}</a>)}
  </> : <>等待确认依赖交付</>
  if (item.goal_on_hold) return <>等待所属目标进入执行阶段</>
  if (item.held_by_task_id) return <>等待项目执行位 <a href={projectHref(projectId, `/tasks/${item.held_by_task_id}`)}>T-{item.held_by_task_id}</a> 正在占用</>
  if (item.status === 'READY') return <>等待匹配的执行节点和可用资源</>
  return null
}

export function RoleQueueList({ projectId, roles }: { projectId: number; roles: RoleQueue[] }) {
  return roles.length ? <ul className="overview-list">{roles.map(entry => {
    const detail = roleDetails[entry.role]
    if (!detail) return null
    return <li key={entry.role}><a className="overview-row" href={projectHref(projectId, detail.path)}>
      <span className="overview-row-icon"><Icon name="Activity" size={16}/></span>
      <span className="overview-row-copy"><span className="overview-row-title">{detail.label}</span>
        <span className="overview-row-meta">{entry.pending} 项待领取 · {entry.running} 项已领取</span></span>
      <Icon name="ChevronRight" className="overview-row-arrow" size={16}/>
    </a></li>
  })}</ul> : <p className="muted">目前没有待领取或已领取的审核、测试计划或知识作业。</p>
}

export function ProjectExecutionQueue({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const [page, setPage] = useState<ExecutionQueuePage>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [refreshing, setRefreshing] = useState(true)
  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const snapshot = await request<ExecutionQueuePage>(`/projects/${projectId}/execution-queue`, { signal: controller.signal })
        if (controller.signal.aborted) return
        setPage(snapshot)
        setError('')
      } catch (cause) {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '无法读取执行队列。')
      }
      if (!controller.signal.aborted) {
        setRefreshing(false)
        timer = setTimeout(read, 8000)
      }
    }
    void read()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [projectId, request, revision])
  function refresh() {
    setRefreshing(true)
    setRevision(n => n + 1)
  }
  return <>
    <PageHeading title="执行队列" actions={<button className="button" disabled={refreshing} onClick={refresh}>
      <Icon name="RefreshCw" size={15}/>{refreshing ? '刷新中…' : '刷新'}
    </button>}/>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {!page && !error && <p className="panel table-message" role="status">正在读取执行队列…</p>}
    {page && <div className="overview-grid">
      <div className="overview-stack">
        {page.items.length === 0 && <section className="panel"><EmptyState icon="ListChecks" title="本项目暂无待执行或正在执行的任务"/></section>}
        {lanes.map(lane => {
          const count = page.counts[lane.status] ?? 0
          if (!count) return null
          const items = page.items.filter(item => item.status === lane.status)
          return <section key={lane.status} className="panel overview-card" aria-labelledby={'queue-' + lane.status}>
            <div className="overview-heading"><h2 id={'queue-' + lane.status}>{lane.title} · {count}</h2>
              {count > items.length && <a className="overview-link" href={projectHref(projectId, `/tasks?status=${lane.status}`)}>全部 {count} 项<Icon name="ChevronRight" size={14}/></a>}
            </div>
            <table className="task-table project-work-table"><thead><tr><th scope="col">任务</th><th scope="col">分工与进度</th></tr></thead>
              <tbody>{items.map(item => <tr key={item.task_id}>
                <td><a className="task-title" href={queueTaskHref(projectId, item)}>{item.title}</a>
                  <small className="muted">T-{item.task_id}{item.execution_id ? ` · 执行 #${item.execution_id}` : ''}</small>
                </td>
                <td><Status value={item.status}/><div>{taskRoles[item.role]}{item.worker ? ` · ${item.worker}` : ''}</div>
                  <div className="muted"><QueueReason projectId={projectId} item={item}/></div>
                  {item.updated_at && <small className="muted"><time dateTime={item.updated_at}>{formatTime(item.updated_at)}</time></small>}
                </td>
              </tr>)}</tbody>
            </table>
          </section>
        })}
      </div>
      <section className="panel overview-card" aria-labelledby="role-queue-heading">
        <div className="overview-heading"><h2 id="role-queue-heading">Agent 审核、测试与知识作业</h2></div>
        <RoleQueueList projectId={projectId} roles={page.roles}/>
      </section>
    </div>}
  </>
}
