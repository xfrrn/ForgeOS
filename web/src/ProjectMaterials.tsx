import { useEffect, useRef, useState, type FormEvent } from 'react'
import { KnowledgeCuration } from './KnowledgeCuration'
import { ExperienceSearch } from './ExperienceSearch'
import { RepositoryDocumentChanges } from './RepositoryDocumentChanges'
import { ShareDraft } from './ProjectShares'
import { ApiError, formatTime, type ApiRequest } from './shared'
import { BackLink, PageHeading } from './ui'

const kinds = { material: '导入资料', profile: '项目档案', environment: '环境说明', memory: 'Owner 记忆请求' }
type Kind = keyof typeof kinds
type Draft = { key: string; kind: Kind; title: string; source: string; content: string }
type Document = Draft & { revision: number; content_sha256: string; imported_at: string }
type Summary = Omit<Document, 'content'> & { excerpt: string; start_char?: number; end_char?: number }
const empty: Draft = { key: '', kind: 'material', title: '', source: '', content: '' }
const options = Object.entries(kinds).map(([k, label]) => <option key={k} value={k}>{label}</option>)

function DocumentText({ doc }: { doc: Document }) {
  return <>
    <h3>{doc.title}</h3>
    <p>{kinds[doc.kind]} · 修订 {doc.revision} · {formatTime(doc.imported_at)}</p>
    <p>来源 · {doc.source}</p>
    <p>SHA-256 · <code>{doc.content_sha256}</code></p>
    <pre className="material-content">{doc.content}</pre>
  </>
}

export function ProjectMaterials({ project, request }: { project: number; request: ApiRequest }) {
  const base = `/projects/${project}/materials`
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState('')
  const [filter, setFilter] = useState({ q: '', kind: '', after: '' })
  const [rows, setRows] = useState<Summary[]>([])
  const [more, setMore] = useState(false)
  const [listing, setListing] = useState(true)
  const [listError, setListError] = useState('')
  const [doc, setDoc] = useState<Document>()
  const [latest, setLatest] = useState(0)
  const [draft, setDraft] = useState<Draft>()
  const [previous, setPrevious] = useState(0)
  const [conflict, setConflict] = useState<Document>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const lifetime = useRef<AbortController | null>(null)
  const contentHeading = useRef<HTMLHeadingElement>(null)
  const editing = Boolean(draft)
  useEffect(() => { contentHeading.current?.focus() }, [doc, editing])
  useEffect(() => {
    const c = new AbortController()
    lifetime.current = c
    return () => c.abort()
  }, [request, project])
  useEffect(() => {
    const c = new AbortController()
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(filter)) if (value) params.set(key, value)
    setListing(true); setListError(''); setMore(false)
    if (!filter.after) setRows([])
    request<Summary[]>(base + (params.size ? '?' + params : ''), { signal: c.signal }).then(items => {
      if (c.signal.aborted) return
      setRows(old => filter.after ? old.concat(items.filter(i => !old.some(r => r.key === i.key))) : items)
      setMore(!filter.q && items.length === 30)
    }).catch(e => { if (!c.signal.aborted) setListError(e.message) })
      .finally(() => { if (!c.signal.aborted) setListing(false) })
    return () => c.abort()
  }, [request, base, filter])

  async function read(key: string, revision?: number, fromList = false) {
    const signal = lifetime.current?.signal
    setBusy(true); setError('')
    try {
      const path = base + '/' + encodeURIComponent(key)
      const head = fromList ? await request<Document>(path, { signal }) : undefined
      const value = head && head.revision === revision ? head : await request<Document>(path + (revision ? '?revision=' + revision : ''), { signal })
      if (signal?.aborted) return
      setDoc(value); setDraft(undefined); setConflict(undefined)
      if (head || !revision) setLatest(head?.revision ?? value.revision)
    } catch (e) { if (!signal?.aborted) setError(e instanceof Error ? e.message : '无法读取资料。') }
    finally { if (!signal?.aborted) setBusy(false) }
  }
  function edit(value: Draft, revision = 0) {
    setDraft(value); setPrevious(revision); setConflict(undefined); setError('')
  }
  function remember() {
    const key = 'memory-' + crypto.getRandomValues(new Uint32Array(4)).join('-')
    edit({ ...empty, kind: 'memory', key, source: 'Owner 工作台' })
  }
  async function save(e: FormEvent) {
    e.preventDefault()
    if (!draft || busy) return
    const bytes = (value: string) => new TextEncoder().encode(value).length
    if (!/^[\p{L}\p{N}][\p{L}\p{N}._-]{0,95}$/u.test(draft.key)) { setError('标识须以字母或数字开头，最多 96 字，可包含点、横线和下划线。'); return }
    if ([draft.title, draft.source, draft.content].some(value => !value.trim() || value.includes('\0'))) { setError('请填写有效的标题、来源和正文。'); return }
    if (bytes(draft.content) > 256 * 1024 || bytes(draft.title) > 256 || bytes(draft.source) > 2048) { setError('正文最多 256 KiB，标题最多 256 字节，来源最多 2048 字节。'); return }
    const path = base + '/' + encodeURIComponent(draft.key)
    const signal = lifetime.current?.signal
    setBusy(true); setError('')
    try {
      const { kind, title, source, content } = draft
      const saved = await request<Document>(path, { method: 'PUT', signal, body: JSON.stringify({ previous_revision: previous, kind, title, source, content }) })
      if (signal?.aborted) return
      setDraft(undefined); setConflict(undefined); setDoc(saved); setLatest(saved.revision)
      setFilter(old => ({ ...old, after: '' }))
    } catch (e) {
      if (signal?.aborted) return
      if (e instanceof ApiError && e.status === 409) {
        setError('资料已有其他修订。请核对最新版，草稿已保留。')
        try {
          const current = await request<Document>(path, { signal })
          if (!signal?.aborted) setConflict(current)
        } catch { if (!signal?.aborted) setError('无法读取冲突版本，草稿已保留。请稍后重试。') }
      } else setError(e instanceof Error ? e.message : '无法保存资料。')
    } finally { if (!signal?.aborted) setBusy(false) }
  }
  const formLocked = busy || Boolean(draft)
  return <div className="project-setup">
    <BackLink href="#/projects">返回项目接入</BackLink>
    <PageHeading title={`项目 #${project} · 项目资料`}/>
    <RepositoryDocumentChanges key={project} project={project} request={request}/>
    <ExperienceSearch key={'experience-' + project} project={project} request={request}/>
    <section className="panel detail-card">
      <form className="project-input" onSubmit={e => { e.preventDefault(); setFilter({ q: query.trim(), kind, after: '' }) }}>
        <input aria-label="检索资料" value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索标题或正文"/>
        <select aria-label="筛选资料类型" value={kind} onChange={e => setKind(e.target.value)}><option value="">全部类型</option>{options}</select>
        <button className="button">检索</button>
        <button type="button" className="button" disabled={formLocked} onClick={() => edit(empty)}>导入资料</button>
        <button type="button" className="button" disabled={formLocked} onClick={remember}>记住一条项目经验</button>
      </form>
      {listError && <p role="alert">{listError}</p>}
      {listing && <p role="status">正在读取资料…</p>}
      {!listing && !listError && !rows.length && <p>没有匹配的资料。</p>}
      {rows.map(row => <article className="registered-project" key={row.key}><div>
        <h3>{row.title}</h3><p><code>{row.key}</code> · {kinds[row.kind]} · 修订 {row.revision}</p>
        {row.start_char !== undefined && row.end_char !== undefined && <p>{row.source} · 正文第 {row.start_char + 1}–{row.end_char} 字符</p>}
        <p>{row.excerpt}</p></div><button className="button" disabled={formLocked} onClick={() => void read(row.key, row.revision, true)}>查看 · {row.key}</button></article>)}
      {more && <button className="button" disabled={listing} onClick={() => setFilter({ ...filter, after: rows[rows.length-1].key })}>更多资料</button>}
    </section>
    {error && <p role="alert" className="detail-problem">{error}</p>}
    {draft ? <form className="panel detail-card material-form" onSubmit={save}>
      <h2 ref={contentHeading} tabIndex={-1}>{previous ? '编辑资料' : draft.kind === 'memory' ? '记住一条项目经验' : '导入项目资料'}</h2>
      <fieldset disabled={busy}>
        <label>资料标识<input required value={draft.key} readOnly={previous > 0} onChange={e => { setConflict(undefined); setDraft({ ...draft, key: e.target.value }) }}/></label>
        <label htmlFor="material-kind">资料类型</label>
        <select id="material-kind" value={draft.kind} onChange={e => setDraft({ ...draft, kind: e.target.value as Kind })}>{options}</select>
        <label>标题<input required value={draft.title} onChange={e => setDraft({ ...draft, title: e.target.value })}/></label>
        <label>来源<input required value={draft.source} onChange={e => setDraft({ ...draft, source: e.target.value })}/></label>
        <label htmlFor="material-body">正文</label>
        <textarea id="material-body" required rows={10} value={draft.content} onChange={e => setDraft({ ...draft, content: e.target.value })}/>
        {conflict && <section><h3>当前最新版</h3><DocumentText doc={conflict}/>
          <button className="button" type="button" onClick={() => { setPrevious(conflict.revision); setConflict(undefined); setError('') }}>以修订 {conflict.revision} 为基础继续编辑</button></section>}
        <div className="detail-actions"><button className="button" disabled={Boolean(conflict)}>{!previous && draft.kind === 'memory' ? '提交记忆请求' : `保存修订 ${previous + 1}`}</button>
          <button type="button" className="button" onClick={() => { setDraft(undefined); setConflict(undefined); setError('') }}>取消编辑</button></div>
      </fieldset>
    </form> : doc && <section key={doc.key + '/' + doc.revision} className="panel detail-card"><h2 ref={contentHeading} tabIndex={-1}><code>{doc.key}</code></h2><DocumentText doc={doc}/><div className="detail-actions">
      {doc.revision > 1 && <button className="button" disabled={busy} onClick={() => void read(doc.key, doc.revision-1)}>上一修订</button>}
      {doc.revision < latest
        ? <button className="button" disabled={busy} onClick={() => void read(doc.key)}>查看最新版</button>
        : <button className="button" disabled={busy} onClick={() => edit(doc, doc.revision)}>编辑此修订</button>}
    </div><KnowledgeCuration project={project} path={`${base}/${encodeURIComponent(doc.key)}/revisions/${doc.revision}/curation`} request={request}/>
      <ShareDraft project={project} material={doc.key} revision={doc.revision} request={request}/></section>}
  </div>
}
