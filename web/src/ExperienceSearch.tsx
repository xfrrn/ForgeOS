import { useEffect, useRef, useState, type FormEvent } from 'react'
import { KnowledgeCuration } from './KnowledgeCuration'
import { formatTime, type ApiRequest } from './shared'

type ExperienceHit = {
  id: number; fact_number: number; statement: string; quote: string
  title: string; source: string; revision: number; receipt_path: string; reviewed_at: string
  start_char: number; end_char: number; content_sha256: string
}

export function ExperienceSearch({ project, request }: { project: number; request: ApiRequest }) {
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<ExperienceHit[]>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => active.current?.abort(), [project, request])
  async function search(e: FormEvent) {
    e.preventDefault()
    active.current?.abort()
    const call = new AbortController()
    active.current = call
    setHits(undefined); setBusy(true); setError('')
    try {
      const rows = await request<ExperienceHit[]>(`/projects/${project}/experience?q=${encodeURIComponent(query.trim())}`, { signal: call.signal })
      if (!call.signal.aborted) setHits(rows)
    } catch (err) {
      if (!call.signal.aborted) setError(err instanceof Error ? err.message : '无法检索项目经验。')
    } finally { if (!call.signal.aborted) setBusy(false) }
  }
  return <section className="panel detail-card">
    <h2>已审核项目经验</h2>
    <form className="project-input" onSubmit={search}>
      <input aria-label="检索已审核经验" value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索技术观察或原文引句"/>
      <button className="button" disabled={!query.trim()}>检索经验</button>
    </form>
    {busy && <p role="status">正在检索项目经验…</p>}
    {error && <p role="alert">{error}</p>}
    {hits && !hits.length && <p>没有匹配的已审核经验。</p>}
    {hits?.map(hit => <article className="registered-project" key={hit.id + '/' + hit.fact_number}><div>
      <h3>{hit.statement}</h3><blockquote>{hit.quote}</blockquote>
      <p>{hit.title} · 修订 {hit.revision} · {formatTime(hit.reviewed_at)}</p>
      <p>{hit.source} · 原文第 {hit.start_char + 1}–{hit.end_char} 字符</p>
      <p>正文 SHA-256 · <code>{hit.content_sha256}</code></p>
      <KnowledgeCuration project={project} path={hit.receipt_path} request={request}/>
    </div></article>)}
  </section>
}
