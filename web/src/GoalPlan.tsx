import { useEffect, useState } from 'react'
import { ApiError, Fact, Status, type ApiRequest } from './shared'

type PlanItem = { key: string; objective: string; scope: string; acceptance_criteria: string; deliverables: string; dependencies?: string[] }
export type Plan = { revision: number; git_commit?: string; review?: { level: string; effective_level: string; validation_minimum?: string; verdict: string; rationale: string; evidence: string; session_id: string }; plan: { objective: string; milestones: { name: string; items: PlanItem[] | null }[] } }
export type GoalChild = { id: number; key: string; status: string; plan_revision?: number }
type Props = { taskID: number; selected?: number; latest?: Plan; active?: number; items?: GoalChild[]; request: ApiRequest }

export default function GoalPlan({taskID, selected, latest, active, items, request}: Props) {
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
  const path = `#/tasks/${taskID}`
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
      {plan.git_commit && <p>计划归档提交 <code>{plan.git_commit}</code></p>}
      {plan.review && <div><h3>Management 审核 · {plan.review.effective_level}</h3>
        <dl><Fact label="建议">{{approve: '通过', revise: '修订', needs_owner: '请 Owner 决定'}[plan.review.verdict] ?? plan.review.verdict}</Fact>
          <Fact label="理由">{plan.review.rationale}</Fact><Fact label="依据">{plan.review.evidence}</Fact>
          {plan.review.validation_minimum && <Fact label="建议最低验证等级">{plan.review.validation_minimum}</Fact>}
          <Fact label="审核会话" code>{plan.review.session_id}</Fact></dl>
      </div>}
      {plan.plan.milestones.map((milestone, index) => <div key={index}>
        <h3>{milestone.name}</h3>
        <ul>{milestone.items?.map(item => <li key={item.key}>
          {children.has(item.key) ? <a href={`#/tasks/${children.get(item.key)!.id}`}>{item.key} · {item.objective} {active !== shown && '当前状态：'}<Status value={children.get(item.key)!.status}/></a>
            : <strong>{item.key} · {item.objective}</strong>}
          <dl><Fact label="范围">{item.scope}</Fact><Fact label="验收条件">{item.acceptance_criteria}</Fact>
            <Fact label="交付物">{item.deliverables}</Fact>
            {item.dependencies?.length ? <Fact label="依赖">{item.dependencies.join('、')}</Fact> : null}</dl>
        </li>)}</ul>
      </div>)}
    </>}
  </section>
}
