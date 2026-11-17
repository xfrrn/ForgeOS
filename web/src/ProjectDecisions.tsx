import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, formatTime, type ApiRequest } from './shared'
import { KnowledgeCuration } from './KnowledgeCuration'

const labels = { PROPOSED: '待确认', CONFIRMED: '已确认', REJECTED: '已拒绝', SUPERSEDED: '已替代', EXPIRED: '已失效' }
type State = keyof typeof labels
type Decision = {
  id: number; statement: string; source: string; validity: string; valid_until?: string; status: State
  evidence_refs: string[]; supersedes?: number; decided_by?: string; history: { status: State; actor: string; at: string }[]
}

export function ProjectDecisions({ project, decision, request }: { project: number; decision?: number; request: ApiRequest }) {
  const base = `/projects/${project}/decisions`
  const [page, setPage] = useState({ status: 'CONFIRMED', before: 0 })
  const [rows, setRows] = useState<Decision[]>([])
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [readError, setReadError] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [proposal, setProposal] = useState<{ replaces?: Decision }>()
  const retry = useRef<{ payload: string; key: string } | null>(null)
  const formHeading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { if (proposal) formHeading.current?.focus() }, [proposal])
  useEffect(() => {
    const controller = new AbortController()
    const q = new URLSearchParams({ status: page.status })
    if (page.before) q.set('before', String(page.before))
    setReadError(''); setLoading(true); setMore(false)
    if (!page.before) setRows([])
    const read = decision
      ? request<Decision>(base + '/' + decision, { signal: controller.signal }).then(row => [row])
      : request<Decision[]>(base + '?' + q, { signal: controller.signal })
    read.then(items => {
      if (controller.signal.aborted) return
      setRows(old => page.before ? [...old, ...items.filter(i => !old.some(row => row.id === i.id))] : items)
      setMore(items.length === 30)
    }).catch(e => { if (!controller.signal.aborted) setReadError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [base, decision, request, page])
  useEffect(() => {
    if (loading || readError) return
    const deadlines = rows.filter(r => r.valid_until && (r.status === 'CONFIRMED' || r.status === 'PROPOSED'))
      .map(r => Date.parse(r.valid_until!)).filter(Number.isFinite)
    if (!deadlines.length) return
    const delay = Math.max(2000, Math.min(...deadlines) - Date.now() + 250)
    const timer = setTimeout(() => setPage(p => ({ status: p.status, before: 0 })), Math.min(delay, 2147483647))
    return () => clearTimeout(timer)
  }, [rows, loading, readError])
  function refresh(status = page.status) { setPage({ status, before: 0 }) }
  function start(replaces?: Decision) {
    retry.current = null
    setProposal({ replaces }); setNotice(''); setError('')
  }
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!proposal || busy) return
    const data = new FormData(e.currentTarget)
    const field = (name: string) => String(data.get(name) || '').trim()
    const refs = field('evidence').split(/\r?\n/).map(s => s.trim()).filter(Boolean)
    const until = field('until')
    const input = { statement: field('statement'), source: field('source'), validity: field('validity'), evidence_refs: refs,
      valid_until: until ? new Date(until).toISOString() : undefined, supersedes: proposal.replaces?.id }
    const bytes = (s: string) => new TextEncoder().encode(s).length
    if (!input.statement || !input.source || !input.validity || !refs.length || refs.length > 16 ||
      bytes(input.statement) > 4096 || bytes(input.source) > 1024 || bytes(input.validity) > 1024 || refs.some(s => bytes(s) > 1024)) {
      setError('请填写决定、来源、适用条件及 1 至 16 条证据。决定最多 4096 字节，其余各项最多 1024 字节。'); return
    }
    const payload = JSON.stringify(input)
    if (retry.current?.payload !== payload) {
      retry.current = { payload, key: Array.from(crypto.getRandomValues(new Uint8Array(16)), n => n.toString(16).padStart(2,'0')).join('') }
    }
    setBusy(true); setError(''); setNotice('')
    try {
      const saved = await request<Decision>(base, { method: 'POST', body: JSON.stringify({ ...input, request_key: retry.current.key }) })
      setProposal(undefined); setNotice(`已登记决策 #${saved.id} · ${labels[saved.status]}`); refresh(saved.status)
      if (decision) location.hash = base + '/' + saved.id
    } catch (e) { setError(e instanceof Error ? e.message : '无法确认提案保存结果，请重试。') }
    finally { setBusy(false) }
  }
  async function decide(id: number, status: State) {
    setBusy(true); setError(''); setNotice('')
    try {
      const result = await request<Decision>(base + '/' + id + '/status', { method: 'POST', body: JSON.stringify({ status }) })
      setNotice(`决策 #${id} · ${labels[result.status]}`); refresh(result.status)
    } catch (e) {
      setError(e instanceof ApiError && e.status === 409 ? '决策状态已变化，请核对最新记录与替代对象。' : e instanceof Error ? e.message : '无法更新决策。')
      refresh()
    } finally { setBusy(false) }
  }
  return <div className="project-setup"><a href="#/projects">返回项目接入</a>
    <div className="page-heading"><h1>项目 #{project} · 项目决策</h1></div>
    {error && <p role="alert" className="detail-problem">{error}</p>}
    {notice && <p role="status">{notice}</p>}
    {proposal && <form onSubmit={submit} className="panel detail-card material-form">
      <h2 ref={formHeading} tabIndex={-1}>{proposal.replaces ? '提议替代决策 #' + proposal.replaces.id : '提出决策'}</h2>
      <fieldset disabled={busy}>
        {proposal.replaces && <p>{proposal.replaces.statement}</p>}
        <label htmlFor="decision-text">决定内容</label><textarea name="statement" id="decision-text" required rows={3}/>
        <label>来源<input name="source" required/></label>
        <label>适用条件<input name="validity" required/></label>
        <label htmlFor="decision-evidence">证据引用（每行一项）</label><textarea name="evidence" id="decision-evidence" required rows={3}/>
        <label>截止时间（本地时间，可选）<input name="until" type="datetime-local"/></label>
        <div className="detail-actions"><button className="button">保存提案</button><button className="button" type="button" onClick={() => { setProposal(undefined); setError('') }}>取消编辑</button></div>
      </fieldset></form>}
    <section className="panel detail-card">{decision ? <a href={'#' + base}>查看本项目全部决策</a> : <div className="project-input">
      <select aria-label="决策状态" value={page.status} onChange={e => refresh(e.target.value)}>
        {Object.entries(labels).map(([k,v]) => <option key={k} value={k}>{v}</option>)}<option value="ALL">全部状态</option>
      </select><button className="button" disabled={loading} onClick={() => refresh()}>刷新</button>
      <button className="button" disabled={busy || Boolean(proposal)} onClick={() => start()}>提出决策</button></div>}
      {readError && <p role="alert">{readError}</p>}
      {loading && <p role="status">正在读取决策…</p>}
      {!loading && !readError && !rows.length && <p>没有符合条件的决策。</p>}
      {rows.map(row => <article key={row.id} className="registered-project"><div>
        <h3>{decision ? '#' + row.id : <a href={'#' + base + '/' + row.id}>#{row.id}</a>} · {row.statement}</h3><p>{labels[row.status]}{row.decided_by && ' · ' + row.decided_by}</p>
        {row.status === 'CONFIRMED' && <p>计划引用 · <code>{base}/{row.id}</code></p>}
        <p>来源 · {row.source}</p><p>适用条件 · {row.validity}</p>
        {row.valid_until && <p>截止时间 · {formatTime(row.valid_until)}</p>}{row.supersedes && <p>替代对象 · #{row.supersedes}</p>}
        <details><summary>证据与历史</summary><ul>{row.evidence_refs.map((ref,i) => <li key={i}>{ref}</li>)}</ul>
          <ol>{row.history.map(h => <li key={h.status}>{formatTime(h.at)} · {h.actor} · {labels[h.status]}</li>)}</ol></details>
        {decision && row.history.some(h => h.status === 'CONFIRMED') &&
          <KnowledgeCuration project={project} path={base + '/' + row.id + '/curation'} request={request}/>}
        {row.status === 'PROPOSED' && <div className="detail-actions"><button className="button" disabled={busy} onClick={() => void decide(row.id,'CONFIRMED')}>确认决策</button>
          <button className="button" disabled={busy} onClick={() => void decide(row.id,'REJECTED')}>拒绝提案</button></div>}
        {row.status === 'CONFIRMED' && <div className="detail-actions"><button className="button" disabled={busy || Boolean(proposal)} onClick={() => start(row)}>提议替代</button>
          <button className="button" disabled={busy} onClick={() => void decide(row.id,'EXPIRED')}>标记失效</button></div>}
      </div></article>)}
      {more && <button className="button" disabled={loading} onClick={() => setPage({ ...page, before: rows[rows.length-1].id })}>更多决策</button>}
    </section></div>
}
