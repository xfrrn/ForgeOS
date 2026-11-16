import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ApiError, Icon, Status, formatTime, safeGitHubURL, type ApiRequest } from './shared'
import './detail.css'
import { CandidateTestPlans } from './CandidateTestPlans'
import { CandidatePreviews } from './CandidatePreviews'

type CheckRun = { id: string; runner: string; level: string; profile?: string; config_blob?: string; image?: string
  decision?: { rules_blob?: string; changed_paths: number; reasons: { source: string; level: string; evidence: string }[] }
  started_at: string; finished_at: string; failure?: string
  checks: { name: string; required: boolean; status: string; failure?: string; exit_code: number | null; log: string
    log_digest?: { bytes: number; sha256: string }
    started_at: string; finished_at: string; outputs?: { source: string; kind: string; file: string; bytes: number; sha256: string }[] }[] }
type Validation = { profile: string; commit: string; exit_code: number; output: string; run?: CheckRun }
type HarnessProcess = { instance: number; pid: number; harness: string; version: string; session_id?: string; started_at: string; exited_at: string; exit_code: number }
type Result = { commit?: string; base_commit?: string; session_id?: string; harness?: string; harness_version?: string; processes?: HarnessProcess[]; exit_code: number; error?: string; validation?: Validation; plan?: { objective: string } }
type Attempt = {
  review_feedback?: { id: number; preview_id: string; decision: 'approve' | 'reject'; note: string }[]
  test_plan_revision?: number; test_plan_state?: string; test_case_state?: string; test_case_attempts?: number; requires_test_plan?: boolean
  commit_status?: { id: number; state: string; context: string; commit: string }
  code_review_state?: 'pending' | 'failed' | 'needs_check' | 'clear'
  id: number; status: string; worker: string; workspace: string; lease_until: string
  uses_checkpoints?: boolean; requirement_revision?: number; lease_expired: boolean; abandoned_at?: string; end_reason?: string; result?: Result
}
type Issue = { title: string; number: number; body: string; url: string; repository: string; updated_at: string }
type Review = { execution_id: number; commit: string; decision: string; note: string; actor: string; at: string; feedback_ids?: number[] }
type Delivery = { execution_id: number; commit: string; state: string; detail?: string; branch: string; target_branch: string; pr_url?: string; pr_number?: number; merge_sha?: string }
type TaskEvent = { id: number; execution_id?: number; type: string; at: string }
type Question = { id: number; execution_id: number; commit_sha: string; question: string; evidence: string; answer?: string }
type PlanItem = { key: string; objective: string; scope: string; acceptance_criteria: string; deliverables: string; dependencies?: string[] }
type Plan = { revision: number; review?: { level: string; effective_level: string; validation_minimum?: string; verdict: string; rationale: string; evidence: string; session_id: string }; plan: { objective: string; milestones: { name: string; items: PlanItem[] | null }[] } }
type Task = {
  feedback_sources?: { feedback_id: number; task_id: number; preview: string; execution_id: number; commit: string }[]
  requires_code_review?: boolean
  id: number; project_id: number; status: string; kind: string
  spec?: { issue: Issue; base_commit: string; validation: string; harness?: string; child_harness?: string; project_config_blob?: string }
  execution: Attempt | null; attempts: Attempt[]; events: TaskEvent[]; reviews?: Review[]; delivery?: Delivery; delivery_refreshes?: { execution_id: number; old_delivery: Delivery; new_base: string; candidate: string }[]
  plan?: Plan
  requirement_revision?: number; latest_requirement?: number; active_revision?: number; goal_id?: number; plan_revision?: number
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
  goal_item_superseded: '已从后续计划移除', goal_item_revised: '已更新待执行工作项',
  delivery_base_refreshed: '已在新的目标提交上准备候选',
  requirement_adopted: '已采用新的需求修订',
  requirement_observed: '已记录新的 Issue 需求版本',
  requirement_revised_from_feedback: 'Owner 已从预览反馈修订需求',
  feedback_work_item_dispatched: '已从反馈派发独立工作项',
  feedback_screenshot_saved: '已归档预览反馈截图',
  management_reviewed: 'Management 已审核本版计划',
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

function DeclaredChecks({ run, path, request }: { run: CheckRun; path: string; request: ApiRequest }) {
  const [error, setError] = useState('')
  const download = async (file: string) => {
    setError('')
    try {
      const blob = await request<Blob>(path + '/' + encodeURIComponent(run.id) + '/' + encodeURIComponent(file), undefined, 'blob')
      const link = document.createElement('a')
      link.href = URL.createObjectURL(blob)
      link.download = file
      link.click()
      setTimeout(() => URL.revokeObjectURL(link.href), 1000)
    } catch (e) { setError(e instanceof Error ? e.message : '文件下载失败') }
  }
  return <details><summary>逐项检查</summary>
    <dl className="detail-facts"><Fact label="运行编号" code>{run.id}</Fact><Fact label="Runner" code>{run.runner}</Fact>
      {run.profile && <Fact label="验证方案">{run.level} · {run.profile}</Fact>}
      {run.config_blob && <Fact label="配置 blob" code>{run.config_blob}</Fact>}
      {run.image && <Fact label="容器镜像" code>{run.image}</Fact>}
      <Fact label="运行时间">{formatTime(run.started_at)} — {formatTime(run.finished_at)}</Fact></dl>
    {run.decision && <><h4>风险选级依据</h4><p>变更路径 {run.decision.changed_paths} 条</p>
      {run.decision.rules_blob && <p>验收规则 <code>{run.decision.rules_blob}</code></p>}
      <ul>{run.decision.reasons.map(r => <li key={r.source + ':' + r.evidence}>{r.level} · {r.source} · {r.evidence}</li>)}</ul></>}
    <ol>{run.checks.map(c => <li key={c.name}><strong>{c.name}</strong> · {c.required ? '必需' : '可选'} · {c.status === 'PASSED' ? '通过' : c.failure || '失败'}
      <dl className="detail-facts"><Fact label="退出码" code>{c.exit_code ?? '无退出码'}</Fact>
        <Fact label="检查时间">{formatTime(c.started_at)} — {formatTime(c.finished_at)}</Fact><Fact label="本地日志" code>{c.log}{c.log_digest && ` · ${c.log_digest.bytes} 字节`}</Fact>
        {c.log_digest && <Fact label="日志 SHA-256" code>{c.log_digest.sha256}</Fact>}</dl>
      {c.log_digest && <button type="button" onClick={() => download(c.log)}>下载 {c.log}</button>}
      {c.outputs?.map(o => <dl key={o.file} className="detail-facts"><Fact label={o.kind === 'report' ? '报告' : '产物'}>{o.source} · {o.bytes} 字节</Fact>
        <Fact label="归档文件" code><button type="button" onClick={() => download(o.file)}>下载 {o.file}</button></Fact><Fact label="SHA-256" code>{o.sha256}</Fact></dl>)}
    </li>)}</ol>
    {error && <p role="alert">{error}</p>}
  </details>
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

type Requirement = { revision: number; feedback_id?: number; feedback_task?: number; feedback_preview?: string; observed_at: string; issue: Issue }

function RequirementHistory({ id, latest, request, canApply, onApply }: { id: number; latest: number; request: ApiRequest; canApply: boolean; onApply: (message: string) => void }) {
  const [revision, setRevision] = useState<number | null>(null)
  const [value, setValue] = useState<Requirement | null>(null)
  const [failed, setFailed] = useState(false)
  const [applying, setApplying] = useState(false)
  async function apply() {
    setApplying(true)
    try {
      await request(`/tasks/${id}/requirements/${revision}/apply`, { method: 'POST' })
      onApply(`任务已采用需求修订 ${revision}。`)
    } catch {
      onApply('请核对最新需求修订与任务状态。')
    } finally { setApplying(false) }
  }
  useEffect(() => {
    if (revision === null) return
    const controller = new AbortController()
    setFailed(false)
    setValue(null)
    request<Requirement>(`/tasks/${id}/requirements/${revision}`, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setValue(result) })
      .catch(() => { if (!controller.signal.aborted) setFailed(true) })
    return () => controller.abort()
  }, [id, revision, request])
  if (revision === null) return <button className="button" onClick={() => setRevision(latest)}>查看需求历史（{latest} 版）</button>
  return <div>
    <h3>需求修订 {revision}</h3>
    <div className="detail-actions"><button className="button" disabled={revision <= 1} onClick={() => setRevision(revision - 1)}>上一版</button>
      <button className="button" disabled={revision >= latest} onClick={() => setRevision(revision + 1)}>下一版</button>
      <button className="button" onClick={() => setRevision(null)}>收起历史</button></div>
    {failed ? <p role="alert">无法读取此需求版本。</p> : value?.revision === revision
      ? <><h4>{value.issue.title}</h4><p className="detail-issue-text">{value.issue.body}</p>
        <p>{value.feedback_id ? <a href={`#/tasks/${value.feedback_task}/previews/${value.feedback_preview}/feedback`}>Owner 修订 · 预览反馈 #{value.feedback_id} · {formatTime(value.observed_at)}</a> : `GitHub 更新时间 · ${formatTime(value.issue.updated_at)}`}</p></>
      : <p role="status">正在读取需求修订…</p>}
    {canApply && revision === latest && value?.revision === latest &&
      <button className="button primary" disabled={applying} onClick={() => void apply()}>采用修订 {revision} 并排队执行</button>}
  </div>
}

type Checkpoint = { sequence: number; through_event: number; comment_count: number; at: string; session_id?: string }

function CheckpointHistory({ task, attempt, request }: { task: Task; attempt: Attempt; request: ApiRequest }) {
  const [open, setOpen] = useState(false)
  const [rows, setRows] = useState<Checkpoint[]>([])
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setFailed(false)
    request<Checkpoint[]>(`/tasks/${task.id}/checkpoints?execution_id=${attempt.id}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setRows(value) })
      .catch(() => { if (!controller.signal.aborted) setFailed(true) })
    return () => controller.abort()
  }, [open, task, attempt.id, request])
  return <div><button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起执行检查点' : '查看执行检查点'}</button>
    {open && (failed ? <p role="alert">无法读取执行检查点。</p> : <ol>{rows.map(row => <li key={row.sequence}>
      <strong>{row.sequence === 0 ? '回合开始' : '提交前'} · {row.comment_count} 条评论</strong> · {formatTime(row.at)}
      <dl><Fact label="截至评论事件">{row.through_event}</Fact>
        {row.session_id && <Fact label="处理会话" code>{row.session_id}</Fact>}</dl>
    </li>)}</ol>)}</div>
}

type ReviewReceipt = {
  base_commit: string; commit_sha: string; owner_note?: string; owner_checked_at?: string
  report: { session_id: string; summary: string; findings: { id: string; severity: string; file: string; side: string
    line: number; evidence: string; evidence_verified: boolean; blocking: boolean; confidence: number; description: string; recommendation: string }[] }
}

function CandidateReview({ task, attempt, request, refresh, current }: {
  task: Task; attempt: Attempt; request: ApiRequest; refresh: (message: string) => void; current: boolean
}) {
  const state = attempt.code_review_state
  const [open, setOpen] = useState(false)
  const [receipt, setReceipt] = useState<ReviewReceipt | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const path = `/tasks/${task.id}/code-review`
  const hasReport = state === 'needs_check' || state === 'clear'
  useEffect(() => {
    if (!open || !hasReport) return
    const controller = new AbortController()
    setReceipt(null)
    setError('')
    request<ReviewReceipt>(`${path}?execution_id=${attempt.id}`, { signal: controller.signal })
      .then(data => { if (!controller.signal.aborted) setReceipt(data) })
      .catch(() => { if (!controller.signal.aborted) setError('无法读取审查报告，请重新打开。') })
    return () => controller.abort()
  }, [path, attempt.id, state, hasReport, open, request])
  async function submit(check: boolean) {
    setBusy(true)
    setError('')
    try {
      await request<void>(path + (check ? '/check' : ''), { method: 'POST', body: JSON.stringify({
        execution_id: attempt.id, commit: attempt.result?.commit, ...(check ? { note: note.trim() } : {}),
      }) })
      refresh(check ? '已保存本次审查的核对说明。' : '已为此候选请求独立审查。')
    } catch (e) { setError(e instanceof Error ? e.message : '提交失败，请重试。') }
    finally { setBusy(false) }
  }
  if (!state && !current) return null
  const labels = { pending: '等待审查完成', failed: '审查失败', needs_check: '发现待核对', clear: '审查完成' }
  return <section className={`detail-validation detail-code-review${state === 'failed' ? ' failed' : ''}`}>
    <h3>独立代码审查</h3>
    <p>{state ? labels[state] : task.requires_code_review ? '此项目要求审查，正在等待 Reviewer 领取。' : '尚未请求审查。'}</p>
    {current && (!state || state === 'failed') && <button className="button" disabled={busy}
      onClick={() => void submit(false)}>{state === 'failed' ? '重试审查' : '请求独立审查'}</button>}
    {hasReport && <button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起审查报告' : '查看审查报告'}</button>}
    {open && !receipt && !error && <p role="status">正在读取审查报告…</p>}
    {open && receipt && <>
      <dl className="detail-facts"><Fact label="审查会话" code>{receipt.report.session_id}</Fact>
        <Fact label="基础 SHA" code>{receipt.base_commit}</Fact><Fact label="候选 SHA" code>{receipt.commit_sha}</Fact></dl>
      <p>{receipt.report.summary}</p>
      {receipt.report.findings.length === 0 ? <p>未报告发现。</p> : <ol>{receipt.report.findings.map(f => <li key={f.id}>
        <strong>{f.id} · {f.severity}{f.blocking && ' · 阻塞'}</strong>
        <p>{f.description}</p><code>{f.side === 'base' ? '基础' : '候选'} · {f.file}:{f.line}</code><pre>{f.evidence}</pre>
        <p>{f.evidence_verified ? '引用与 Git 原文匹配' : '引用尚未核实'} · 置信度 {f.confidence}</p><p>建议：{f.recommendation}</p>
      </li>)}</ol>}
      {receipt.owner_note && <p>Owner 核对 · {receipt.owner_note}</p>}
      {current && state === 'needs_check' && <form className="detail-accept" onSubmit={e => { e.preventDefault(); void submit(true) }}>
        <label htmlFor={`review-note-${attempt.id}`}>发现核对说明</label>
        <textarea id={`review-note-${attempt.id}`} required value={note} onChange={e => setNote(e.target.value)} />
        <button className="button" disabled={busy || !note.trim() || new TextEncoder().encode(note.trim()).length > 4096}>保存核对说明</button>
      </form>}
    </>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
  </section>
}

function Execution({ attempt, task, latest, request, refresh }: { attempt: Attempt; task: Task; latest: boolean; request: ApiRequest; refresh: (message: string) => void }) {
  const result = attempt.result
  const validation = result?.validation
  const start = task.events.find(event => event.execution_id === attempt.id && event.type === 'execution_claimed')
  return <details className="detail-attempt" open={latest}>
    <summary><Icon name="ChevronRight" size={15} /><strong>执行 #{attempt.id}</strong><span className="detail-worker mono">{attempt.worker}</span><Status value={attempt.status} /></summary>
    <div className="detail-attempt-body"><dl className="detail-facts">
      {start && <Fact label="领取时间">{formatTime(start.at)}</Fact>}
      <Fact label="Worktree" code>{attempt.workspace}</Fact>
      {attempt.requirement_revision && <Fact label="执行绑定需求">{attempt.requirement_revision}</Fact>}
      {result?.harness && <Fact label="执行 Harness">{result.harness === 'claude' ? 'Claude Code' : 'Codex'} {result.harness_version}</Fact>}
      {result?.session_id && <Fact label={result.harness === 'claude' ? 'Claude Code Session' : 'Codex Session'} code>{result.session_id}</Fact>}
      {result?.base_commit && <Fact label="基础 SHA" code>{result.base_commit}</Fact>}
      {result?.commit && <Fact label={task.spec ? '候选 SHA' : '提交 SHA'} code>{result.commit}</Fact>}
      {attempt.commit_status && <Fact label="GitHub 状态" code>{attempt.commit_status.context} · {attempt.commit_status.state} · #{attempt.commit_status.id}</Fact>}
      {result && <Fact label="执行退出码" code>{result.exit_code}</Fact>}
      {attempt.status === 'RUNNING' && <Fact label="租约">{attempt.lease_expired ? '已过期 · 等待核对' : formatTime(attempt.lease_until)}</Fact>}
      {attempt.abandoned_at && <Fact label="人工结束">{formatTime(attempt.abandoned_at)}</Fact>}
    </dl>
    {result?.processes && result.processes.length > 0 && <details>
      <summary>Harness 进程记录（{result.processes.length}）</summary>
      {result.processes.map(p => <dl className="detail-facts" key={p.instance}>
        <Fact label="进程 PID" code>{p.pid}</Fact>
        <Fact label="Harness 版本">{p.harness} {p.version}</Fact>
        <Fact label="开始时间">{formatTime(p.started_at)}</Fact>
        <Fact label="退出时间">{formatTime(p.exited_at)}</Fact>
        <Fact label="进程退出码" code>{p.exit_code}</Fact>
        {p.session_id && <Fact label="原生会话" code>{p.session_id}</Fact>}
      </dl>)}
    </details>}
    {attempt.uses_checkpoints && <CheckpointHistory task={task} attempt={attempt} request={request}/>}
    {result?.error && <p className="detail-problem" role="status">{result.error}</p>}
    {validation ? <section className={'detail-validation ' + (validation.exit_code === 0 ? 'passed' : 'failed')}>
      <div><Icon name={validation.exit_code === 0 ? 'CheckCircle2' : 'AlertCircle'} size={16} />
        <strong>{validation.exit_code === 0 ? '本地验证通过' : '本地验证失败'}</strong><code>{validation.profile}</code>
      </div>
      <dl className="detail-facts"><Fact label="验证 SHA" code>{validation.commit}</Fact><Fact label="验证退出码" code>{validation.exit_code}</Fact></dl>
      {validation.output && <pre>{validation.output}</pre>}
      {validation.run && <DeclaredChecks run={validation.run} path={`/tasks/${task.id}/executions/${attempt.id}/ci`} request={request} />}
    </section> : result?.plan ? <p>规划结果 · {result.plan.objective}</p> : task.spec && <p className="detail-empty-text">此执行尚无本地验证结果。</p>}
    {task.spec && <CandidateDiff key={result?.commit} taskID={task.id} attempt={attempt} request={request} />}
    {task.spec && result?.commit && validation?.commit === result.commit && validation.exit_code === 0 &&
      <CandidatePreviews task={task.id} execution={attempt.id} commit={result.commit} request={request} />}
    {result?.commit && validation?.exit_code === 0 && (attempt.test_plan_revision || (latest && task.status === 'WAITING_ACCEPTANCE')) ?
      <CandidateTestPlans task={task.id} execution={attempt.id} commit={result.commit} latest={attempt.test_plan_revision ?? 0}
        state={attempt.test_plan_state ?? ''} attempts={attempt.test_case_attempts ?? 0} required={Boolean(attempt.requires_test_plan)}
        current={latest && task.status === 'WAITING_ACCEPTANCE'} request={request} refresh={refresh} /> : null}
    {task.kind === 'codex_issue' && result?.commit && validation?.commit === result.commit && validation.exit_code === 0 &&
      <CandidateReview task={task} attempt={attempt} request={request} refresh={refresh} current={latest && task.status === 'WAITING_ACCEPTANCE'} />}
    </div>
  </details>
}

function Acceptance({ taskID, attempt, request, onUpdate, refresh, locked, reviewPending }: {
  taskID: number; attempt: Attempt; request: ApiRequest; onUpdate: (task: Task) => void; refresh: (message: string) => void; locked: boolean; reviewPending: boolean
}) {
  const commit = attempt.result!.commit!
  const [selected, setSelected] = useState<number[]>([])
  const reports = attempt.review_feedback ?? []
  const incompatible = (decision: string) => reports.some(f => selected.includes(f.id) && f.decision !== decision)
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
        body: JSON.stringify({ execution_id: attempt.id, commit, decision, note: feedback, feedback_ids: selected }),
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
    {reports.length > 0 && <fieldset disabled={busy}><legend>引用预览反馈</legend>
      {reports.map(f => <div key={f.id}><label><input type="checkbox" checked={selected.includes(f.id)}
        disabled={!selected.includes(f.id) && incompatible(f.decision)}
        onChange={e => setSelected(e.target.checked ? [...selected, f.id] : selected.filter(id => id !== f.id))} />
        #{f.id} · {f.decision === 'approve' ? '通过' : '退回'} · {f.note}</label>{' '}
        <a href={`#/tasks/${taskID}/previews/${f.preview_id}/feedback`}>查看反馈依据</a></div>)}
    </fieldset>}
    <label htmlFor="acceptance-note">验收说明</label>
    <textarea id="acceptance-note" ref={noteField} value={note} disabled={busy} placeholder="退回时请说明需要修改的内容" onChange={e => { setNote(e.target.value); setError('') }} />
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {reviewPending && <p role="status">{(attempt.requires_test_plan || attempt.test_plan_state) && attempt.test_case_state !== 'passed'
      ? '请先完成此候选的测试计划、覆盖审查和用例实测。' : '请先在执行记录中完成独立代码审查和发现核对。'}</p>}
    <div className="detail-actions"><button className="button primary" disabled={busy || locked || reviewPending || incompatible('approve')} onClick={() => decide('approve')}><Icon name="Check" size={16} />通过验收</button>
      <button className="button" disabled={busy || locked || incompatible('reject')} onClick={() => decide('reject')}>退回修改</button></div>
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

function RefreshDelivery({ id, delivery, request, reload }: { id: number; delivery: Delivery; request: ApiRequest; reload: (message: string) => void }) {
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    try {
      await request(`/tasks/${id}/delivery/refresh`, { method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ execution_id: delivery.execution_id, commit: delivery.commit }) })
      reload('已在新的目标版本上重新排队验证。')
    } catch (e) { reload(e instanceof Error ? e.message : '请核对交付状态后重试。') }
    finally { setBusy(false) }
  }
  return <div><p>目标分支已前进。重新应用已批准的差异后会关闭此 PR，新候选需再次验证和验收。</p>
    <button className="button" disabled={busy} onClick={() => void submit()}>在最新目标版本上重新验证</button></div>
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
        if (['PLANNING', 'ACTIVE', 'BLOCKED', 'READY', 'RUNNING', 'WAITING_INPUT', 'WAITING_ACCEPTANCE'].includes(fresh.status) || (fresh.status === 'ACCEPTED' && fresh.delivery)) {
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
      {task.spec?.project_config_blob && <details><summary>项目执行配置</summary><dl className="detail-facts">
        <Fact label="实现 Harness">{(task.spec.child_harness || task.spec.harness) === 'claude' ? 'Claude Code' : 'Codex'}</Fact>
        <Fact label="project.yaml 对象" code>{task.spec.project_config_blob}</Fact>
      </dl></details>}
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
          {task.active_revision && task.active_revision !== task.plan.revision ? <p>待审批修订 · 当前生效版本 {task.active_revision}</p> : null}
          {task.plan.review && <div><h3>Management 审核 · {task.plan.review.effective_level}</h3>
            <dl><Fact label="建议">{{approve: '通过', revise: '修订', needs_owner: '请 Owner 决定'}[task.plan.review.verdict] ?? task.plan.review.verdict}</Fact>
              <Fact label="理由">{task.plan.review.rationale}</Fact><Fact label="依据">{task.plan.review.evidence}</Fact>
              {task.plan.review.validation_minimum && <Fact label="建议最低验证等级">{task.plan.review.validation_minimum}</Fact>}
              <Fact label="审核会话" code>{task.plan.review.session_id}</Fact></dl>
          </div>}
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
          {task.feedback_sources?.map(f => <p key={f.feedback_id}><a href={`#/tasks/${f.task_id}/previews/${f.preview}/feedback`}>来源任务 T-{f.task_id} · 执行 #{f.execution_id} · 预览反馈 #{f.feedback_id}</a></p>)}
          {task.requirement_revision && <p>任务绑定需求修订 {task.requirement_revision}</p>}
          <div className="detail-issue-text" tabIndex={0} aria-label="Issue 正文">{issue.body || '此 Issue 没有正文。'}</div>
          {task.latest_requirement && <RequirementHistory id={id} latest={task.latest_requirement} request={request}
            canApply={!task.plan && !task.goal_id && task.latest_requirement !== task.requirement_revision && ['READY','FAILED','WAITING_ACCEPTANCE'].includes(task.status)}
            onApply={message => { refresh(message); onChanged() }}/>}</section>}
        <section className="panel detail-card"><h2><Icon name="Terminal" size={18} />Worker 执行记录<span className="detail-count">{task.attempts.length}</span></h2>
          {task.attempts.length ? <div className="detail-attempts">{[...task.attempts].reverse().map(attempt =>
            <Execution key={attempt.id} attempt={attempt} task={task} latest={last?.id === attempt.id} request={request} refresh={refresh} />)}</div>
            : <p className="detail-empty-text">{task.plan ? '尚无执行记录。' : '等待 Worker 领取任务。'}</p>}</section>
      </div>
      <aside className="detail-column">
        {task.questions?.map(q => <Decision key={q.id} taskID={id} q={q} request={request} onAnswer={() => { refresh('已记录业务答复。'); onChanged() }}/>)}
        {canReview && <Acceptance key={`${last.id}:${result.commit}`} taskID={task.id} attempt={last} request={request} onUpdate={accepted} refresh={refresh} locked={loading || Boolean(error)}
          reviewPending={(Boolean(task.requires_code_review || last.code_review_state) && last.code_review_state !== 'clear') ||
            Boolean((last.test_plan_state || last.requires_test_plan) && last.test_case_state !== 'passed')} />}
        {task.spec && <section className="panel detail-card"><h2><Icon name="GitPullRequest" size={18} />GitHub 交付</h2>
          {delivery ? <><div className="detail-delivery-state"><span className={'detail-state-dot ' + (delivery.state === 'DONE' ? 'complete' : '')} />{deliveryStates[delivery.state] || delivery.state}</div>
            <dl className="detail-facts">{delivery.pr_number && <Fact label="Pull request"><OutLink href={delivery.pr_url}>PR #{delivery.pr_number}</OutLink></Fact>}
              <Fact label="任务分支" code>{delivery.branch}</Fact>{delivery.target_branch && <Fact label="目标分支" code>{delivery.target_branch}</Fact>}
              {delivery.merge_sha && <Fact label="合并 SHA" code>{delivery.merge_sha}</Fact>}</dl>
            {delivery.detail && <p className="detail-delivery-note">{delivery.detail}</p>}
            {delivery.state === 'WAITING' && delivery.detail === 'accepted candidate is behind the protected target branch' &&
              <RefreshDelivery id={id} delivery={delivery} request={request} reload={message => { refresh(message); onChanged() }}/>}</> :
            <p className="detail-empty-text">{task.status === 'ACCEPTED' ? '此版本已通过本地验收，未创建 GitHub 交付。' : '尚无 PR 或合并记录。'}</p>}</section>}
        {task.spec && <section className="panel detail-card"><h2><Icon name="ListChecks" size={18} />验收记录<span className="detail-count">{reviews.length}</span></h2>
          {reviews.length ? <ol className="detail-reviews">{[...reviews].reverse().map(review =>
            <li key={review.execution_id}><strong>{review.decision === 'approve' ? '已通过' : '已退回'}</strong><span>执行 #{review.execution_id} · {review.actor}</span>
              <code>{review.commit}</code>{Boolean(review.feedback_ids?.length) && <p>引用反馈　{review.feedback_ids?.map(id => '#' + id).join('、')}</p>}
              {review.note && <p>{review.note}</p>}<time dateTime={review.at}>{formatTime(review.at)}</time></li>)}</ol>
            : <p className="detail-empty-text">暂无验收决定。</p>}</section>}
        {task.delivery_refreshes?.map(r => <section className="panel detail-card" key={r.execution_id}>
          <h2>已替换的交付 · 执行 #{r.execution_id}</h2><dl><Fact label="原 PR"><OutLink href={r.old_delivery.pr_url}>PR #{r.old_delivery.pr_number}</OutLink></Fact>
          <Fact label="原候选" code>{r.old_delivery.commit}</Fact><Fact label="新基线" code>{r.new_base}</Fact>
          <Fact label="新候选" code>{r.candidate}</Fact></dl></section>)}
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
