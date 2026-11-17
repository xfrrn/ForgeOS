import { useEffect, useState } from 'react'
import { formatTime, Status, type ApiRequest } from './shared'

export const noticeLabels = { question: '需要答复', plan: '计划审核需要处理', acceptance: '候选等待验收', failure: '执行需要核对' }
export type Notice = { id: number; task_id: number; project_id: number; execution_id?: number; plan_revision?: number;
  kind: keyof typeof noticeLabels; title: string; status: string; pending: boolean; at: string; read_at?: string }
type Page = { items: Notice[]; next_before?: number }

export function noticePath(n: Notice) {
  return `#/tasks/${n.task_id}${n.plan_revision ? '/plans/' + n.plan_revision : ''}`
}

export function NotificationCount({request, revision}: {request: ApiRequest; revision: number}) {
  const [count, setCount] = useState(0)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const result = await request<{unread: number}>('/tasks/notifications/unread', {signal: stop.signal})
        if (!stop.signal.aborted) setCount(result.unread)
      } catch { /* The inbox also reports connection failures when opened. */ }
      if (!stop.signal.aborted) timer = setTimeout(read, 8000)
    }
    void read()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request, revision])
  return count > 0 ? <span className="nav-count" aria-label={count + ' 条待处理未读通知'}>{count}</span> : null
}

export function Notifications({request, onRead}: {request: ApiRequest; onRead: () => void}) {
  const [history, setHistory] = useState(false)
  const [cursors, setCursors] = useState<number[]>([])
  const before = cursors.at(-1)
  const [page, setPage] = useState<Page>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [marking, setMarking] = useState(0)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const rows = await request<Page>('/tasks/notifications?scope=' + (history ? 'history' : 'pending') + (before ? '&before=' + before : ''), {signal: stop.signal})
        if (!stop.signal.aborted) { setPage(rows); setError('') }
      } catch (e) { if (!stop.signal.aborted) setError(e instanceof Error ? e.message : '读取通知失败。') }
      if (!stop.signal.aborted) timer = setTimeout(read, 8000)
    }
    void read()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request, history, before, revision])

  async function mark(id: number) {
    setMarking(id)
    try {
      await request('/tasks/notifications/' + id + '/read', {method: 'POST'})
      onRead()
      setRevision(n => n + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '未确认已读状态，请重试。') }
    finally { setMarking(0) }
  }
  return <section className="panel detail-card"><h1>通知</h1>
    <div className="detail-actions">
      <label>通知范围　<select value={history ? 'history' : 'pending'} onChange={e => {
        setPage(undefined); setCursors([]); setHistory(e.target.value === 'history')
      }}><option value="pending">待处理</option><option value="history">全部记录</option></select></label>
      <button className="button" onClick={() => setRevision(n => n + 1)}>刷新通知</button>
    </div>
    {error && <p role="alert">{error}</p>}
    {!page && !error && <p role="status">正在读取通知…</p>}
    {page?.items.length === 0 && <p>{history ? '本页没有通知记录。' : '本页没有待处理通知。'}</p>}
    {page?.items.map(n => <article className="notification-row" key={n.id}>
      <h2>{noticeLabels[n.kind]} · {n.title}</h2>
      <p>项目 #{n.project_id} · 任务 T-{n.task_id}{n.execution_id && ' · 执行 #' + n.execution_id}{n.plan_revision && ' · 修订 ' + n.plan_revision}
        {' · '}<time dateTime={n.at}>{formatTime(n.at)}</time></p>
      <p>{n.pending ? '待处理' : '已离开此处理阶段'} · {n.read_at ? '已读' : '未读'} · <Status value={n.status}/></p>
      <div className="detail-actions"><a href={noticePath(n)}>{n.plan_revision ? '查看计划修订' : '打开任务'}</a>
        {!n.read_at && <button className="button" disabled={!!marking} onClick={() => void mark(n.id)}>标为已读</button>}
      </div>
    </article>)}
    <div className="detail-actions">
      {before && <button className="button" onClick={() => { setPage(undefined); setCursors(c => c.slice(0,-1)) }}>上一页</button>}
      {page?.next_before && <button className="button" onClick={() => { setPage(undefined); setCursors(c => [...c,page.next_before!]) }}>下一页</button>}
    </div>
  </section>
}
