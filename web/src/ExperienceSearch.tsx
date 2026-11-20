import { useEffect, useState } from 'react'
import { KnowledgeCuration } from './KnowledgeCuration'
import { formatTime, type ApiRequest } from './shared'
import './project-work.css'

type Fact = { statement: string; quote: string }
type ExperienceSource = {
  id: number; title: string; source: string; revision: number; receipt_path: string; reviewed_at: string; content_sha256: string
}
type Experience = ExperienceSource & { facts: Fact[] }
type ExperienceHit = ExperienceSource & Fact & { fact_number: number; start_char: number; end_char: number }

export function ExperienceSearch({ project, request }: { project: number; request: ApiRequest }) {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState({ query: '', before: 0 })
  const [rows, setRows] = useState<(Experience | ExperienceHit)[]>([])
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    const suffix = filter.query ? '?q=' + encodeURIComponent(filter.query) : filter.before ? '?before=' + filter.before : ''
    setBusy(true); setError('')
    if (!filter.before) setRows([])
    void request<(Experience | ExperienceHit)[]>(`/projects/${project}/experience` + suffix, { signal: controller.signal })
      .then(items => {
        if (controller.signal.aborted) return
        setRows(old => filter.before ? old.concat(items) : items)
        setMore(!filter.query && items.length === 30)
      })
      .catch(cause => { if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '无法读取项目经验。') })
      .finally(() => { if (!controller.signal.aborted) setBusy(false) })
    return () => controller.abort()
  }, [project, request, filter])
  return <section className="panel detail-card project-experience" aria-labelledby="experience-heading">
    <h2 id="experience-heading">已审核项目经验</h2>
    <form className="project-input" onSubmit={event => { event.preventDefault(); setFilter({ query: query.trim(), before: 0 }) }}>
      <input aria-label="检索已审核经验" value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索技术观察或原文引句"/>
      <button className="button" disabled={!query.trim()}>检索经验</button>
      {filter.query && <button className="button" type="button" onClick={() => { setQuery(''); setFilter({ query: '', before: 0 }) }}>浏览全部经验</button>}
    </form>
    {busy && <p role="status">正在读取项目经验…</p>}
    {error && <p role="alert">{error} <button className="button" onClick={() => setFilter({ ...filter })}>重试</button></p>}
    {!busy && !error && !rows.length && <p>{filter.query ? '没有匹配的已审核经验。' : '本项目暂无已审核经验。'}</p>}
    {rows.map(row => <article className="registered-project" key={'facts' in row ? row.id : `${row.id}/${row.fact_number}`}><div>
      <h3>{'facts' in row ? row.title : row.statement}</h3>
      {'facts' in row ? <ul>{row.facts.map((fact, index) => <li key={index}><strong>{fact.statement}</strong><blockquote>{fact.quote}</blockquote></li>)}</ul> :
        <><blockquote>{row.quote}</blockquote><p>{row.title} · 原文第 {row.start_char + 1}–{row.end_char} 字符</p></>}
      <p>{row.source} · 修订 {row.revision} · {formatTime(row.reviewed_at)}</p>
      <p>正文 SHA-256 · <code>{row.content_sha256}</code></p>
      <KnowledgeCuration project={project} path={row.receipt_path} request={request}/>
    </div></article>)}
    {more && <button className="button" disabled={busy || Boolean(error)} onClick={() => {
      const last = rows.at(-1)
      if (last) setFilter({ query: '', before: last.id })
    }}>更多项目经验</button>}
  </section>
}
