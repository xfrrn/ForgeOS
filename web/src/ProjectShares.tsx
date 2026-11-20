import { ThemedSelect } from './ThemedSelect'
import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, formatTime, projectHref, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'
import { BackLink, PageHeading } from './ui'

type Copy = { id: number; title: string; content: string; digest: string; published_at?: string }
type Receipt = Copy & {
 review_processes?: ReviewProcess[]
  project_id: number; material_key: string; revision: number; recipient: number; proposed_at: string; revoked_at?: string
  review?: { verdict: 'approve' | 'revise' | 'needs_owner'; rationale: string; evidence: string; effective_level: string; session_id: string; profile?: StructuredProfile }
}
const verdicts = { approve: '审核通过，待发布', revise: '需要修改副本', needs_owner: '需要 Owner 决定' }

export function ShareDraft({ project, material, revision, request }: {
  project: number; material: string; revision: number; request: ApiRequest
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const lifetime = useRef<AbortController | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [request, project, material, revision])
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const data = new FormData(e.currentTarget)
    const recipient = Number(data.get('recipient'))
    const title = String(data.get('title') || '').trim(), content = String(data.get('content') || '').trim()
    if (!Number.isSafeInteger(recipient) || recipient <= 0 || recipient === project) { setError('请填写另一已登记项目的编号。'); return }
    if (!title || !content || new TextEncoder().encode(title).length > 256 || new TextEncoder().encode(content).length > 8192) {
      setError('请填写脱敏标题和正文。标题最多 256 字节，正文最多 8192 字节。'); return
    }
    setBusy(true); setError('')
    const signal = lifetime.current?.signal
    try {
      const doc = await request<Receipt>(`/projects/${project}/shares`, { method: 'POST', signal,
        body: JSON.stringify({ material_key: material, revision, recipient, title, content }) })
      if (!signal?.aborted) location.hash = `/projects/${project}/shares/${doc.id}`
    } catch (e) { if (!signal?.aborted) setError(e instanceof ApiError && e.status === 409 ? '请核对固定修订并提炼可共享的经验，完整原文无法提交。' : e instanceof Error ? e.message : '无法提交副本。') }
    finally { if (!signal?.aborted) setBusy(false) }
  }
  return open ? <form className="material-form" onSubmit={save}><h3>起草脱敏副本</h3>
    <p>来源 · {material} · 修订 {revision}。接收项目将看到发布后的标题和正文。</p>
    <fieldset disabled={busy}>
      <label>接收项目编号<input type="number" name="recipient" min="1" step="1" required/></label>
      <label>脱敏标题<input name="title" required/></label>
      <label htmlFor="share-body">脱敏正文</label><textarea id="share-body" name="content" required rows={5}/>
      {error && <p role="alert">{error}</p>}
      <div className="detail-actions"><button className="button">提交 Management 审核</button>
        <button className="button" type="button" onClick={() => { setOpen(false); setError('') }}>取消起草</button></div>
    </fieldset></form> : <button className="button" onClick={() => setOpen(true)}>起草脱敏副本</button>
}

export function ProjectShares({ project, receipt, request }: { project: number; receipt?: number; request: ApiRequest }) {
  const [direction, setDirection] = useState('shared-experience')
  const selected = receipt ? 'shares' : direction
  return <div className="project-setup">
    <BackLink href={projectHref(project)}>返回项目概览</BackLink>
    <PageHeading title={`项目 #${project} · 共享经验`} actions={receipt ? <a className="button" href={`#/projects/${project}/shares`}>浏览共享经验</a> :
      <ThemedSelect label="共享方向" hideLabel value={direction} onChange={setDirection}
        options={[{value:'shared-experience',label:'接收的经验'},{value:'shares',label:'发出的副本'}]}/>}/>
    <ShareList key={selected} project={project} receipt={receipt} direction={selected} request={request}/>
  </div>
}

function ShareList({ project, receipt, direction, request }: { project: number; receipt?: number; direction: string; request: ApiRequest }) {
  const base = `/projects/${project}/${direction}`
  const [query, setQuery] = useState('')
  const [page, setPage] = useState<{ before: number; query?: string }>({ before: 0 })
  const [rows, setRows] = useState<(Copy | Receipt)[]>([])
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [readError, setReadError] = useState('')
  const [error, setError] = useState('')
  useEffect(() => {
    const c = new AbortController()
    setLoading(true); setReadError(''); setMore(false)
    if (!page.before) setRows([])
    const data = receipt ? request<Receipt>(base + '/' + receipt, { signal: c.signal }).then(r => [r]) :
      request<(Receipt | Copy)[]>(base + (page.query ? '/search?q=' + encodeURIComponent(page.query) : page.before ? '?before=' + page.before : ''), { signal: c.signal })
    data.then(items => {
      if (c.signal.aborted) return
      setRows(old => page.before ? old.concat(items.filter(i => !old.some(o => o.id === i.id))) : items)
      setMore(!receipt && !page.query && items.length === 30)
    }).catch(e => { if (!c.signal.aborted) setReadError(e.message) })
      .finally(() => { if (!c.signal.aborted) setLoading(false) })
    return () => c.abort()
  }, [base, receipt, request, page])
  async function publish(row: Receipt, action: 'publish' | 'revoke') {
    setBusy(true); setError('')
    try {
      const updated = await request<Receipt>(`${base}/${row.id}/publication`, {
        method: 'POST', body: JSON.stringify({ action, digest: row.digest }) })
      setRows(old => old.map(r => r.id === updated.id ? updated : r))
    } catch (e) { setError(e instanceof Error ? e.message : '无法更新发布记录。'); setPage({ before: 0 }) }
    finally { setBusy(false) }
  }
  return <section className="panel detail-card">
    {direction === 'shared-experience' && <form onSubmit={e => { e.preventDefault(); setPage({ before: 0, query: query.trim() }) }}>
      <label>检索已接收的经验<input type="search" maxLength={512} value={query} onChange={e => setQuery(e.target.value)}/></label>
      <button className="button">检索</button>{page.query && <button className="button" type="button" onClick={() => {setQuery(''); setPage({before: 0})}}>显示全部</button>}
    </form>}
    <div className="detail-actions"><button className="button" disabled={busy || loading} onClick={() => setPage({ before: 0, query: page.query })}>刷新共享记录</button>
      {direction === 'shares' && <a href={`#/projects/${project}/materials`}>从项目资料起草副本</a>}</div>
    {readError && <p role="alert">{readError}</p>}{error && <p role="alert">{error}</p>}
    {loading && <p role="status">正在读取共享记录…</p>}
    {!loading && !readError && !rows.length && <p>{direction === 'shares' ? '尚无发出的副本。' : page.query ? '没有匹配的已发布经验。' : '尚无已发布给本项目的经验。'}</p>}
    {rows.map(row => <article className="registered-project" key={row.id}><div><h3>#{row.id} · {row.title}</h3>
      <pre className="material-content">{row.content}</pre><p>副本摘要 · <code>{row.digest}</code></p>
      {row.published_at && <p>发布时间 · {formatTime(row.published_at)}</p>}
      {'project_id' in row && <>
        <p>来源 · {row.material_key} · 修订 {row.revision}　接收项目 #{row.recipient}</p>
        <p>{row.revoked_at ? '已撤销 · ' + formatTime(row.revoked_at) : row.published_at ? '已发布' : row.review ? verdicts[row.review.verdict] : '等待 Management 审核'}</p>
        <ManagementProcesses request={request} processes={row.review_processes}/>
        {row.review && <details><summary>审核依据 · {row.review.effective_level}</summary><p>{row.review.rationale}</p>
          <p>证据 · {row.review.evidence}</p><p>原生会话 · <code>{row.review.session_id}</code></p>
          {row.review.profile && <RolePermissions role="Management" profile={row.review.profile}/>}</details>}
        {!row.revoked_at && <div className="detail-actions">
          {row.review?.verdict === 'approve' && !row.published_at && <button className="button" disabled={busy || loading} onClick={() => void publish(row,'publish')}>发布给项目 #{row.recipient}</button>}
          <button className="button" disabled={busy || loading} onClick={() => void publish(row,'revoke')}>撤销副本</button>
        </div>}
      </>}
    </div></article>)}
    {more && <button className="button" disabled={busy || loading} onClick={() => setPage({ before: rows[rows.length-1].id })}>更多共享记录</button>}
  </section>
}
