import { useEffect, useRef, useState } from 'react'
import { MaterialReader } from './MaterialReader'
import { formatTime, type ApiRequest } from './shared'

type Turn = { worker_task_id?: number; id: number; revision: number; item?: string; message: string; draft: boolean; model: string; status: string; answer?: string; generated_revision?: number; at: string }
export type DiscussionTarget = {revision: number; item: string}
type Props = {project: number; task: number; target: DiscussionTarget; canDraft: boolean; path: string; request: ApiRequest}

export default function GoalDiscussion({project, task, target, canDraft, path, request}: Props) {
  const endpoint = `/tasks/${task}/planner-discussion`
  const [turns, setTurns] = useState<Turn[]>()
  const [message, setMessage] = useState('')
  const [posting, setPosting] = useState(false)
  const [readError, setReadError] = useState('')
  const [sendError, setSendError] = useState('')
  const [reload, setReload] = useState(0)
  const [awaiting, setAwaiting] = useState(0)
  const retry = useRef({body: '', id: ''})
  const lock = useRef(false)
  const field = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {field.current?.focus()}, [target])
  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      let delay = 8000
      try {
        const history = await request<Turn[]>(endpoint, {signal: controller.signal})
        if (controller.signal.aborted) return
        setTurns(history)
        setReadError('')
        if (history.some(t => t.status === 'running' || t.status === 'queued')) delay = 1500
        if (history.some(t => t.id === awaiting)) setAwaiting(0)
      } catch (e) {
        if (!controller.signal.aborted) setReadError(e instanceof Error ? e.message : '无法读取讨论记录。')
      }
      if (!controller.signal.aborted) timer = setTimeout(read, delay)
    }
    void read()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [endpoint, request, reload, awaiting])

  const pending = turns?.some(t => t.status === 'queued' || t.status === 'running')
  const blocked = !turns || !!readError || posting || !!awaiting || pending
  async function submit(draft: boolean, previous?: Turn) {
    if (lock.current || blocked) return
    const input = {revision: previous?.revision ?? target.revision, item: previous ? previous.item || '' : target.item,
      message: draft ? '' : previous?.message ?? message.trim(), draft}
    const body = JSON.stringify(input)
    if (retry.current.body !== body) retry.current = {body, id: crypto.randomUUID()}
    lock.current = true
    setPosting(true); setSendError('')
    try {
      const receipt = await request<{id: number}>(endpoint, {method: 'POST', body: JSON.stringify({...input, client_id: retry.current.id})})
      setAwaiting(receipt.id)
      retry.current = {body: '', id: ''}
      if (!draft && !previous) setMessage('')
    } catch (e) { setSendError(e instanceof Error ? e.message : '发送失败，请重试。') }
    finally {lock.current = false; setPosting(false); setReload(n => n + 1)}
  }

  return <section className="goal-discussion" aria-label="Planner 讨论">
    <h3>Planner 讨论</h3>
    <p>Worker 使用<a href={`#/projects/${project}/agents`}>项目 Planner 配置</a>回复和生成草案。</p>
    {turns === undefined && !readError && <p role="status">正在读取讨论…</p>}
    {turns?.length === 0 && <p>可以就这版计划提问，或说明希望调整的工作项。</p>}
    <ol className="goal-discussion-history">{turns?.map(t => <li key={t.id}>
      <small>修订 {t.revision}{t.item && ` · ${t.item}`} · {formatTime(t.at)}</small>
      <strong>我</strong><p className="goal-discussion-message">{t.draft ? '根据讨论生成新的计划草案' : t.message}</p>
      <strong>Planner <small>{t.model}</small></strong>
      {t.worker_task_id && <a href={`#/projects/${project}/tasks/${t.worker_task_id}`}>查看 Worker 执行记录</a>}
      {t.status === 'done' ? <><MaterialReader markdown content={t.answer || ''}/>
        {t.generated_revision && <a href={`${path}/plans/${t.generated_revision}`}>查看计划草案 · 修订 {t.generated_revision}</a>}</> :
        <p role="status">{t.status === 'failed' ? t.answer : t.status === 'queued' ? '等待支持项目 Planner 的 Worker 领取…' : 'Planner 正在处理…'}</p>}
      {t.status === 'failed' && t.id === turns.at(-1)?.id && (!t.draft || (canDraft && t.revision === target.revision)) &&
        <button className="button" disabled={blocked} onClick={() => void submit(t.draft, t)}>重试本轮</button>}
    </li>)}</ol>
    {readError && <p role="alert">{readError} <button className="button" onClick={() => setReload(n => n + 1)}>重新读取</button></p>}
    <form onSubmit={e => {e.preventDefault(); if (message.trim()) void submit(false)}}>
      <label htmlFor="goal-planner-message">讨论修订 {target.revision}{target.item && ` · 工作项 ${target.item}`}</label>
      <textarea id="goal-planner-message" ref={field} rows={3} maxLength={4000} value={message} disabled={posting}
        onChange={e => setMessage(e.target.value)} placeholder="想和 Planner 讨论什么？"/>
      {sendError && <p role="alert">{sendError}</p>}
      <div className="detail-actions">
        <button className="button primary" disabled={blocked || !message.trim()}>发送给 Planner</button>
        {canDraft && <button type="button" className="button" disabled={blocked || !!message.trim() || turns?.filter(t => !t.draft).at(-1)?.status !== 'done'}
          onClick={() => void submit(true)}>根据讨论生成草案</button>}
      </div>
    </form>
  </section>
}
