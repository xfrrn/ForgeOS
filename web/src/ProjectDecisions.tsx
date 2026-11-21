import { ThemedSelect } from './ThemedSelect'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, formatTime, projectHref, type ApiRequest } from './shared'
import { KnowledgeCuration } from './KnowledgeCuration'
import { PageHeading } from './ui'
import { MaterialReader } from './MaterialReader'
import './decision-library.css'

const labels = { PROPOSED: '待确认', CONFIRMED: '已确认', REJECTED: '已拒绝', SUPERSEDED: '已替代', EXPIRED: '已失效' }
type State = keyof typeof labels
type Decision = {
  id: number; statement: string; source: string; validity: string; valid_until?: string; status: State
  evidence_refs: string[]; supersedes?: number; decided_by?: string; history: { status: State; actor: string; at: string }[]
  versions?: {id: number; status: State; statement: string; supersedes?: number}[]
}

function localDeadline(value?: string) {
  if (!value) return ''
  const d = new Date(value)
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 19)
}

function decisionTitle(text: string) {
  const title = text.split(/\r?\n/).find(line => line.trim())?.replace(/^#{1,6}\s+/, '') || text
  return title.length > 110 ? title.slice(0, 110) + '…' : title
}

export function ProjectDecisions({ project, decision, request }: { project: number; decision?: number; request: ApiRequest }) {
  const base = `/projects/${project}/decisions`
  const library = projectHref(project, '/knowledge/decisions')
  const [page, setPage] = useState(() => {
    const params = new URLSearchParams(location.hash.split('?')[1])
    const status = params.get('status') || 'CONFIRMED'
    return {status: Object.hasOwn(labels, status) || status === 'ALL' ? status : 'CONFIRMED', before: 0, query: params.get('q') || ''}
  })
  const [query, setQuery] = useState(page.query)
  const [rows, setRows] = useState<Decision[]>([])
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [readError, setReadError] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [proposal, setProposal] = useState<{ replaces?: Decision }>()
  const [draftText, setDraftText] = useState('')
  const [selected, setSelected] = useState<Decision>()
  const [detailError, setDetailError] = useState('')
  const selectedId = decision || rows[0]?.id
  const suffix = new URLSearchParams({status:page.status, ...(page.query ? {q:page.query} : {})}).toString()
  const detailHref = (id: number) => library + '/' + id + '?' + suffix
  const retry = useRef<{ payload: string; key: string } | null>(null)
  const formHeading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { if (proposal) formHeading.current?.focus() }, [proposal])
  useEffect(() => {
    const controller = new AbortController()
    const q = new URLSearchParams({ status: page.status })
    if (page.query) q.set('q', page.query)
    if (page.before) q.set('before', String(page.before))
    setReadError(''); setLoading(true); setMore(false)
    if (!page.before) setRows([])
    request<Decision[]>(base + (page.query ? '/search?' : '?') + q, { signal: controller.signal }).then(items => {
      if (controller.signal.aborted) return
      setRows(old => page.before ? [...old, ...items.filter(i => !old.some(row => row.id === i.id))] : items)
      setMore(!page.query && items.length === 30)
    }).catch(e => { if (!controller.signal.aborted) setReadError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [base, request, page])
  useEffect(() => {
    const controller = new AbortController()
    setSelected(undefined); setDetailError('')
    if (selectedId) request<Decision>(base + '/' + selectedId + '?versions=1', {signal:controller.signal})
      .then(row => { if (!controller.signal.aborted) setSelected(row) })
      .catch(e => { if (!controller.signal.aborted) setDetailError(e.message) })
    return () => controller.abort()
  }, [base, selectedId, page, request])
  useEffect(() => {
    if (loading || readError) return
    const deadlines = (selected ? [...rows, selected] : rows).filter(r => r.valid_until && (r.status === 'CONFIRMED' || r.status === 'PROPOSED'))
      .map(r => Date.parse(r.valid_until!)).filter(Number.isFinite)
    if (!deadlines.length) return
    const delay = Math.max(2000, Math.min(...deadlines) - Date.now() + 250)
    const timer = setTimeout(() => setPage(p => ({ ...p, before: 0 })), Math.min(delay, 2147483647))
    return () => clearTimeout(timer)
  }, [rows, selected, loading, readError])
  function refresh(status = page.status) { setPage(p => ({ ...p, status, before: 0 })) }
  function filterList(status: string, text: string) {
    const params = new URLSearchParams({status})
    if (text) params.set('q', text)
    const url = library + '?' + params
    if (decision) location.hash = url
    else history.replaceState(history.state, '', url)
    setPage({status, query:text, before:0})
  }
  function start(replaces?: Decision) {
    retry.current = null
    setDraftText(replaces?.statement || ''); setProposal({ replaces }); setNotice(''); setError('')
  }
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!proposal || busy) return
    const data = new FormData(e.currentTarget)
    const field = (name: string) => String(data.get(name) || '').trim()
    const refs = field('evidence').split(/\r?\n/).map(s => s.trim()).filter(Boolean)
    const until = field('until')
    const input = { statement: field('statement'), source: field('source'), validity: field('validity'), evidence_refs: refs,
      valid_until: until && until !== localDeadline(proposal.replaces?.valid_until) ? new Date(until).toISOString() :
        until ? proposal.replaces?.valid_until : undefined, supersedes: proposal.replaces?.id }
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
      location.hash = library + '/' + saved.id + '?status=' + saved.status
    } catch (e) { setError(e instanceof Error ? e.message : '无法确认提案保存结果，请重试。') }
    finally { setBusy(false) }
  }
  async function decide(id: number, status: State) {
    setBusy(true); setError(''); setNotice('')
    try {
      const result = await request<Decision>(base + '/' + id + '/status', { method: 'POST', body: JSON.stringify({ status }) })
      setNotice(`决策 #${id} · ${labels[result.status]}`); refresh(result.status)
      if (decision) {
        const params = new URLSearchParams({status:result.status})
        if (page.query) params.set('q',page.query)
        history.replaceState(history.state,'',library+'/'+id+'?'+params)
      }
    } catch (e) {
      setError(e instanceof ApiError && e.status === 409 ? '决策状态已变化，请核对最新记录与替代对象。' : e instanceof Error ? e.message : '无法更新决策。')
      refresh()
    } finally { setBusy(false) }
  }
  return <div className="project-setup">
    <PageHeading title="项目决策"/>
    {error && <p role="alert" className="detail-problem">{error}</p>}
    {notice && <p role="status">{notice}</p>}
    {proposal && <form key={proposal.replaces?.id ?? 'new'} onSubmit={submit} className="panel detail-card material-form">
      <h2 ref={formHeading} tabIndex={-1}>{proposal.replaces ? '提议替代决策 #' + proposal.replaces.id : '提出决策'}</h2>
      <fieldset disabled={busy}>
        <label htmlFor="decision-text">决定内容</label><textarea name="statement" id="decision-text" required rows={5} value={draftText} onChange={e=>setDraftText(e.target.value)}/>
        {draftText && <details><summary>{proposal.replaces ? '比较原决定与拟议版' : '预览 Markdown'}</summary>
          {proposal.replaces && <><h3>当前决定</h3><MaterialReader markdown content={proposal.replaces.statement}/><h3>拟议版</h3></>}
          <MaterialReader markdown content={draftText}/></details>}
        <label>来源<input name="source" required defaultValue={proposal.replaces?.source}/></label>
        <label>适用条件<input name="validity" required defaultValue={proposal.replaces?.validity}/></label>
        <label htmlFor="decision-evidence">证据引用（每行一项）</label><textarea name="evidence" id="decision-evidence" required rows={3} defaultValue={proposal.replaces?.evidence_refs.join('\n')}/>
        <label>截止时间（本地时间，可选）<input name="until" type="datetime-local" step="1" defaultValue={localDeadline(proposal.replaces?.valid_until)}/></label>
        <div className="detail-actions"><button className="button">保存提案</button><button className="button" type="button" onClick={() => { setProposal(undefined); setError('') }}>取消编辑</button></div>
      </fieldset></form>}
    <section className="panel detail-card decision-library-toolbar">
      <form className="detail-actions" onSubmit={e => { e.preventDefault(); filterList(page.status,query.trim()) }}>
        <input type="search" aria-label="检索项目决策" placeholder="检索决定、来源、适用条件" value={query} onChange={e=>setQuery(e.target.value)}/>
        <button className="button">检索</button>
      </form>
      <ThemedSelect label="决策状态" hideLabel value={page.status} onChange={value=>filterList(value,page.query)}
        options={[...Object.entries(labels).map(([value,label])=>({value,label})),{value:'ALL',label:'全部状态'}]}/>
      <button className="button" disabled={loading} onClick={()=>refresh()}>刷新</button>
      <button className="button primary" disabled={busy || Boolean(proposal)} onClick={()=>start()}>提出决策</button>
    </section>
    <div className={'decision-library-layout ' + (decision ? 'decision-show-record' : 'decision-show-list')}>
      <section aria-label="决策列表" className="panel detail-card decision-library-list">
        {readError && <p role="alert">{readError}</p>}
        {loading && <p role="status">正在读取决策…</p>}
        {!loading && !readError && !rows.length && <p>没有符合条件的决策。</p>}
        {rows.map(row=><a className="decision-list-item" key={row.id} href={detailHref(row.id)} aria-current={selectedId === row.id ? 'page' : undefined}>
          <small>#{row.id} · {labels[row.status]}</small><strong>{decisionTitle(row.statement)}</strong>
          <small>{row.history?.at(-1) && formatTime(row.history.at(-1)!.at)}</small>
        </a>)}
        {more && <button className="button" disabled={loading} onClick={()=>setPage({...page,before:rows[rows.length-1].id})}>更多决策</button>}
      </section>
      <section aria-label="决策详情" className="panel detail-card decision-library-record">
        {decision && <a className="decision-back" href={library + '?' + suffix}>返回决策列表</a>}
        {detailError && <p role="alert">{detailError} <button className="button" onClick={()=>refresh()}>重试</button></p>}
        {!selected && !detailError && <p>{selectedId ? '正在读取决策…' : '选择一条决策查看详情。'}</p>}
        {selected && <article key={selected.id}>
          <h2>决策 #{selected.id} · {decisionTitle(selected.statement)}</h2><p>{labels[selected.status]}{selected.decided_by && ' · ' + selected.decided_by}</p>
          <MaterialReader markdown content={selected.statement}/>
          <p>适用条件 · {selected.validity}</p><p>来源 · {selected.source}</p>
          {selected.valid_until && <p>截止时间 · {formatTime(selected.valid_until)}</p>}
          {selected.supersedes && <p>替代对象 · <a href={detailHref(selected.supersedes)}>决策 #{selected.supersedes}</a></p>}
          {selected.status === 'CONFIRMED' && <p>计划引用 · <code>{base}/{selected.id}</code> <button type="button" className="button"
            onClick={()=>navigator.clipboard.writeText(base + '/' + selected.id).then(()=>setNotice('已复制计划引用。'),()=>setNotice('请复制上方的计划引用。'))}>复制</button></p>}
          <details><summary>证据与历史</summary><ul>{selected.evidence_refs.map((ref,i)=><li key={i}>{ref}</li>)}</ul>
            <ol>{selected.history.map(h=><li key={h.status}>{formatTime(h.at)} · {h.actor} · {labels[h.status]}</li>)}</ol></details>
          {(selected.versions?.length ?? 0)>1 && <details open><summary>历次决定与替代提案</summary><ul>
            {selected.versions?.map(v=><li key={v.id}><a href={detailHref(v.id)} aria-current={v.id === selected.id ? 'page' : undefined}>#{v.id} · {labels[v.status]} · {decisionTitle(v.statement)}</a></li>)}
          </ul></details>}
          {selected.status === 'PROPOSED' && <div className="detail-actions"><button className="button primary" disabled={busy} onClick={()=>void decide(selected.id,'CONFIRMED')}>确认决策</button>
            <button className="button" disabled={busy} onClick={()=>void decide(selected.id,'REJECTED')}>拒绝提案</button></div>}
          {selected.status === 'CONFIRMED' && <div className="detail-actions"><button className="button" disabled={busy || Boolean(proposal)} onClick={()=>start(selected)}>提议替代</button>
            <button className="button" disabled={busy} onClick={()=>void decide(selected.id,'EXPIRED')}>标记失效</button></div>}
          {selected.history.some(h=>h.status === 'CONFIRMED') &&
            <KnowledgeCuration project={project} path={base + '/' + selected.id + '/curation'} request={request}/>}
        </article>}
      </section>
    </div></div>
}
