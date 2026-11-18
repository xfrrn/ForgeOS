import { workTypeName } from './workTypes'

export type IssueSuggestion = { work_type: string; quote: string; rationale: string; session_id: string }

export function ClassificationAdvice({ suggestion }: { suggestion: IssueSuggestion }) {
  return <div><p><strong>分类 Agent 建议 · {workTypeName(suggestion.work_type) || '需要澄清'}</strong></p>
    <blockquote>{suggestion.quote}</blockquote><p>{suggestion.rationale}</p>
    <details><summary>分类会话</summary><code>{suggestion.session_id}</code></details></div>
}
