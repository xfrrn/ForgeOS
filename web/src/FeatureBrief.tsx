import { useEffect, useState } from 'react'
import { ChatMarkdown } from './ChatMarkdown'
import { formatTime, Icon, projectHref, type ApiRequest } from './shared'

export type FeatureBriefState = {
  brief: string
  updated_at?: string
  source_turn?: number
  latest_turn?: number
  active_agreements: number
  open_questions: number
  job?: { id: number; status: 'queued' | 'running' | 'completed' | 'failed'; error?: string }
}

export function FeatureBrief({ projectId, featureId, request, version, profileId, model, enabled, hasTurns }: {
  projectId: number
  featureId: number
  request: ApiRequest
  version: number
  profileId?: string
  model?: string
  enabled?: boolean
  hasTurns?: boolean
}) {
  const path = `/projects/${projectId}/features/${featureId}/brief`
  const [state, setState] = useState<FeatureBriefState>()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)

  const generating = state?.job?.status === 'queued' || state?.job?.status === 'running'
  const stale = !!state?.brief && !!state.latest_turn && (state.source_turn || 0) < state.latest_turn

  useEffect(() => {
    const controller = new AbortController()
    let timer: number
    async function read() {
      try {
        const next = await request<FeatureBriefState>(path, { signal: controller.signal })
        if (controller.signal.aborted) return
        setState(next)
        setError('')
        if (next.job?.status === 'queued' || next.job?.status === 'running') {
          timer = window.setTimeout(read, 2000)
        }
      } catch (e) {
        if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '无法读取本轮需求终稿。')
      }
    }
    void read()
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [path, request, version, revision])

  async function generate() {
    if (busy || generating || !profileId) return
    setBusy(true)
    setError('')
    setEditing(false)
    setOpen(true)
    try {
      await request(`${path}/generate`, {
        method: 'POST',
        body: JSON.stringify({ client_id: crypto.randomUUID(), profile_id: profileId, model: model || undefined }),
      })
      setRevision(n => n + 1)
    } catch (e) {
      setError(e instanceof Error ? e.message : '发起需求终稿整理失败，请重试。')
    } finally {
      setBusy(false)
    }
  }

  async function save() {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      const next = await request<FeatureBriefState>(path, {
        method: 'PUT',
        body: JSON.stringify({ brief: draft }),
      })
      setState(next)
      setEditing(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存需求终稿失败，请重试。')
    } finally {
      setBusy(false)
    }
  }

  function goToProposal() {
    window.location.hash = projectHref(projectId, `/features/${featureId}?view=proposal&compose=1`)
  }

  return <section className="feature-brief" aria-label="本轮需求终稿">
    <div className="feature-brief-bar">
      <button type="button" className="feature-brief-toggle" aria-expanded={open} onClick={() => setOpen(v => !v)}>
        <Icon name="ClipboardCheck" size={14}/>
        <strong>本轮需求终稿</strong>
        {generating ? <span className="feature-brief-pill is-running">AI 正在整理…</span> :
          state?.brief ? <span className={'feature-brief-pill' + (stale ? ' is-stale' : ' is-ready')}>{stale ? '已整理 · 有新讨论可更新' : '已整理'}</span> :
          <span className="feature-brief-pill">未整理</span>}
        <Icon name="ChevronDown" size={14} className={'feature-brief-chevron' + (open ? ' open' : '')}/>
      </button>
      <div className="feature-brief-quick">
        {!state?.brief ? (enabled && profileId && <button type="button" className="button feature-assist-btn"
          disabled={busy || generating || (!hasTurns && !(state?.active_agreements || state?.open_questions))}
          onClick={() => void generate()}>
          <Icon name="Sparkles" size={13}/>{generating ? '整理中…' : '收口生成本轮需求'}
        </button>) : <>
          {stale && enabled && profileId && <button type="button" className="button feature-assist-btn" disabled={busy || generating} onClick={() => void generate()}>
            <Icon name="Sparkles" size={13}/>更新终稿
          </button>}
          <button type="button" className="button primary feature-brief-cta" onClick={goToProposal}>
            带入方案与拍板<Icon name="ArrowRight" size={13}/>
          </button>
        </>}
      </div>
    </div>

    {open && <div className="feature-brief-body">
      {!!state?.open_questions && <p className="feature-brief-notice">
        <Icon name="AlertCircle" size={14}/>当前还有 {state.open_questions} 条待决定问题，可继续讨论收敛，或直接带入「方案与拍板」由双模型权衡。
      </p>}
      {state?.job?.status === 'failed' && <p className="detail-problem" role="alert">{state.job.error || '整理需求终稿失败，请重试。'}</p>}
      {error && <p className="detail-problem" role="alert">{error}</p>}
      {generating && <p className="muted" role="status">正在基于近期讨论与有效话题记录整理本轮需求终稿…</p>}
      {editing ? <div className="feature-brief-editor">
        <textarea rows={12} maxLength={4000} value={draft} disabled={busy} onChange={e => setDraft(e.target.value)}
          placeholder="填写本轮需求终稿（背景与目标、范围与约束、关键交互、待权衡问题、验收标准）…"/>
        <div className="detail-actions">
          <button type="button" className="button primary" disabled={busy} onClick={() => void save()}>{busy ? '正在保存…' : '保存终稿'}</button>
          <button type="button" className="button" disabled={busy} onClick={() => setEditing(false)}>取消</button>
        </div>
      </div> : state?.brief ? <>
        {state.updated_at && <small className="muted">更新于 {formatTime(state.updated_at)}</small>}
        <div className="feature-brief-preview"><ChatMarkdown content={state.brief}/></div>
        <div className="detail-actions">
          <button type="button" className="button primary" onClick={goToProposal}>
            用此需求发起方案与拍板<Icon name="ArrowRight" size={14}/>
          </button>
          {enabled && profileId && <button type="button" className="button feature-assist-btn" disabled={busy || generating} onClick={() => void generate()}>
            <Icon name="Sparkles" size={13}/>{generating ? '正在整理…' : '按最新讨论重新整理'}
          </button>}
          <button type="button" className="button" disabled={busy || generating} onClick={() => { setDraft(state.brief); setEditing(true) }}>编辑终稿</button>
        </div>
      </> : !generating && <div className="feature-brief-empty">
        <p className="muted">讨论达成共识后，可让 AI 一键整合讨论记录与「话题记录」生成本轮需求终稿，确认后直接带入「方案与拍板」。</p>
        <div className="detail-actions">
          {enabled && profileId && <button type="button" className="button primary" disabled={busy || (!hasTurns && !(state?.active_agreements || state?.open_questions))} onClick={() => void generate()}>
            <Icon name="Sparkles" size={14}/>AI 收口生成本轮需求
          </button>}
          <button type="button" className="button" onClick={() => { setDraft(''); setEditing(true) }}>手动编写</button>
        </div>
      </div>}
    </div>}
  </section>
}
