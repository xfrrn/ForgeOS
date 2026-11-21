import type { GlobalRulesRevision } from './GlobalRules'
import { formatTime } from './shared'

const sourceKinds = { instruction: '项目指令', decision: '已确认项目决策', experience: '已审核项目经验', shared_experience: '收到的共享经验', git: 'Git 仓库文档', material: 'ForgeOS 资料' }

type KnowledgeReference = {
  citation_id: number
  cited_at: string
  kind: keyof typeof sourceKinds
  title: string
  source: string
  version: string
  excerpt: string
  content: string
  validity?: string
  valid_until?: string
  publication_status?: 'current' | 'superseded' | 'revoked'
  offset_unit?: 'byte' | 'char'
  start?: number
  end?: number
}

export type CandidateContext = {
  implementation_execution_id?: number
  implementation_commit?: string
  owner_instructions?: {sequence: number; session_id: string; instructions: {key: string; body: string}[]}[]
  owner_clarifications?: {id?: number; execution_id?: number; sequence?: number; question: string; answer: string; commit_sha?: string}[]
  management_references?: {question_id: number; sequence: number; session_id: string; decision: {id: string; text: string; validity: string; source: string}}[]
  knowledge_references?: KnowledgeReference[]
  global_rules?: GlobalRulesRevision
  decisions?: {id: number; ref: string; statement: string; validity: string; plan_keys?: string[]}[]
  acceptance_feedback?: {execution_id: number; commit: string; note: string; actor: string}
}

export function CandidateRuleEvidence({context, title, digest}: {context: CandidateContext; title: string; digest?: string}) {
  return <details><summary>{title}</summary>
    {digest && <p>依据 SHA-256 · <code>{digest}</code></p>}
    {context.implementation_execution_id && <p>原实现执行 #{context.implementation_execution_id} · <code>{context.implementation_commit}</code></p>}
    {context.acceptance_feedback && <p>退回执行 #{context.acceptance_feedback.execution_id} · {context.acceptance_feedback.note}</p>}
    {context.owner_clarifications?.map(q => <p key={q.id != null ? 'question-' + q.id : 'turn-' + q.sequence}>
      {q.execution_id && `原执行 #${q.execution_id} · `}{q.sequence == null ? '候选验收' : `检查点 ${q.sequence}`} · {q.question} · Owner 答复 · {q.answer}
      {q.commit_sha && <> · <code>{q.commit_sha}</code></>}</p>)}
    {context.owner_instructions?.map(batch => <div key={batch.sequence}>
      <p>检查点 {batch.sequence} · 已处理的 Owner 指令</p>
      <ol>{batch.instructions.map(item => <li key={item.key}>{item.body}</li>)}</ol>
      <small>处理会话 <code>{batch.session_id}</code></small>
    </div>)}
    {context.management_references?.map(ref => <div key={ref.question_id}>
      <p>检查点 {ref.sequence} · Management 引用 <code>{ref.decision.id}</code> · 问题 #{ref.question_id}</p>
      <blockquote>{ref.decision.text}</blockquote>
      <p>适用范围 · {ref.decision.validity} · 来源 · {ref.decision.source}</p>
      <small>Management 会话 <code>{ref.session_id}</code></small>
    </div>)}
    {context.knowledge_references?.map(ref => <details className="detail-knowledge-reference" key={ref.citation_id}>
      <summary>{sourceKinds[ref.kind]} · {ref.title}{ref.offset_unit && ` · ${ref.offset_unit === 'byte' ? '字节' : '字符'} [${ref.start ?? 0}, ${ref.end})`}</summary>
      <p>引用 #{ref.citation_id} · {formatTime(ref.cited_at)}</p>
      <p>来源 · {ref.source}</p>
      {ref.publication_status === 'superseded' && <p>采用时有效，现已有新版。</p>}
      {ref.publication_status === 'revoked' && <p>采用时有效，现已撤销共享。</p>}
      <p>读取版本 · <code>{ref.version}</code></p>
      {ref.validity && <p>适用条件 · {ref.validity}</p>}
      {ref.valid_until && <p>有效期至 {formatTime(ref.valid_until)}</p>}
      {ref.kind === 'experience' && ref.excerpt !== ref.content && <p>经验 · {ref.excerpt}</p>}
      <pre className="material-content">{ref.content}</pre>
    </details>)}
    {context.decisions?.map(d => <p key={d.id}><a href={'#' + d.ref}>决策 #{d.id}</a> · {d.statement} · {d.validity}
      {d.plan_keys && <> · 计划工作项：{d.plan_keys.join('、')}</>}</p>)}
    {context.global_rules && <><p>全局规则修订 {context.global_rules.revision} · <code>{context.global_rules.content_sha256}</code></p>
      <pre className="material-content">{context.global_rules.content}</pre></>}
  </details>
}
