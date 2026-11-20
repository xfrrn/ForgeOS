import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import { useEffect, useState } from 'react'
import { ApiError, Fact, projectHref, Status, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'
import { CandidateRuleEvidence, type CandidateContext } from './CandidateRuleEvidence'

export type PlanItem = { key: string; objective: string; scope: string; acceptance_criteria: string; deliverables: string; dependencies?: string[]; context_refs?: string[] }
export type Plan = { review_processes?: ReviewProcess[]; revision: number; minimum_level?: string; git_commit?: string; review_context?: CandidateContext; review?: { level: string; effective_level: string; validation_minimum?: string; verdict: string; rationale: string; evidence: string; session_id: string; profile?: StructuredProfile }; plan: { objective: string; milestones: { name: string; items: PlanItem[] | null }[] } }
export type GoalChild = { id: number; key: string; status: string; plan_revision?: number }
type Props = { projectId: number; taskID: number; selected?: number; latest?: Plan; active?: number; items?: GoalChild[]; request: ApiRequest; activate?: (revision: number) => Promise<void>; disabled: boolean }

function PlanArchive({taskID, plan, request, disabled}: Pick<Props, 'taskID' | 'request' | 'disabled'> & {plan: Plan}) {
  const [receipt, setReceipt] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const sha = plan.git_commit || receipt
  if (sha) return <p>计划归档提交 <code>{sha}</code></p>
  async function archive() {
    setBusy(true)
    setError('')
    try {
      const saved = await request<Plan>(`/tasks/${taskID}/plans/${plan.revision}/archive`, {method: 'POST'})
      setReceipt(saved.git_commit ?? '')
    } catch (cause) {
      setError(cause instanceof ApiError && cause.status === 409
        ? '归档冲突，请核对项目克隆、固定基线和此修订的 Git 引用。'
        : cause instanceof ApiError ? cause.message : '未确认归档结果，可重试此修订。')
    } finally { setBusy(false) }
  }
  return <div><button className="button" disabled={disabled || busy} onClick={() => void archive()}>将修订 {plan.revision} 归档到 Git</button>
    {error && <p role="alert">{error}</p>}</div>
}

export default function GoalPlan({projectId, taskID, selected, latest, active, items, request, activate, disabled}: Props) {
  const [archived, setArchived] = useState<Plan>()
  const [error, setError] = useState('')
  const historical = selected !== undefined && selected !== latest?.revision
  useEffect(() => {
    if (!historical) return
    setError('')
    const controller = new AbortController()
    request<Plan>(`/tasks/${taskID}/plans/${selected}`, {signal: controller.signal})
      .then(plan => { if (!controller.signal.aborted) setArchived(plan) })
      .catch(cause => { if (!controller.signal.aborted) setError(cause instanceof ApiError && cause.status === 404 ? '找不到此计划修订。' : '无法读取计划修订。') })
    return () => controller.abort()
  }, [taskID, selected, historical, request])

  const plan = historical ? archived : latest
  const shown = selected ?? latest?.revision
  const path = projectHref(projectId, `/tasks/${taskID}`)
  const children = new Map(items?.filter(child =>
    (active === shown && child.status !== 'SUPERSEDED') || child.plan_revision === shown
  ).map(child => [child.key, child]))
  return <section className="panel detail-card">
    <h2>Goal 计划 · 修订 {shown}</h2>
    {latest && shown && <nav className="plan-revisions" aria-label="计划修订">
      {shown > 1 && shown <= latest.revision && <a href={`${path}/plans/${shown - 1}`}>上一版</a>}
      {shown < latest.revision && <a href={`${path}/plans/${shown + 1}`}>下一版</a>}
      {active && active !== shown ? <a href={`${path}/plans/${active}`}>生效版 · 修订 {active}</a> : null}
      {shown !== latest.revision && <a href={path}>最新修订 · {latest.revision}</a>}
    </nav>}
    {historical && error ? <p role="alert">{error}</p> : !plan ? <p role="status">正在读取计划修订…</p> : <>
      {shown === active ? <p>当前生效版本</p> : shown === latest?.revision ? <p>尚未生效</p> : <p>历史修订</p>}
      <p>{plan.plan.objective}</p>
      <PlanArchive key={plan.revision} taskID={taskID} plan={plan} request={request} disabled={disabled}/>
      {plan.review_context && <CandidateRuleEvidence context={plan.review_context} title="计划审核使用的规则依据"/>}
      <ManagementProcesses request={request} processes={plan.review_processes}/>
      {plan.review && <div><h3>Management 审核 · {plan.review.effective_level}</h3>
        <dl><Fact label="建议">{{approve: '通过', revise: '修订', needs_owner: '请 Owner 决定'}[plan.review.verdict] ?? plan.review.verdict}</Fact>
          <Fact label="理由">{plan.review.rationale}</Fact><Fact label="依据">{plan.review.evidence}</Fact>
          {plan.review.validation_minimum && <Fact label="建议最低验证等级">{plan.review.validation_minimum}</Fact>}
          <Fact label="审核会话" code>{plan.review.session_id}</Fact></dl>
        {plan.review.profile && <RolePermissions role="Management" profile={plan.review.profile}/>}
      </div>}
      {plan.plan.milestones.map((milestone, index) => <div key={index}>
        <h3>{milestone.name}</h3>
        <ul>{milestone.items?.map(item => <li key={item.key}>
          {children.has(item.key) ? <a href={projectHref(projectId, `/tasks/${children.get(item.key)!.id}`)}>{item.key} · {item.objective} {active !== shown && '当前状态：'}<Status value={children.get(item.key)!.status}/></a>
            : <strong>{item.key} · {item.objective}</strong>}
          <dl><Fact label="范围">{item.scope}</Fact><Fact label="验收条件">{item.acceptance_criteria}</Fact>
            <Fact label="交付物">{item.deliverables}</Fact>
            {item.dependencies?.length ? <Fact label="依赖">{item.dependencies.join('、')}</Fact> : null}
            {item.context_refs?.length ? <Fact label="上下文引用">{item.context_refs.join('、')}</Fact> : null}</dl>
        </li>)}</ul>
      </div>)}
      {activate && plan.revision === latest?.revision && plan.revision !== active &&
        <button className="button primary" disabled={disabled} onClick={() => void activate(plan.revision)}>批准并激活修订 {plan.revision}</button>}
    </>}
  </section>
}
