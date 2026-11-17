import type { GlobalRulesRevision } from './GlobalRules'

export type CandidateContext = {
  implementation_execution_id?: number
  implementation_commit?: string
  owner_clarifications?: {sequence: number; question: string; answer: string}[]
  global_rules?: GlobalRulesRevision
  decisions?: {id: number; ref: string; statement: string; validity: string}[]
  acceptance_feedback?: {execution_id: number; commit: string; note: string; actor: string}
}

export function CandidateRuleEvidence({context, title, digest}: {context: CandidateContext; title: string; digest?: string}) {
  return <details><summary>{title}</summary>
    {digest && <p>依据 SHA-256 · <code>{digest}</code></p>}
    {context.implementation_execution_id && <p>原实现执行 #{context.implementation_execution_id} · <code>{context.implementation_commit}</code></p>}
    {context.acceptance_feedback && <p>退回执行 #{context.acceptance_feedback.execution_id} · {context.acceptance_feedback.note}</p>}
    {context.owner_clarifications?.map(q => <p key={q.sequence}>检查点 {q.sequence} · {q.question} · Owner 答复 · {q.answer}</p>)}
    {context.decisions?.map(d => <p key={d.id}><a href={'#' + d.ref}>决策 #{d.id}</a> · {d.statement} · {d.validity}</p>)}
    {context.global_rules && <><p>全局规则修订 {context.global_rules.revision} · <code>{context.global_rules.content_sha256}</code></p>
      <pre className="material-content">{context.global_rules.content}</pre></>}
  </details>
}
