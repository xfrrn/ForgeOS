import { useEffect, useRef, useState, type FormEvent } from 'react'
import { KnowledgeBrowser, KnowledgeComparison, KnowledgeSearch, LibraryNotice, useKnowledgeDetail, useKnowledgeList } from './KnowledgeBrowser'
import { MaterialReader } from './MaterialReader'
import { KnowledgeCuration } from './KnowledgeCuration'
import { ShareDraft } from './ShareDraft'
import { ApiError, formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { PageHeading } from './ui'
import { ThemedSelect } from './ThemedSelect'

const states = { current: '当前有效', pending: '待处理', withdrawn: '已撤回', stale: '来源已失效', rejected: '未采纳' }
const stateTones: Record<keyof typeof states, string> = { current: 'positive', pending: 'warning', withdrawn: 'neutral', stale: 'danger', rejected: 'danger' }
const changes = { pending: '审核中', ready: '待选择审核事实', rejected: '已退回', applied: '已采纳', obsolete: '已有其他修订', discarded: '已放弃' }
const filters = [{ value: 'current', label: '当前有效' }, { value: 'pending', label: '待处理' }, { value: 'history', label: '历史' }, { value: 'all', label: '全部' }]

type Experience = {
  id: number; current_revision: number; status: keyof typeof states; title: string; statement?: string; quote?: string; source?: string
  validity: string; reviewed_at?: string; updated_at: string; has_pending: boolean
}
type Revision = { revision: number; title: string; statement: string; quote: string; source: string; validity: string; receipt_path: string; actor: string; change_reason: string; created_at: string; is_current: boolean }
type Change = {
  id: number; expected_revision: number; title: string; source: string; content: string; reason: string; status: keyof typeof changes
  validity: string; facts: { statement: string; quote: string }[]; review?: { rationale?: string }; created_at: string; receipt_path?: string
}
type Entry = Experience & { source_retro?: number; revisions: Revision[]; changes: Change[] }
type EditorState = { entryID?: number; revision: number; title: string; validity: string; source: string; content: string }

function comparisonText(title: string, source: string, validity: string, content: string) {
  return [title, '来源 · ' + source, '适用场景 · ' + (validity || '未填写'), '', content].join('\n')
}

export function ProjectExperience({ project, entry, request }: { project: number; entry?: number; request: ApiRequest }) {
  const api = '/projects/' + project + '/knowledge/experience'
  const href = projectHref(project, '/knowledge/experience')
  const [filter, setFilter] = useState('current'), [text, setText] = useState(''), [query, setQuery] = useState('')
  const [revision, setRevision] = useState(0)
  const [editor, setEditor] = useState<EditorState>()
  const [notice, setNotice] = useState('')
  const params = new URLSearchParams({ status: filter })
  if (query) params.set('q', query)
  const list = useKnowledgeList<Experience>(request, api + '?' + params, revision)
  const selected = entry ?? list.rows[0]?.id
  const detail = useKnowledgeDetail<Entry>(request, selected ? api + '/' + selected : undefined, revision)
  const refresh = () => setRevision(r => r + 1)
  const leave = () => { setEditor(undefined); setNotice(''); location.hash = href }
  const editing = editor && (!editor.entryID || editor.entryID === selected)

  function start(item?: Entry) {
    setNotice('')
    location.hash = item ? href + '/' + item.id : href
    const prev = item?.revisions.find(r => r.revision === item.current_revision)
    const draft = item?.changes[0]
    setEditor({
      entryID: item?.id, revision: item?.current_revision ?? 0, title: item?.title ?? '',
      source: prev?.source ?? draft?.source ?? '', validity: item?.validity ?? '',
      content: prev?.statement ?? draft?.content ?? '',
    })
  }
  function saved(doc: Entry) {
    setEditor(undefined); setNotice('已提交审核。审核通过后，可在此选择并采纳事实。')
    location.hash = href + '/' + doc.id
    refresh()
  }

  return <div className="knowledge-page">
    <PageHeading title="项目经验" description="从实践、复盘与资料中沉淀带原文引句的工程经验，供 Agent 按需召回。"
      actions={<button className="button primary" onClick={() => start()}><Icon name="Plus" size={15}/>记录经验</button>}/>
    {notice && <p className="detail-notice" role="status">{notice}</p>}
    <KnowledgeBrowser detailActive={Boolean(entry || editing)} back={leave}
      toolbar={<KnowledgeSearch query={text} setQuery={setText} placeholder="搜索经验、来源或引句" refresh={refresh}
        search={q => { setQuery(q); leave() }}
        filter={<ThemedSelect label="经验状态" hideLabel value={filter} options={filters} onChange={f => { setFilter(f); leave() }}/>}/>}
      catalog={<>
        {list.rows.map(row => <a key={row.id} className="knowledge-row" href={href + '/' + row.id}
          aria-current={selected === row.id && !editing ? 'true' : undefined} onClick={() => { setEditor(undefined); setNotice('') }}>
          <div className="knowledge-row-top">
            <span className="knowledge-row-id">#{row.id}</span>
            <span className={'kb-badge ' + stateTones[row.status]}>{states[row.status]}</span>
          </div>
          <strong className="knowledge-row-title">{row.title}</strong>
          {row.statement && row.statement !== row.title && <small className="knowledge-row-sub">{row.statement}</small>}
          <small className="knowledge-row-meta">
            {formatTime(row.updated_at)}
            {row.has_pending && row.status !== 'pending' && ' · 有待处理更正'}
          </small>
        </a>)}
        <LibraryNotice {...list} retry={refresh} empty={list.rows.length ? '' : query ? '没有匹配的项目经验。' : '当前筛选下暂无项目经验。'}/>
      </>}>
      {editing && editor ? <ExperienceEditor key={String(editor.entryID ?? 'new')} initial={editor} current={detail.data}
        base={api} request={request} onConflict={refresh} onCancel={() => setEditor(undefined)} onSaved={saved}/> :
        detail.data ? <ExperienceDetails key={detail.data.id} doc={detail.data} project={project} request={request}
          edit={() => start(detail.data)} refresh={() => { setNotice(''); refresh() }}/> :
        <LibraryNotice pane error={detail.error} loading={detail.loading} empty="从左侧选择一条项目经验，查看正文、原文引句与修订历史。" retry={refresh}/>}
    </KnowledgeBrowser>
  </div>
}

function ExperienceEditor({ initial, current, base, request, onConflict, onCancel, onSaved }: {
  initial: EditorState; current?: Entry; base: string; request: ApiRequest; onConflict: () => void; onCancel: () => void; onSaved: (doc: Entry) => void
}) {
  const [title, setTitle] = useState(initial.title), [source, setSource] = useState(initial.source)
  const [content, setContent] = useState(initial.content), [reason, setReason] = useState('')
  const [validity, setValidity] = useState(initial.validity)
  const [expected, setExpected] = useState(initial.revision)
  const [busy, setBusy] = useState(false), [error, setError] = useState('')
  const retry = useRef<{ body: string; key: string } | null>(null)
  const lifetime = useRef<AbortController | null>(null)
  useEffect(() => { const controller = new AbortController(); lifetime.current = controller; return () => controller.abort() }, [])
  const changed = Boolean(initial.entryID && current?.id === initial.entryID && current.current_revision !== expected)
  const unavailable = Boolean(initial.entryID && current?.status === 'withdrawn')

  async function save(e: FormEvent) {
    e.preventDefault()
    if (busy || changed || unavailable) return
    const payload = { expected_revision: expected, title: title.trim(), source: source.trim(), content, reason: reason.trim(), validity: validity.trim() }
    const bytes = new TextEncoder()
    for (const [name, value, limit] of [['标题', payload.title, 256], ['依据来源', payload.source, 2048], ['适用场景', payload.validity, 1024], ['正文', content, 256 * 1024], ['更正原因', payload.reason, 2048]] as const) {
      if (bytes.encode(value).length > limit) { setError(name + '最多允许 ' + limit + ' 字节。'); return }
    }
    const body = JSON.stringify(payload)
    if (retry.current?.body !== body) retry.current = { body, key: crypto.randomUUID() }
    const signal = lifetime.current?.signal
    setBusy(true); setError('')
    try {
      const item = await request<Entry>(initial.entryID ? base + '/' + initial.entryID + '/changes' : base,
        { method: 'POST', body: JSON.stringify({ ...payload, request_key: retry.current.key }), signal })
      if (signal?.aborted) return
      onSaved(item)
    } catch (err) {
      if (signal?.aborted) return
      if (err instanceof ApiError && err.status === 409) { setError('当前状态已变化，已保留草稿。请核对最新修订及待处理请求。'); onConflict() }
      else setError(err instanceof Error ? err.message : '提交失败。')
    } finally { if (!signal?.aborted) setBusy(false) }
  }

  return <form className="knowledge-form" onSubmit={save}>
    <h2>{initial.entryID ? '更正本条经验' : '记录项目经验'}</h2>
    <fieldset disabled={busy}>
      <label>标题<input required value={title} onChange={e => setTitle(e.target.value)}/></label>
      <label>依据来源<input required value={source} onChange={e => setSource(e.target.value)}/></label>
      <label>适用场景（选填）<input value={validity} onChange={e => setValidity(e.target.value)}/></label>
      <label>经验与原始依据（Markdown）<textarea required rows={7} value={content} onChange={e => setContent(e.target.value)}/></label>
      {content && <details className="kb-drawer"><summary>预览拟议正文<Icon name="ChevronDown" size={14}/></summary>
        <div className="kb-drawer-body"><MaterialReader markdown content={content}/></div></details>}
      {initial.entryID && <>
        <KnowledgeComparison summary="比较当前版与拟议版" from="当前版本" to="拟议版本"
          previous={comparisonText(current?.title ?? initial.title, current?.source ?? initial.source, current?.validity ?? initial.validity, current?.statement ?? initial.content)}
          proposed={comparisonText(title, source, validity, content)}/>
        <label>更正原因<input required value={reason} onChange={e => setReason(e.target.value)}/></label>
      </>}
      {error && <p className="detail-problem" role="alert">{error}</p>}
      {changed && current && !unavailable && <button className="button" type="button" onClick={() => { setExpected(current.current_revision); setError('') }}>已核对，基于修订 {current.current_revision} 提交</button>}
      {unavailable && <p className="muted">这条经验已撤回。可复制草稿另行记录。</p>}
      <div className="knowledge-actions">
        <button className="button primary" disabled={unavailable || changed || !title.trim() || !source.trim() || !content.trim()}>提交审核</button>
        <button className="button" type="button" onClick={onCancel}>取消</button>
      </div>
    </fieldset>
  </form>
}

function ExperienceDetails({ doc, project, request, edit, refresh }: { doc: Entry; project: number; request: ApiRequest; edit: () => void; refresh: () => void }) {
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(''), [error, setError] = useState('')
  const [confirm, setConfirm] = useState(false)
  const active = doc.revisions.find(r => r.revision === doc.current_revision)
  const path = '/projects/' + project + '/knowledge/experience/' + doc.id

  async function update(suffix: string, body: object, ok: string) {
    setBusy(true); setError(''); setMessage('')
    try { await request<Entry>(path + suffix, { method: 'POST', body: JSON.stringify(body) }); setMessage(ok); setConfirm(false); refresh() }
    catch (err) { setError(err instanceof ApiError && err.status === 409 ? '条目已变化，请核对最新状态。' : err instanceof Error ? err.message : '提交失败。'); refresh() }
    finally { setBusy(false) }
  }

  return <article>
    {/* Tier 1: Header */}
    <header className="kb-header">
      <div className="kb-header-eyebrow">
        <span className="kb-id-pill">经验 #{doc.id}</span>
        <span className={'kb-badge ' + stateTones[doc.status]}>{states[doc.status]}</span>
        {doc.current_revision > 0 && <span className="kb-id-pill">修订 v{doc.current_revision}</span>}
        {doc.source_retro && <a href={projectHref(project, '/planning/retrospectives?retro=' + doc.source_retro)}>查看来源复盘</a>}
      </div>
      <div className="kb-header-bar">
        <h2>{doc.title}</h2>
        <div className="knowledge-actions">
          {doc.status !== 'withdrawn' && <button className="button" onClick={edit}>更正本条</button>}
          {doc.status === 'current' && <ShareDraft project={project} experience={doc.id} revision={doc.current_revision} request={request}/>}
          {doc.status !== 'withdrawn' && <button className="button" onClick={() => setConfirm(v => !v)}>撤回本条</button>}
        </div>
      </div>
    </header>

    {doc.status === 'stale' && <p className="detail-problem">原始依据已失效。可提交更正并补充当前适用的证据。</p>}
    {confirm && <div className="kb-confirm-box">
      <p>撤回后，后续任务将无法检索这条经验。已有执行仍可核对当时采用的版本。</p>
      <div className="knowledge-actions">
        <button className="button danger" disabled={busy} onClick={() => void update('/withdraw', { expected_revision: doc.current_revision }, '已撤回本条经验。')}>确认撤回</button>
        <button className="button" onClick={() => setConfirm(false)}>取消</button>
      </div>
    </div>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {message && <p className="detail-notice" role="status">{message}</p>}

    {/* Tier 2: Core Experience Statement */}
    {active && <>
      <section className="kb-body-card" aria-label="经验陈述">
        <div className="kb-section-label"><Icon name="BrainCircuit" size={14}/>核心经验</div>
        <MaterialReader markdown content={active.statement}/>
      </section>

      {/* Tier 3: Original Quote & Metadata Grid */}
      <section className="kb-quote-card" aria-label="原文引句">
        <div className="kb-section-label"><Icon name="BookOpen" size={14}/>原文引句</div>
        <blockquote>{active.quote}</blockquote>
      </section>

      <dl className="kb-meta-grid">
        <div className="kb-meta-cell"><dt>依据来源</dt><dd><code>{active.source}</code></dd></div>
        {active.validity && <div className="kb-meta-cell"><dt>适用场景</dt><dd>{active.validity}</dd></div>}
        {doc.reviewed_at && <div className="kb-meta-cell"><dt>审核时间</dt><dd>{formatTime(doc.reviewed_at)}</dd></div>}
      </dl>

      {active.receipt_path && <KnowledgeCuration project={project} path={active.receipt_path} request={request}/>}
    </>}

    {/* Tier 4: Pending Changes & Revision History */}
    {doc.changes.map(change => <details key={change.id} className="kb-drawer" open={change.status === 'ready' || change.status === 'pending'}>
      <summary>
        <span><span className="kb-badge warning">{changes[change.status]}</span> {change.title}</span>
        <Icon name="ChevronDown" size={14}/>
      </summary>
      <div className="kb-drawer-body">
        <dl className="kb-meta-grid">
          <div className="kb-meta-cell"><dt>变更基线</dt><dd>{change.expected_revision ? '修订 v' + change.expected_revision : '新条目'} · {formatTime(change.created_at)}</dd></div>
          <div className="kb-meta-cell"><dt>依据来源</dt><dd><code>{change.source}</code></dd></div>
          {change.reason && <div className="kb-meta-cell"><dt>更正原因</dt><dd>{change.reason}</dd></div>}
          {change.validity && <div className="kb-meta-cell"><dt>适用场景</dt><dd>{change.validity}</dd></div>}
        </dl>
        <MaterialReader markdown content={change.content}/>
        {change.review?.rationale && <p className="detail-notice">审核意见 · {change.review.rationale}</p>}
        {change.receipt_path && <KnowledgeCuration project={project} path={change.receipt_path} request={request}/>}
        {(change.facts ?? []).map((fact, i) => <section className="kb-fact-card" key={i}>
          <h4>审核事实 #{i + 1}</h4>
          <p>{fact.statement}</p>
          <blockquote>{fact.quote}</blockquote>
          {change.status === 'ready' && doc.status !== 'withdrawn' && <div className="knowledge-actions">
            <button className="button primary" disabled={busy} onClick={() => void update('/changes/' + change.id + '/adopt', { fact_number: i + 1 }, '已采用审核事实，当前修订已更新。')}>采纳事实 {i + 1}</button>
          </div>}
        </section>)}
        {(change.status === 'ready' || change.status === 'pending') && <div className="knowledge-actions">
          <button className="button" disabled={busy} onClick={() => void update('/changes/' + change.id + '/discard', {}, '已放弃本次更正。')}>放弃本次更正</button>
        </div>}
      </div>
    </details>)}

    {doc.revisions.length > 0 && <details className="kb-drawer">
      <summary>版本与依据历史（{doc.revisions.length} 个修订）<Icon name="ChevronDown" size={14}/></summary>
      <div className="kb-drawer-body">
        {doc.revisions.map((v, index) => {
          const prior = doc.revisions[index + 1]
          return <section className="kb-revision-item" key={v.revision}>
            <div className="kb-header-eyebrow">
              <span className="kb-id-pill">v{v.revision}</span>
              <span>{v.actor}</span>
              <time dateTime={v.created_at}>{formatTime(v.created_at)}</time>
            </div>
            <h4>{v.title}</h4>
            {v.change_reason && <p className="muted">变更原因 · {v.change_reason}</p>}
            <p>{v.statement}</p>
            <blockquote>{v.quote}</blockquote>
            <p className="muted">来源 <code>{v.source}</code>{v.validity && <> · 适用场景 {v.validity}</>}</p>
            {prior && <KnowledgeComparison summary={'比较修订 v' + prior.revision + ' 与 v' + v.revision} from={'修订 v' + prior.revision} to={'修订 v' + v.revision}
              previous={comparisonText(prior.title, prior.source, prior.validity, prior.statement)} proposed={comparisonText(v.title, v.source, v.validity, v.statement)}/>}
            {v.receipt_path && <KnowledgeCuration project={project} path={v.receipt_path} request={request}/>}
          </section>
        })}
      </div>
    </details>}
  </article>
}
