import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ApiError, Icon, Status, formatTime, safeGitHubURL, type ApiRequest } from './shared'
import './detail.css'

type Validation = { profile: string; commit: string; exit_code: number; output: string }
type Result = { commit?: string; base_commit?: string; session_id?: string; exit_code: number; error?: string; validation?: Validation; plan?: { objective: string } }
type Attempt = {
  id: number; status: string; worker: string; workspace: string; lease_until: string
  lease_expired: boolean; abandoned_at?: string; end_reason?: string; result?: Result
}
type Issue = { title: string; number: number; body: string; url: string; repository: string; updated_at: string }
type Review = { execution_id: number; commit: string; decision: string; note: string; actor: string; at: string }
type Delivery = { state: string; detail?: string; branch: string; target_branch: string; pr_url?: string; pr_number?: number; merge_sha?: string }
type TaskEvent = { id: number; execution_id?: number; type: string; at: string }
type Question = { id: number; execution_id: number; commit_sha: string; question: string; evidence: string; answer?: string }
type PlanItem = { key: string; objective: string; scope: string; acceptance_criteria: string; deliverables: string; dependencies?: string[] }
type Plan = { revision: number; plan: { objective: string; milestones: { name: string; items: PlanItem[] | null }[] } }
type Task = {
  id: number; project_id: number; status: string; kind: string
  spec?: { issue: Issue; base_commit: string; validation: string }
  execution: Attempt | null; attempts: Attempt[]; events: TaskEvent[]; reviews?: Review[]; delivery?: Delivery
  plan?: Plan
  goal_id?: number; plan_revision?: number
  children?: { id: number; key: string; status: string }[]
  questions?: Question[]
}
type Props = { id: number; request: ApiRequest; onChanged: () => void }
type Diff = { diff: string; truncated: boolean }

const events: Record<string, string> = {
  plan_revised: '已保存计划版本',
  plan_activated: '计划已生效', goal_item_created: '从计划创建工作项',
  dependencies_delivered: '前置交付已确认',
  decision_requested: '已请求业务决定', decision_answered: 'Owner 已回答',
  goal_delivered: '全部子项已交付，父 Issue 已关闭',
  planner_completed: '只读 Planner 已提交草案',
  task_created: '创建任务', issue_imported: '导入 GitHub Issue', execution_claimed: 'Worker 领取任务',
  execution_completed: '执行完成', execution_failed: '执行失败', execution_abandoned: '结束失联执行',
  execution_retry_requested: '请求重新执行', acceptance_approved: '候选版本通过验收',
  acceptance_rejected: '候选版本已退回', pr_created: '已创建 PR', pr_merged: 'PR 已合并', delivery_completed: '交付完成',
}
const deliveryStates: Record<string, string> = { PENDING: '准备交付', WAITING: '等待合并', MERGED: '已合并 · 等待回写', DONE: '已交付' }
const fullSHA = /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/

function OutLink({ href, children }: { href?: string; children: ReactNode }) {
  const url = safeGitHubURL(href)
  return url ? <a className="detail-link" href={url} target="_blank" rel="noopener noreferrer">{children}<Icon name="ExternalLink" size={13} /></a> : <span>{children}</span>
}

function Fact({ label, children, code = false }: { label: string; children: ReactNode; code?: boolean }) {
  return <div className="detail-fact"><dt>{label}</dt><dd className={code ? 'mono' : undefined}>{children}</dd></div>
}

function CandidateDiff({ taskID, attempt, request }: { taskID: number; attempt: Attempt; request: ApiRequest }) {
  const commit = attempt.result?.commit
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState<Diff | null>(null)
  const [error, setError] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    if (!open || !commit || value) return
    const controller = new AbortController()
    setError(false)
    request<Diff>(`/tasks/${taskID}/diff?execution_id=${attempt.id}&commit=${encodeURIComponent(commit)}`, { signal: controller.signal })
      .then(data => { if (!controller.signal.aborted) setValue(data) })
      .catch(() => { if (!controller.signal.aborted) setError(true) })
    return () => controller.abort()
  }, [open, value, request, taskID, attempt.id, commit, retry])
  if (!commit || !fullSHA.test(commit)) return null
  return <div className="detail-diff">
    <button type="button" className="detail-disclosure" aria-expanded={open} onClick={() => setOpen(!open)}>
      <Icon name="FileCode2" size={16} />{open ? '收起代码差异' : '查看代码差异'}<Icon name="ChevronRight" size={14} />
    </button>
    {open && (error ? <p role="alert">无法读取这次执行的差异。 <button className="detail-text-button" onClick={() => setRetry(retry + 1)}>重试</button></p>
      : !value ? <p role="status" className="muted">正在读取差异…</p>
      : <>{value.diff ? <pre className="detail-patch" tabIndex={0} aria-label="候选提交的代码差异">{value.diff.split('\n').map((line, index) =>
        <span key={index} className={line.startsWith('+') ? 'patch-add' : line.startsWith('-') ? 'patch-del' : line.startsWith('@@') ? 'patch-hunk' : undefined}>{line}{'\n'}</span>
      )}</pre> : <p className="muted">此提交没有可显示的文本差异。</p>}
        {value.truncated && <p className="detail-warning">差异已截断。完整内容可在此执行的 Worktree 中核对。</p>}</>)}
  </div>
}

function Execution({ attempt, task, latest, request }: { attempt: Attempt; task: Task; latest: boolean; request: ApiRequest }) {
  const result = attempt.result
  const validation = result?.validation
  const start = task.events.find(event => event.execution_id === attempt.id && event.type === 'execution_claimed')
  return <details className="detail-attempt" open={latest}>
    <summary><Icon name="ChevronRight" size={15} /><strong>执行 #{attempt.id}</strong><span className="detail-worker mono">{attempt.worker}</span><Status value={attempt.status} /></summary>
    <div className="detail-attempt-body"><dl className="detail-facts">
      {start && <Fact label="领取时间">{formatTime(start.at)}</Fact>}
      <Fact label="Worktree" code>{attempt.workspace}</Fact>
      {result?.session_id && <Fact label="Codex Session" code>{result.session_id}</Fact>}
      {result?.base_commit && <Fact label="基础 SHA" code>{result.base_commit}</Fact>}
      {result?.commit && <Fact label={task.spec ? '候选 SHA' : '提交 SHA'} code>{result.commit}</Fact>}
      {result && <Fact label="执行退出码" code>{result.exit_code}</Fact>}
      {attempt.status === 'RUNNING' && <Fact label="租约">{attempt.lease_expired ? '已过期 · 等待核对' : formatTime(attempt.lease_until)}</Fact>}
      {attempt.abandoned_at && <Fact label="人工结束">{formatTime(attempt.abandoned_at)}</Fact>}
    </dl>
    {result?.error && <p className="detail-problem" role="status">{result.error}</p>}
    {validation ? <section className={'detail-validation ' + (validation.exit_code === 0 ? 'passed' : 'failed')}>
      <div><Icon name={validation.exit_code === 0 ? 'CheckCircle2' : 'AlertCircle'} size={16} />
        <strong>{validation.exit_code === 0 ? '本地验证通过' : '本地验证失败'}</strong><code>{validation.profile}</code>
      </div>
      <dl className="detail-facts"><Fact label="验证 SHA" code>{validation.commit}</Fact><Fact label="验证退出码" code>{validation.exit_code}</Fact></dl>
      {validation.output && <pre>{validation.output}</pre>}
    </section> : result?.plan ? <p>规划结果 · {result.plan.objective}</p> : task.spec && <p className="detail-empty-text">此执行尚无本地验证结果。</p>}
    {task.spec && <CandidateDiff key={result?.commit} taskID={task.id} attempt={attempt} request={request} />}
    </div>
  </details>
}

function Acceptance({ taskID, attempt, request, onUpdate, refresh, locked }: {
  taskID: number; attempt: Attempt; request: ApiRequest; onUpdate: (task: Task) => void; refresh: (message: string) => void; locked: boolean
}) {
  const commit = attempt.result!.commit!
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const noteField = useRef<HTMLTextAreaElement>(null)
  const pending = useRef(false)
  const live = useRef(true)
  useEffect(() => { live.current = true; return () => { live.current = false } }, [])
  async function decide(decision: 'approve' | 'reject') {
    if (pending.current || locked) return
    const feedback = note.trim()
    if ((decision === 'reject' && !feedback) || new TextEncoder().encode(feedback).length > 4096) {
      setError(feedback ? '验收说明最多 4096 字节，请缩短说明。' : '请填写退回原因，便于 Worker 继续修订。')
      noteField.current?.focus()
      return
    }
    pending.current = true
    setBusy(true)
    setError('')
    try {
      const updated = await request<Task>(`/tasks/${taskID}/acceptance`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ execution_id: attempt.id, commit, decision, note: feedback }),
      })
      if (live.current) onUpdate(updated)
    } catch (cause) {
      if (live.current) refresh(cause instanceof ApiError && cause.status === 409
        ? '候选版本或验收记录已变更。请核对刷新后的记录。'
        : '尚未确认本次提交的结果。请核对刷新后的验收记录。')
    } finally {
      pending.current = false
      if (live.current) setBusy(false)
    }
  }
  return <section className="panel detail-card detail-accept">
    <h2><Icon name="ListChecks" size={18} />候选验收</h2>
    <div className="detail-target"><span>执行 #{attempt.id} · 候选 SHA</span><code>{commit}</code></div>
    <p className="detail-approval-hint">已启用 GitHub 交付的项目会在批准后自动创建 PR。</p>
    <label htmlFor="acceptance-note">验收说明</label>
    <textarea id="acceptance-note" ref={noteField} value={note} disabled={busy} placeholder="退回时请说明需要修改的内容" onChange={e => { setNote(e.target.value); setError('') }} />
    {error && <p className="detail-problem" role="alert">{error}</p>}
    <div className="detail-actions"><button className="button primary" disabled={busy || locked} onClick={() => decide('approve')}><Icon name="Check" size={16} />通过验收</button>
      <button className="button" disabled={busy || locked} onClick={() => decide('reject')}>退回修改</button></div>
    {busy && <p role="status">正在提交验收决定…</p>}
  </section>
}

function Decision({ taskID, q, request, onAnswer }: { taskID: number; q: Question; request: ApiRequest; onAnswer: () => void }) {
  const [answer, setAnswer] = useState('')
  const [status, setStatus] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    try {
      await request(`/tasks/${taskID}/questions/${q.id}/answer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ answer }) })
      onAnswer()
    } catch (error) { setStatus(error instanceof Error ? error.message : '提交失败，请重试。') }
    finally { setBusy(false) }
  }
  return <section className="panel detail-card detail-accept">
    <h2>业务决定 #{q.id}</h2><p>{q.question}</p>
    <dl><Fact label="依据">{q.evidence}</Fact><Fact label={`执行 #${q.execution_id}`} code>{q.commit_sha}</Fact></dl>
    {q.answer ? <p>Owner 答复 · {q.answer}</p> : <form onSubmit={e => { e.preventDefault(); void submit() }}>
      <label htmlFor={`answer-${q.id}`}>业务答复</label>
      <textarea id={`answer-${q.id}`} required value={answer} onChange={e => setAnswer(e.target.value)}/>
      <button className="button primary" disabled={busy || !answer.trim()}>提交答复</button>
      {status && <p role="alert">{status}</p>}
    </form>}
  </section>
}

function Detail({ id, request, onChanged }: Props) {
  const [task, setTask] = useState<Task | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  const generation = useRef(0)

  useEffect(() => {
    const epoch = ++generation.current
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const fresh = await request<Task>(`/tasks/${id}`, { signal: controller.signal })
        if (controller.signal.aborted || epoch !== generation.current) return
        setTask(fresh)
        setError('')
        if (['ACTIVE', 'BLOCKED', 'READY', 'RUNNING', 'WAITING_INPUT', 'WAITING_ACCEPTANCE'].includes(fresh.status) || (fresh.status === 'ACCEPTED' && fresh.delivery)) {
          timer = setTimeout(read, 8000)
        }
      } catch (cause) {
        if (controller.signal.aborted || epoch !== generation.current) return
        setError(cause instanceof ApiError && cause.status === 404 ? '找不到这项任务。' : '无法获取最新任务记录，请重新加载。')
      } finally {
        if (!controller.signal.aborted && epoch === generation.current) setLoading(false)
      }
    }
    void read()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [id, request, revision])

  function refresh(message = '') {
    generation.current++
    setNotice(message)
    setLoading(true)
    setRevision(current => current + 1)
  }
  function accepted(updated: Task) {
    setTask(updated)
    refresh(updated.reviews?.at(-1)?.decision === 'reject' ? '候选已退回，任务已重新排队。' : '已记录对该候选 SHA 的批准。')
    onChanged()
  }
  if (!task) return <div className="panel detail-placeholder" role="status">{error || '正在读取任务详情…'}
    {error && <button className="button" onClick={() => refresh()}>重新加载</button>}</div>
  const issue = task.spec?.issue
  const last = task.execution
  const result = last?.result
  const delivery = task.delivery
  const reviews = task.reviews ?? []
  const children = new Map(task.children?.map(child => [child.key, child]))
  const canReview = task.status === 'WAITING_ACCEPTANCE' && task.spec && last?.status === 'COMPLETED' &&
    result?.exit_code === 0 && result.commit && fullSHA.test(result.commit) && result.validation?.commit === result.commit &&
    result.validation.exit_code === 0 && !reviews.some(review => review.execution_id === last.id)
  return <article className="task-detail">
    <div className="detail-heading">
      <div className="detail-heading-meta"><span className="mono">T-{task.id}</span><Status value={task.status} /></div>
      <h1>{issue?.title || 'Git HEAD 检查'}</h1>
      {task.goal_id && <a href={`#/tasks/${task.goal_id}`}>Goal #{task.goal_id} · 修订 {task.plan_revision}</a>}
      <div className="detail-subtitle">{issue ? <><Icon name="GitBranch" size={15} /><span>{issue.repository}</span><span aria-hidden="true">/</span>
        <OutLink href={issue.url}>Issue #{issue.number}</OutLink></> : <span>项目 #{task.project_id} · git_head</span>}
        <button className="button detail-refresh" disabled={loading} onClick={() => refresh()}><Icon name="RefreshCw" size={15} />刷新</button>
      </div>
    </div>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {notice && <p className="detail-notice" role="status">{notice}</p>}
    <div className="detail-layout">
      <div className="detail-column">
        {task.plan && <section className="panel detail-card">
          <h2>Goal 计划 · 修订 {task.plan.revision}</h2>
          <p>{task.plan.plan.objective}</p>
          {task.plan.plan.milestones.map((milestone, index) => <div key={index}>
            <h3>{milestone.name}</h3>
            <ul>{milestone.items?.map(item => <li key={item.key}>
              {children.has(item.key) ? <a href={`#/tasks/${children.get(item.key)!.id}`}>{item.key} · {item.objective} <Status value={children.get(item.key)!.status}/></a>
                : <strong>{item.key} · {item.objective}</strong>}
              <dl><Fact label="范围">{item.scope}</Fact><Fact label="验收条件">{item.acceptance_criteria}</Fact>
                <Fact label="交付物">{item.deliverables}</Fact>
                {item.dependencies?.length ? <Fact label="依赖">{item.dependencies.join('、')}</Fact> : null}</dl>
            </li>)}</ul>
          </div>)}
        </section>}
        {issue && <section className="panel detail-card"><h2><Icon name="MessageSquare" size={18} />Issue 说明</h2>
          <div className="detail-issue-text" tabIndex={0} aria-label="Issue 正文">{issue.body || '此 Issue 没有正文。'}</div></section>}
        <section className="panel detail-card"><h2><Icon name="Terminal" size={18} />Worker 执行记录<span className="detail-count">{task.attempts.length}</span></h2>
          {task.attempts.length ? <div className="detail-attempts">{[...task.attempts].reverse().map(attempt =>
            <Execution key={attempt.id} attempt={attempt} task={task} latest={last?.id === attempt.id} request={request} />)}</div>
            : <p className="detail-empty-text">{task.plan ? '尚无执行记录。' : '等待 Worker 领取任务。'}</p>}</section>
      </div>
      <aside className="detail-column">
        {task.questions?.map(q => <Decision key={q.id} taskID={id} q={q} request={request} onAnswer={() => { refresh('已记录业务答复。'); onChanged() }}/>)}
        {canReview && <Acceptance key={`${last.id}:${result.commit}`} taskID={task.id} attempt={last} request={request} onUpdate={accepted} refresh={refresh} locked={loading || Boolean(error)} />}
        {task.spec && <section className="panel detail-card"><h2><Icon name="GitPullRequest" size={18} />GitHub 交付</h2>
          {delivery ? <><div className="detail-delivery-state"><span className={'detail-state-dot ' + (delivery.state === 'DONE' ? 'complete' : '')} />{deliveryStates[delivery.state] || delivery.state}</div>
            <dl className="detail-facts">{delivery.pr_number && <Fact label="Pull request"><OutLink href={delivery.pr_url}>PR #{delivery.pr_number}</OutLink></Fact>}
              <Fact label="任务分支" code>{delivery.branch}</Fact>{delivery.target_branch && <Fact label="目标分支" code>{delivery.target_branch}</Fact>}
              {delivery.merge_sha && <Fact label="合并 SHA" code>{delivery.merge_sha}</Fact>}</dl>
            {delivery.detail && <p className="detail-delivery-note">{delivery.detail}</p>}</> :
            <p className="detail-empty-text">{task.status === 'ACCEPTED' ? '此版本已通过本地验收，未创建 GitHub 交付。' : '尚无 PR 或合并记录。'}</p>}</section>}
        {task.spec && <section className="panel detail-card"><h2><Icon name="ListChecks" size={18} />验收记录<span className="detail-count">{reviews.length}</span></h2>
          {reviews.length ? <ol className="detail-reviews">{[...reviews].reverse().map(review =>
            <li key={review.execution_id}><strong>{review.decision === 'approve' ? '已通过' : '已退回'}</strong><span>执行 #{review.execution_id} · {review.actor}</span>
              <code>{review.commit}</code>{review.note && <p>{review.note}</p>}<time dateTime={review.at}>{formatTime(review.at)}</time></li>)}</ol>
            : <p className="detail-empty-text">暂无验收决定。</p>}</section>}
        <section className="panel detail-card"><h2><Icon name="History" size={18} />任务时间线</h2>
          <ol className="detail-timeline">{[...task.events].reverse().map(event =>
            <li key={event.id}><strong>{events[event.type] || event.type}</strong>
              <div><time dateTime={event.at}>{formatTime(event.at)}</time>{event.execution_id != null && <span> · 执行 #{event.execution_id}</span>}</div></li>)}</ol>
          {!task.events.length && <p className="detail-empty-text">暂无事件。</p>}</section>
      </aside>
    </div>
  </article>
}

export default function TaskDetail(props: Props) {
  return <Detail key={props.id} {...props} />
}
