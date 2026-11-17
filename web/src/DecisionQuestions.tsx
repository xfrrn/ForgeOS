import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, Icon, type ApiRequest } from './shared'
import { QuestionAdvice, type Advice } from './QuestionAdvice'

type Question = {
  management_advice?: Advice
  id: number; task_id: number; project_id: number; execution_id: number
  repository?: string; title?: string; checkpoint_sequence?: number; commit_sha: string; question: string; evidence: string
}
type Row = Question & { draft: string; error: string; recorded?: string }
type Page = { items: Question[]; next_before: number | null }

export function DecisionQuestions({ request, onChanged }: { request: ApiRequest; onChanged: () => void }) {
  const [rows, setRows] = useState<Row[]>([])
  const [before, setBefore] = useState<number | null>(null)
  const [busy, setBusy] = useState(true)
  const [problem, setProblem] = useState('')
  const [notice, setNotice] = useState('')
  const lifetime = useRef<AbortController | null>(null)

  const load = useCallback(async (cursor?: number) => {
    const signal = lifetime.current?.signal
    setBusy(true); setProblem('')
    try {
      const page = await request<Page>('/tasks/decision-questions' + (cursor ? '?before=' + cursor : ''), { signal })
      if (signal?.aborted) return
      setRows(previous => {
        const prior = new Map(previous.map(q => [q.id, q]))
        const items = page.items.map(q => ({ draft: '', error: '', ...prior.get(q.id), ...q }))
        const seen = new Set(items.map(q => q.id))
        const keep = previous.filter(q => !seen.has(q.id) && (cursor || q.draft.trim()))
        return [...items, ...keep].sort((a, b) => b.id - a.id)
      })
      setBefore(page.next_before)
      if (!cursor) onChanged()
    } catch (e) {
      if (!signal?.aborted) setProblem(e instanceof Error ? e.message : '读取问题失败。')
    } finally { if (!signal?.aborted) setBusy(false) }
  }, [request, onChanged])

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    void load()
    return () => controller.abort()
  }, [load])

  const selected = rows.filter(q => !q.recorded && q.draft.trim())
  async function submit() {
    const signal = lifetime.current?.signal
    setBusy(true); setProblem(''); setNotice('')
    let saved = 0
    for (const q of selected) {
      if (signal?.aborted) return
      try {
        await request(`/tasks/${q.task_id}/questions/${q.id}/answer`, {
          method: 'POST', body: JSON.stringify({ answer: q.draft.trim() }), signal,
        })
        if (signal?.aborted) return
        saved++
        setRows(prev => prev.filter(item => item.id !== q.id))
      } catch (e) {
        if (signal?.aborted) return
        const error = e instanceof Error ? e.message : '答复提交失败。'
        let recorded: string | undefined
        if (e instanceof ApiError && e.status === 409) {
          try {
            const task = await request<{ questions: { id: number; answer?: string }[] }>('/tasks/' + q.task_id, { signal })
            recorded = task.questions.find(item => item.id === q.id)?.answer
          } catch { /* The original conflict and draft remain visible if the read fails. */ }
        }
        if (signal?.aborted) return
        setRows(prev => prev.map(item => item.id === q.id ? { ...item, error, recorded } : item))
      }
    }
    const failed = selected.length - saved
    setNotice(`已提交 ${saved} 条答复。` + (failed ? `另有 ${failed} 条未确认，请核对下方记录。` : ''))
    setBusy(false)
    onChanged()
  }

  return <>
    <div className="page-heading"><h1>待答问题</h1>
      <button className="button" disabled={busy} onClick={() => void load()}><Icon name="RefreshCw" size={15}/>刷新</button>
    </div>
    {problem && <p className="detail-problem" role="alert">{problem}</p>}
    {notice && <p className="detail-notice" role="status">{notice}</p>}
    <form className="decision-queue" aria-busy={busy} onSubmit={e => { e.preventDefault(); if (!busy && selected.length) void submit() }}>
      {rows.map(q => <section className="panel detail-card detail-accept" key={q.id} aria-labelledby={`question-${q.id}`}>
        <a className="detail-link" href={'#/tasks/' + q.task_id}>T-{q.task_id} · {q.repository || '项目 ' + q.project_id}{q.title && ' · ' + q.title}</a>
        <h2 id={`question-${q.id}`}>#{q.id} {q.question}</h2>
        <p>依据 · {q.evidence}</p>
        {q.management_advice && <QuestionAdvice advice={q.management_advice}/>}
        <p>执行 #{q.execution_id} · {q.checkpoint_sequence != null && `检查点 ${q.checkpoint_sequence} · 基础提交 `}<code>{q.commit_sha}</code></p>
        <label htmlFor={`batch-answer-${q.id}`}>问题 #{q.id} 的答复</label>
        <textarea id={`batch-answer-${q.id}`} value={q.draft} disabled={busy} readOnly={!!q.recorded} onChange={e => {
          const draft = e.currentTarget.value
          setRows(prev => prev.map(item => item.id === q.id ? { ...item, draft, error: '' } : item))
        }}/>
        {q.recorded ? <><p className="detail-notice" role="status">已记录的答复 · {q.recorded}</p>
          <button type="button" className="detail-text-button" onClick={() => setRows(prev => prev.filter(item => item.id !== q.id))}>移除已答问题</button></> :
          q.error && <p className="detail-problem" role="alert">{q.error}</p>}
      </section>)}
      {!rows.length && !problem && <div className="panel table-message">{busy ? '正在读取问题…' : before ? '本页问题已处理。' : '没有待答问题。'}</div>}
      <div className="detail-actions">
        {rows.length > 0 && <button type="submit" className="button primary" disabled={busy || !selected.length}>提交已填答复（{selected.length}）</button>}
        {before && <button type="button" className="button" disabled={busy} onClick={() => void load(before)}>加载更多问题</button>}
      </div>
    </form>
  </>
}
