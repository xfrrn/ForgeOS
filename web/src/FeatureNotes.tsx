import { useEffect, useRef, useState } from 'react'
import { ChatMarkdown } from './ChatMarkdown'
import { formatTime, Icon, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'

type Note = { id: number; turn_id: number; kind: 'agreement' | 'question'; body: string; closed: boolean }
type Source = { text: string; answer: string; created_at: string; requested_model: string; native_backend?: string }
type Props = { path: string; request: ApiRequest }

type NoteSuggestion = { turn_id: number; kind: 'agreement' | 'question'; body: string }
type CloseSuggestion = { id: number; body: string; reason: string }
type AssistJob = {
  id: number
  kind: string
  target_turn?: number
  status: 'queued' | 'running' | 'completed' | 'failed'
  error?: string
  suggestions?: NoteSuggestion[]
  close_suggestions?: CloseSuggestion[]
}
type CandidateNote = NoteSuggestion & { key: string; checked: boolean }
type CandidateClose = CloseSuggestion & { checked: boolean }

function useNoteExtractor({ path, request, profileId, model, onApplied }: {
  path: string
  request: ApiRequest
  profileId?: string
  model?: string
  onApplied: (summary: string) => void
}) {
  const [jobId, setJobId] = useState<number>()
  const [extracting, setExtracting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [emptyHint, setEmptyHint] = useState('')
  const [candidates, setCandidates] = useState<CandidateNote[]>([])
  const [closes, setCloses] = useState<CandidateClose[]>([])

  useEffect(() => {
    if (!jobId) return
    const controller = new AbortController()
    let timer: number
    async function poll() {
      try {
        const job = await request<AssistJob>(`${path}/extract/${jobId}/status`, { signal: controller.signal })
        if (controller.signal.aborted) return
        if (job.status === 'completed') {
          setJobId(undefined)
          setExtracting(false)
          const nextNotes = (job.suggestions || []).map(item => ({ ...item, key: crypto.randomUUID(), checked: true }))
          const nextCloses = (job.close_suggestions || []).map(item => ({ ...item, checked: true }))
          setCandidates(nextNotes)
          setCloses(nextCloses)
          if (!nextNotes.length && !nextCloses.length) {
            setEmptyHint('AI 未发现需要新增的约定或待处理的问题。')
          }
          return
        }
        if (job.status === 'failed') {
          setJobId(undefined)
          setExtracting(false)
          setError(job.error || 'AI 提炼失败，请重试。')
          return
        }
        timer = window.setTimeout(poll, 1500)
      } catch (e) {
        if (!controller.signal.aborted) {
          setJobId(undefined)
          setExtracting(false)
          setError(e instanceof Error ? e.message : '无法获取提炼进度。')
        }
      }
    }
    void poll()
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [jobId, path, request])

  async function start(turnId = 0) {
    if (extracting || saving || !profileId) return
    setExtracting(true)
    setError('')
    setEmptyHint('')
    setCandidates([])
    setCloses([])
    try {
      const job = await request<AssistJob>(`${path}/extract`, {
        method: 'POST',
        body: JSON.stringify({
          client_id: crypto.randomUUID(),
          profile_id: profileId,
          model: model || undefined,
          turn_id: turnId || undefined,
        }),
      })
      setJobId(job.id)
    } catch (e) {
      setExtracting(false)
      setError(e instanceof Error ? e.message : '发起 AI 提炼失败，请重试。')
    }
  }

  async function apply() {
    if (saving || extracting) return
    const items = candidates
      .filter(c => c.checked && c.body.trim())
      .map(c => ({ turn_id: c.turn_id, client_id: c.key, kind: c.kind, body: c.body.trim() }))
    const closeIds = closes.filter(c => c.checked).map(c => c.id)
    if (!items.length && !closeIds.length) return
    setSaving(true)
    setError('')
    try {
      const res = await request<{ saved: number; closed: number }>(`${path}/batch`, {
        method: 'POST',
        body: JSON.stringify({ items, close_ids: closeIds }),
      })
      setCandidates([])
      setCloses([])
      const parts: string[] = []
      if (res.saved > 0) parts.push(`已保存 ${res.saved} 条记录`)
      if (res.closed > 0) parts.push(`已关闭 ${res.closed} 条旧问题`)
      onApplied(parts.join('，') || '已更新话题记录')
    } catch (e) {
      setError(e instanceof Error ? e.message : '批量保存失败，请重试。')
    } finally {
      setSaving(false)
    }
  }

  function clear() {
    setJobId(undefined)
    setExtracting(false)
    setCandidates([])
    setCloses([])
    setError('')
    setEmptyHint('')
  }

  return { extracting, saving, error, emptyHint, candidates, setCandidates, closes, setCloses, start, apply, clear }
}

function CandidateReviewCard({
  title,
  showTurnBadge,
  candidates,
  setCandidates,
  closes,
  setCloses,
  saving,
  onApply,
  onClose,
}: {
  title: string
  showTurnBadge: boolean
  candidates: CandidateNote[]
  setCandidates: React.Dispatch<React.SetStateAction<CandidateNote[]>>
  closes: CandidateClose[]
  setCloses: React.Dispatch<React.SetStateAction<CandidateClose[]>>
  saving: boolean
  onApply: () => void
  onClose: () => void
}) {
  const checkedNotes = candidates.filter(c => c.checked && c.body.trim()).length
  const checkedCloses = closes.filter(c => c.checked).length
  const totalSelected = checkedNotes + checkedCloses
  return <section className="feature-assist-card" aria-label={title}>
    <div className="feature-assist-head">
      <strong><Icon name="Sparkles" size={14}/>{title}</strong>
      <button type="button" className="icon-button" aria-label="收起提炼结果" onClick={onClose} disabled={saving}>
        <Icon name="X" size={14}/>
      </button>
    </div>
    {!!candidates.length && <div className="feature-assist-list">
      {candidates.map((item, idx) => <div key={item.key} className={'feature-assist-item' + (item.checked ? '' : ' unchecked')}>
        <div className="feature-assist-row">
          <label className="feature-assist-check">
            <input type="checkbox" checked={item.checked} disabled={saving}
              onChange={e => setCandidates(old => old.map((c, i) => i === idx ? { ...c, checked: e.target.checked } : c))}/>
            <span>采用</span>
          </label>
          <ThemedSelect label="记录类型" hideLabel value={item.kind} disabled={saving}
            onChange={kind => setCandidates(old => old.map((c, i) => i === idx ? { ...c, kind: kind as 'agreement' | 'question' } : c))}
            options={[{ value: 'agreement', label: '已确认的约定' }, { value: 'question', label: '待决定的问题' }]}/>
          {showTurnBadge && <span className="feature-assist-turn">#{item.turn_id}</span>}
        </div>
        <textarea rows={3} maxLength={1200} value={item.body} disabled={saving}
          onChange={e => setCandidates(old => old.map((c, i) => i === idx ? { ...c, body: e.target.value } : c))}/>
      </div>)}
    </div>}
    {!!closes.length && <div className="feature-assist-closes">
      <h4>建议标为已处理的旧记录</h4>
      {closes.map((item, idx) => <label key={item.id} className="feature-assist-close-row">
        <input type="checkbox" checked={item.checked} disabled={saving}
          onChange={e => setCloses(old => old.map((c, i) => i === idx ? { ...c, checked: e.target.checked } : c))}/>
        <span>
          <strong>{item.body}</strong>
          {item.reason && <small>{item.reason}</small>}
        </span>
      </label>)}
    </div>}
    <div className="detail-actions">
      <button type="button" className="button primary" disabled={saving || totalSelected === 0} onClick={onApply}>
        {saving ? '正在保存…' : `应用所选（${totalSelected}）`}
      </button>
      <button type="button" className="button" disabled={saving} onClick={onClose}>取消</button>
    </div>
  </section>
}

export function RecordFeatureNote({ path, request, turn, onSaved, profileId, model, enabled }: Props & {
  turn: number
  onSaved: () => void
  profileId?: string
  model?: string
  enabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState('agreement')
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const intent = useRef({ key: '', id: '' })

  const ai = useNoteExtractor({
    path,
    request,
    profileId,
    model,
    onApplied: summary => {
      setMessage(summary)
      onSaved()
    },
  })

  async function save() {
    if (busy || !body.trim()) return
    const key = JSON.stringify([kind, body.trim()])
    if (intent.current.key !== key) intent.current = { key, id: crypto.randomUUID() }
    setBusy(true)
    setMessage('')
    try {
      await request(path, { method: 'POST', body: JSON.stringify({ turn_id: turn, kind, body: body.trim(), client_id: intent.current.id }) })
      setOpen(false)
      setBody('')
      setMessage('已保存到话题记录')
      intent.current = { key: '', id: '' }
      onSaved()
    } catch (e) { setMessage(e instanceof Error ? e.message : '保存失败，请重试。') }
    finally { setBusy(false) }
  }

  return <div className="feature-note-editor">
    {open ? <form onSubmit={event => { event.preventDefault(); void save() }}>
      <ThemedSelect label="记录类型" value={kind} disabled={busy} onChange={setKind} options={[
        { value: 'agreement', label: '已确认的约定' }, { value: 'question', label: '待决定的问题' }
      ]}/>
      <label>记录内容<textarea autoFocus required rows={4} maxLength={1200} value={body} disabled={busy} placeholder="写下已确认的约定或待决定的问题…" onChange={e => setBody(e.target.value)}/></label>
      <div className="detail-actions">
        <button className="button primary" disabled={busy || !body.trim()}>保存记录</button>
        <button type="button" className="button" disabled={busy} onClick={() => { setOpen(false); setMessage('') }}>收起</button>
      </div>
    </form> : <div className="detail-actions">
      <button className="button" type="button" onClick={() => { setOpen(true); setMessage('') }}>从本轮记一条</button>
      {enabled && profileId && <button className="button feature-assist-btn" type="button" disabled={ai.extracting || ai.saving}
        onClick={() => { setMessage(''); void ai.start(turn) }}>
        <Icon name="Sparkles" size={13}/>{ai.extracting ? 'AI 提炼中…' : 'AI 提炼本轮'}
      </button>}
    </div>}
    {(ai.candidates.length > 0 || ai.closes.length > 0) && <CandidateReviewCard
      title={`AI 提炼建议 · 回合 #${turn}`}
      showTurnBadge={false}
      candidates={ai.candidates}
      setCandidates={ai.setCandidates}
      closes={ai.closes}
      setCloses={ai.setCloses}
      saving={ai.saving}
      onApply={() => void ai.apply()}
      onClose={ai.clear}
    />}
    {ai.error && <p className="detail-problem" role="alert">{ai.error}</p>}
    {ai.emptyHint && <span className="muted" role="status">{ai.emptyHint}</span>}
    {message && <span className="feature-assist-status" role="status">{message}</span>}
  </div>
}

function NoteItem({ note, request, path, onClosed }: Props & { note: Note; onClosed: () => void }) {
  const [source, setSource] = useState<Source>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function act(action: 'source' | 'close') {
    setError('')
    if (action === 'source' && source) { setSource(undefined); return }
    setBusy(true)
    try {
      if (action === 'source') setSource(await request<Source>(path + '/' + note.id + '/source'))
      else { await request(path + '/' + note.id + '/close', { method: 'POST' }); onClosed() }
    } catch (e) { setError(e instanceof Error ? e.message : '操作失败，请重试。') }
    finally { setBusy(false) }
  }
  return <li><strong>{note.kind === 'agreement' ? '已确认的约定' : '待决定的问题'}</strong>
    <p>{note.body}</p>
    <div className="detail-actions">
      <button type="button" className="button" disabled={busy} aria-expanded={!!source} onClick={() => void act('source')}>{source ? '收起原文' : '查看原文'} · #{note.turn_id}</button>
      {!note.closed && <button type="button" className="button" disabled={busy} onClick={() => void act('close')}>{note.kind === 'question' ? '标为已处理' : '结束此约定'}</button>}
    </div>
    {source && <blockquote><small>{formatTime(source.created_at)} · 你</small><p>{source.text}</p>
      <small>{source.native_backend === 'claude' ? 'Claude Code · ' : source.native_backend === 'codex' ? 'Codex · ' : ''}{source.requested_model || '模型回复'}</small><ChatMarkdown content={source.answer}/></blockquote>}
    {error && <p role="alert">{error}</p>}
  </li>
}

export function FeatureNotes({ path, request, version, profileId, model, enabled, hasTurns, onChanged }: Props & {
  version: number
  profileId?: string
  model?: string
  enabled?: boolean
  hasTurns?: boolean
  onChanged?: () => void
}) {
  const [items, setItems] = useState<Note[]>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [history, setHistory] = useState(false)
  const [query, setQuery] = useState('')
  const [notice, setNotice] = useState('')
  const refresh = () => {
    setRevision(n => n + 1)
    onChanged?.()
  }
  const ai = useNoteExtractor({
    path,
    request,
    profileId,
    model,
    onApplied: summary => {
      setNotice(summary)
      refresh()
    },
  })
  useEffect(() => {
    const controller = new AbortController()
    request<Note[]>(path, { signal: controller.signal }).then(data => {
      if (!controller.signal.aborted) { setItems(data); setError('') }
    }).catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [path, request, version, revision])
  const active = items?.filter(n => !n.closed).length
  const visible = items?.filter(n => n.closed === history && n.body.includes(query.trim())).slice().reverse()
  return <details className="feature-notes" onToggle={e => { if (e.currentTarget.open) setRevision(n => n + 1) }}>
    <summary>话题记录{active !== undefined && ' · ' + active + ' 条有效'}</summary>
    <div className="feature-notes-content">
      <div className="detail-actions">
        <input type="search" aria-label="搜索话题记录" placeholder="搜索话题记录" value={query} onChange={e => setQuery(e.target.value)}/>
        <button className="button" type="button" aria-pressed={history} onClick={() => setHistory(!history)}>{history ? '返回有效记录' : '查看已结束记录'}</button>
        {enabled && profileId && <button className="button feature-assist-btn" type="button" disabled={!hasTurns || ai.extracting || ai.saving}
          onClick={() => { setNotice(''); void ai.start(0) }}>
          <Icon name="Sparkles" size={13}/>{ai.extracting ? 'AI 梳理中…' : 'AI 梳理近期讨论'}
        </button>}
      </div>
      {(ai.candidates.length > 0 || ai.closes.length > 0) && <CandidateReviewCard
        title="AI 梳理近期讨论建议"
        showTurnBadge={true}
        candidates={ai.candidates}
        setCandidates={ai.setCandidates}
        closes={ai.closes}
        setCloses={ai.setCloses}
        saving={ai.saving}
        onApply={() => void ai.apply()}
        onClose={ai.clear}
      />}
      {ai.error && <p className="detail-problem" role="alert">{ai.error}</p>}
      {ai.emptyHint && <p className="muted" role="status">{ai.emptyHint}</p>}
      {notice && <p className="feature-assist-status" role="status">{notice}</p>}
      {error && <p role="alert">{error} <button className="button" onClick={() => setRevision(n => n + 1)}>重试</button></p>}
      {!items && !error && <p role="status">正在读取话题记录…</p>}
      {visible?.length === 0 && <p className="muted">{query ? '没有匹配的记录。' : history ? '暂无已结束的记录。' : '在一轮已完成的讨论下，可点「AI 提炼本轮」或「从本轮记一条」保存约定或问题。'}</p>}
      <ul>{visible?.map(note => <NoteItem key={note.id} note={note} path={path} request={request} onClosed={refresh}/>)}</ul>
    </div>
  </details>
}
