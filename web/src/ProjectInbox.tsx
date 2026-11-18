import { useEffect, useRef, useState } from 'react'
import { safeGitHubURL, type ApiRequest } from './shared'

type Discussion = { id: number; number: number; title: string; url: string }

export function ProjectInbox({ project, request }: { project: number; request: ApiRequest }) {
  const [rows, setRows] = useState<Discussion[]>([])
  const [before, setBefore] = useState(0)
  const [revision, setRevision] = useState(0)
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [converting, setConverting] = useState(0)
  const conversion = useRef<AbortController | null>(null)
  useEffect(() => () => conversion.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError('')
    request<Discussion[]>(`/projects/${project}/inbox` + (before ? '?before=' + before : ''), { signal: controller.signal })
      .then(items => {
        if (controller.signal.aborted) return
        setRows(old => before ? [...old, ...items.filter(i => !old.some(r => r.id === i.id))] : items)
        setMore(items.length === 100)
      })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [before, project, request, revision])

  async function convert(number: number) {
    if (conversion.current) return
    const controller = new AbortController()
    conversion.current = controller
    setConverting(number); setError('')
    try {
      const task = await request<{ id: number }>(`/projects/${project}/issues/${number}`, {
        method: 'POST', body: JSON.stringify({ validation: 'auto' }), signal: controller.signal,
      })
      if (!controller.signal.aborted) location.hash = '/tasks/' + task.id
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '无法转换讨论。')
    } finally {
      conversion.current = null
      if (!controller.signal.aborted) setConverting(0)
    }
  }

  return <section className="panel detail-card">
    <a href="#/projects">返回项目接入</a>
    <h1>项目 #{project} · 讨论收件箱</h1>
    <p>选择讨论后可派发实现任务，由项目配置决定执行模型和验证规则。</p>
    <div><button className="button" disabled={loading || !!converting} onClick={() => { setBefore(0); setRevision(v => v + 1) }}>刷新讨论</button></div>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {loading && <p role="status">正在读取讨论…</p>}
    {!loading && !error && !rows.length && <p>暂无待转换的讨论。</p>}
    {rows.map(row => <article className="registered-project" key={row.id}>
      <div><h2>#{row.number} · {row.title}</h2>
        {safeGitHubURL(row.url) && <a href={safeGitHubURL(row.url)} target="_blank" rel="noreferrer">查看原 Issue #{row.number}</a>}</div>
      <button className="button" disabled={loading || !!converting} onClick={() => void convert(row.number)}>
        {converting === row.number ? '正在转换…' : '转为实现任务'}</button>
    </article>)}
    {more && rows.length > 0 && <div><button className="button" disabled={loading || !!converting} onClick={() => { setBefore(rows[rows.length - 1].number); setRevision(v => v + 1) }}>更多讨论</button></div>}
  </section>
}
