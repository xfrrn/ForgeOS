import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, formatTime, type ApiRequest } from './shared'

type Revision = { revision: number; content: string; content_sha256: string; published_by: string; published_at: string }

export function GlobalRules({request}: {request: ApiRequest}) {
  const [open, setOpen] = useState(false)
  const [current, setCurrent] = useState<Revision | null>(null)
  const [view, setView] = useState<Revision | null>(null)
  const [draft, setDraft] = useState<{base: number; content: string}>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => active.current?.abort(), [request])

  async function read(revision?: number) {
    const controller = new AbortController()
    active.current = controller
    setBusy(true); setError('')
    try {
      const result = await request<Revision | null>('/global-rules' + (revision ? '?revision=' + revision : ''), {signal: controller.signal})
      if (controller.signal.aborted) return
      if (!revision) setCurrent(result)
      setView(result); setOpen(true)
    } catch (e) { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '读取全局规则失败。') }
    finally { if (!controller.signal.aborted) setBusy(false) }
  }
  async function publish(e: FormEvent) {
    e.preventDefault()
    if (!draft || busy) return
    const controller = new AbortController()
    active.current = controller
    setBusy(true); setError(''); setNotice('')
    try {
      const saved = await request<Revision>('/global-rules', {method: 'PUT', signal: controller.signal,
        body: JSON.stringify({previous_revision: draft.base, content: draft.content})})
      const latest = await request<Revision>('/global-rules', {signal: controller.signal})
      if (controller.signal.aborted) return
      setCurrent(latest); setView(saved); setDraft(undefined); setNotice('已发布修订 ' + saved.revision + '。')
    } catch (e) {
      if (controller.signal.aborted) return
      setError(e instanceof ApiError && e.status === 409 ? '发布冲突。请读取当前修订，核对后再编辑。' : e instanceof Error ? e.message : '发布全局规则失败。')
    } finally { if (!controller.signal.aborted) setBusy(false) }
  }

  const bytes = new TextEncoder().encode(draft?.content || '').length
  const latest = current?.revision || 0
  return <section className="panel detail-card"><h2>全局默认规则</h2>
    <button className="button" disabled={busy} onClick={() => void read()}>读取当前全局规则</button>
    {error && <p role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}
    {open && <>
      <p>普通规则冲突时，用户当前明确指令优先，随后是项目规则和全局默认规则。</p>
      {view ? <><p>{view.revision === latest ? '当前' : '历史'}修订 {view.revision} · {view.published_by} · {formatTime(view.published_at)}</p>
        {view.content ? <pre className="material-content">{view.content}</pre> : <p>此修订已撤下全部默认规则。</p>}
        <p>SHA-256 · <code>{view.content_sha256}</code></p>
        <div className="detail-actions">
          {view.revision > 1 && <button className="button" disabled={busy} onClick={() => void read(view.revision-1)}>上一版全局规则</button>}
          {view.revision < latest && <button className="button" disabled={busy} onClick={() => void read(view.revision+1)}>下一版全局规则</button>}
        </div></> : <p>尚未发布全局默认规则。</p>}
      {!draft ? <button className="button" disabled={busy || (view?.revision || 0) !== latest}
        onClick={() => {setDraft({base: latest, content: current?.content || ''}); setNotice('')}}>编辑全局默认规则</button> :
        <form onSubmit={publish} className="material-form"><fieldset disabled={busy}>
          <p>发布内容对所有已登记项目可见。项目资料请通过脱敏共享流程发布。清空正文并发布可撤下当前默认规则。</p>
          <label>全局规则正文<textarea rows={5} value={draft.content} onChange={e => setDraft({...draft, content: e.target.value})}/></label>
          <p>基于修订 {draft.base} · {bytes} / 16384 字节</p>
          {draft.base !== latest && <button className="button" type="button" onClick={() => {setView(current); setDraft({...draft, base: latest})}}>以当前修订为基础保留草稿</button>}
          <button className="button" disabled={draft.base!==latest || bytes>16384 || draft.content === (current?.content || '') || (bytes>0 && !draft.content.trim())}>
            {draft.content ? '向所有项目发布' : '撤下全部全局默认规则'}</button>
          <button type="button" className="button" onClick={() => setDraft(undefined)}>取消编辑</button>
        </fieldset></form>}
    </>}
  </section>
}
