import { formatTime } from './shared'

export type Advice = { level: 'D2' | 'D3'; rationale: string; suggestion: string; source: string; quote: string; session_id: string; at: string }

export function QuestionAdvice({ advice }: { advice: Advice }) {
  return <div>
    <h3>Management 建议 · {advice.level}</h3>
    <p>{advice.rationale}</p>
    {advice.suggestion && <p>建议答复 · {advice.suggestion}</p>}
    <p>引用依据 · <code>{advice.source}</code></p>
    <blockquote>{advice.quote}</blockquote>
    <small>审阅于 {formatTime(advice.at)} · 会话 <code>{advice.session_id}</code></small>
  </div>
}
