import { workTypeName } from './workTypes'
import { useEffect, useState } from 'react'
import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import type { ApiRequest } from './shared'

export type IssueSuggestion = { work_type: string; quote: string; rationale: string; session_id: string }

export function ClassificationAdvice({ suggestion, classification, project, request }: { suggestion?: IssueSuggestion; classification?: number; project: number; request: ApiRequest }) {
  const [open, setOpen] = useState(false)
  const [history, setHistory] = useState<ReviewProcess[]>()
  const [error, setError] = useState('')
  useEffect(() => {
    if (!open || !classification) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function load() {
      try {
        const result = await request<{ settled: boolean; processes: ReviewProcess[] }>(`/projects/${project}/classifications/${classification}/processes`, { signal: controller.signal })
        if (controller.signal.aborted) return
        setHistory(result.processes); setError('')
        if (result.settled) return
      } catch (e) {
        if (controller.signal.aborted) return
        setError(e instanceof Error ? e.message : '无法读取分类进程。')
      }
      timer = setTimeout(load, 4000)
    }
    void load()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [open, classification, project, request])
  return <div>{suggestion && <><p><strong>分类 Agent 建议 · {workTypeName(suggestion.work_type) || '需要澄清'}</strong></p>
    <blockquote>{suggestion.quote}</blockquote><p>{suggestion.rationale}</p>
    <details><summary>分类会话</summary><code>{suggestion.session_id}</code></details></>}
    {classification && <details onToggle={e => setOpen(e.currentTarget.open)}><summary>查看分类进程</summary>
      {error && <p role="alert">{error}</p>}
      {!error && !history?.length && <p role="status">{history ? '尚无节点进程回执。' : '正在读取分类进程…'}</p>}
      <ManagementProcesses processes={history} title="分类 Agent 进程"/>
    </details>}</div>
}
