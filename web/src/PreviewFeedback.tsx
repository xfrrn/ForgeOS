import { useEffect, useState, type FormEvent } from 'react'
import { formatTime, type ApiRequest } from './shared'

type Note = { id: number; created_at: string; state: string; classification?: { category: string; rationale: string; evidence: string; session_id: string }; body: { path: string; note: string; element?: string; browser?: string; width?: number; height?: number; case_id?: string; plan_revision?: number } }
const categories: Record<string, string> = { acceptance: '验收通过', bug: 'Bug', ui_change: 'UI 修改', requirement_change: '需求变更', new_work_item: '新工作项', blocked: '阻塞或澄清' }
type Page = { preview: { execution_id: number; commit: string; directory: string; state: string }; feedback: Note[]; plan: { revision: number; cases: { id: string; objective: string }[] } | null }

export function PreviewFeedback({ task, preview, captureQuery, request }: { task: number; preview: string; captureQuery?: string; request: ApiRequest }) {
  const capture = new URLSearchParams(captureQuery)
  const [page, setPage] = useState<Page | null>(null)
  const [path, setPath] = useState(() => capture.get('path') || '/')
  const [note, setNote] = useState('')
  const [caseID, setCaseID] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [version, setVersion] = useState(0)
  const [key, setKey] = useState(() => crypto.randomUUID())
  const api = `/tasks/${task}/previews/${preview}/feedback`
  const mismatch = Boolean(captureQuery && page && (capture.get('commit') !== page.preview.commit || capture.get('execution') !== String(page.preview.execution_id)))
  useEffect(() => {
    const controller = new AbortController()
    request<Page>(api, { signal: controller.signal }).then(result => { if (!controller.signal.aborted) setPage(result) })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [api, request, version])
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!page || busy || mismatch) return
    setBusy(true); setError(''); setMessage('')
    try {
      const result = await request<{ id: number }>(api, { method: 'POST', body: JSON.stringify({ request_id: key,
        execution_id: page.preview.execution_id, commit: page.preview.commit, path: path.trim(), note: note.trim(),
        case_id: caseID, plan_revision: caseID ? page.plan?.revision : 0,
        element: capture.get('element') || '', browser: capture.get('browser') || '',
        width: Number(capture.get('width')), height: Number(capture.get('height')) }) })
      setMessage(`反馈 #${result.id} 已登记。`); setNote(''); setKey(crypto.randomUUID()); setVersion(n => n + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '反馈提交失败。') }
    finally { setBusy(false) }
  }
  return <section className="panel"><a href={`#/tasks/${task}`}>返回任务 T-{task}</a><h1>预览反馈</h1>
    {error && <p role="alert">{error}</p>}{message && <p role="status">{message}</p>}
    {page && <><p>执行 {page.preview.execution_id} · {page.preview.directory} · <code>{page.preview.commit}</code></p>
      <form onSubmit={submit}><fieldset><legend>记录观察</legend>
        {mismatch && <p role="alert">捕获页面的候选或执行与此预览不一致，请返回预览重新选择。</p>}
        {captureQuery && <div><p>采集环境　{capture.get('browser')} · {capture.get('width')} × {capture.get('height')}</p>
          {capture.get('element') && <p>所选元素　<code>{capture.get('element')}</code></p>}</div>}
        <label>页面路径（省略查询参数和片段）<input required maxLength={2048} value={path} onChange={e => setPath(e.target.value)} /></label>
        <label>关联测试用例<select value={caseID} onChange={e => setCaseID(e.target.value)}><option value="">无关联用例</option>
          {page.plan?.cases.map(c => <option key={c.id} value={c.id}>{c.id} · {c.objective}</option>)}</select></label>
        <label>观察结果或修改意见<textarea required maxLength={8192} value={note} onChange={e => setNote(e.target.value)} /></label>
        <button className="button" disabled={busy || mismatch || !note.trim()}>提交反馈</button>
      </fieldset></form><h2>该预览的反馈</h2>
      <button className="detail-text-button" onClick={() => { setError(''); setVersion(n => n + 1) }}>刷新反馈分类</button>
      {!page.feedback.length && <p>尚无反馈。</p>}
      {page.feedback.map(f => <article key={f.id}><h3>反馈 #{f.id}</h3><p>{f.classification ? 'Management 分类 · ' + categories[f.classification.category] : f.state === 'in_review' ? 'Management 正在分类' : '待 Management 分类'} · {formatTime(f.created_at)} · <code>{f.body.path}</code>
        {f.body.case_id && ` · 计划 v${f.body.plan_revision} · ${f.body.case_id}`}</p><p>{f.body.note}</p>
        {f.body.element && <p>元素 <code>{f.body.element}</code></p>}
        {f.body.browser && <p>{f.body.browser} · {f.body.width} × {f.body.height}</p>}
        {f.classification && <details><summary>分类依据</summary><p>{f.classification.rationale}</p><blockquote>{f.classification.evidence}</blockquote>
          <p>Management 会话　<code>{f.classification.session_id}</code></p></details>}</article>)}
    </>}
  </section>
}
