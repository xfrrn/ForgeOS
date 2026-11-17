import { formatTime } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'

export type Advice = { level: 'D1' | 'D2' | 'D3'; rationale: string; suggestion: string; source: string; quote: string; session_id: string; at: string; profile?: StructuredProfile }

export function QuestionAdvice({ advice }: { advice: Advice }) {
  return <div>
    <h3>Management {advice.level === 'D1' ? '决策引用' : '建议'} · {advice.level}</h3>
    <p>{advice.rationale}</p>
    {advice.suggestion && <p>建议答复 · {advice.suggestion}</p>}
    <p>引用依据 · <code>{advice.source}</code></p>
    <blockquote>{advice.quote}</blockquote>
    <small>审阅于 {formatTime(advice.at)} · 会话 <code>{advice.session_id}</code></small>
    {advice.profile && <RolePermissions role="Management" profile={advice.profile}/>}
  </div>
}
