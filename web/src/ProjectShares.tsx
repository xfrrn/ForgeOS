import { useEffect, useState } from 'react'
import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import { ApiError, formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'
import { PageHeading } from './ui'
import { MaterialReader } from './MaterialReader'
import { KnowledgeBrowser, KnowledgeSearch, LibraryNotice, useKnowledgeDetail, useKnowledgeList } from './KnowledgeBrowser'
import { ShareEditor, type SharedCopy, type ShareRevision } from './ShareDraft'

type Receipt = ShareRevision & {
  review_processes?: ReviewProcess[]
  review?: { verdict: 'approve' | 'revise' | 'needs_owner'; rationale: string; evidence: string; effective_level: string; session_id: string; profile?: StructuredProfile }
}
const verdicts = { approve: '已通过审核，待发布', revise: '需要修改副本', needs_owner: '需要 Owner 决定' }

function status(row: Receipt) {
  if (row.revoked_at) return '已撤销'
  if (row.superseded_at) return '已替代'
  if (row.published_at) return '已发布'
  if (row.stale) return '发布状态已变化'
  return row.review ? verdicts[row.review.verdict] : '待审核'
}

function shareTone(row: Receipt) {
  if (row.revoked_at || row.superseded_at) return 'neutral'
  if (row.published_at) return 'positive'
  if (row.review?.verdict === 'revise') return 'danger'
  return 'warning'
}

export function ProjectShares({ project, receipt, received, request }: { project: number; receipt?: number; received: boolean; request: ApiRequest }) {
  const base = '/projects/' + project
  const section = projectHref(project, '/knowledge/shares/')
  const href = section + (received ? 'received' : 'sent')
  const [creating, setCreating] = useState(false)
  const [query, setQuery] = useState(''), [text, setText] = useState('')
  const [revision, setRevision] = useState(0)
  const refresh = () => setRevision(r => r + 1)

  useEffect(() => {
    if (!received) return
    const update = () => { if (document.visibilityState === 'visible') setRevision(r => r + 1) }
    window.addEventListener('focus', update)
    document.addEventListener('visibilitychange', update)
    return () => { window.removeEventListener('focus', update); document.removeEventListener('visibilitychange', update) }
  }, [received])

  const path = received ? base + '/shared-experience' + (query ? '/search?versions=1&q=' + encodeURIComponent(query) : '?versions=1') :
    base + '/shares?latest=1' + (query ? '&q=' + encodeURIComponent(query) : '')
  const list = useKnowledgeList<Receipt | SharedCopy>(request, path, revision, !received || !query)
  const first = list.rows[0]
  const selected = receipt ?? (received ? first?.series_id : first?.id)
  const detailPath = selected ? base + (received ? '/shared-experience/' : '/shares/') + selected : undefined
  const detail = useKnowledgeDetail<Receipt | SharedCopy>(request, detailPath, revision)
  function back() { setCreating(false); location.hash = href }
  function saved(row: SharedCopy) { setCreating(false); location.hash = section + 'sent/' + row.id; refresh() }

  return <div className="knowledge-page">
    <PageHeading title="共享经验" description="在项目间受控发布脱敏后的经验副本，保持知识边界清晰可追溯。"
      actions={!received && <button className="button primary" onClick={() => setCreating(true)}><Icon name="Plus" size={15}/>起草脱敏副本</button>}/>
    <nav className="knowledge-direction" aria-label="共享方向">
      <a className="button" aria-current={!received ? 'page' : undefined} href={section + 'sent'}>我发出的</a>
      <a className="button" aria-current={received ? 'page' : undefined} href={section + 'received'}>我收到的</a>
    </nav>
    <KnowledgeBrowser detailActive={Boolean(receipt || creating)} back={back}
      toolbar={<KnowledgeSearch query={text} setQuery={setText} search={q => { setQuery(q); back() }} refresh={refresh}
        placeholder={received ? '搜索收到的共享经验' : '搜索发出的副本'}/>}
      catalog={<>
        {list.rows.map(row => {
          const id = received ? row.series_id : row.id
          const isSent = 'project_id' in row
          return <a href={href + '/' + id} className="knowledge-row" key={row.id} aria-current={selected === id && !creating ? 'true' : undefined}
            onClick={() => setCreating(false)}>
            <div className="knowledge-row-top">
              <span className="knowledge-row-id">v{row.share_version}</span>
              <span className={'kb-badge ' + (isSent ? shareTone(row) : 'positive')}>{isSent ? status(row) : '已发布'}</span>
            </div>
            <strong className="knowledge-row-title">{row.title}</strong>
            <small className="knowledge-row-meta">
              {isSent && <>发往项目 #{row.recipient} · </>}
              {formatTime((isSent ? row.superseded_at || row.revoked_at || row.published_at || row.proposed_at : row.published_at) ?? '')}
            </small>
          </a>
        })}
        <LibraryNotice {...list} retry={refresh} empty={list.rows.length ? '' : query ? '没有匹配的共享经验。' : received ? '尚无发布给本项目的共享经验。' : '尚无发出的副本。'}/>
      </>}>
      {creating ? <ShareEditor project={project} request={request} onCancel={() => setCreating(false)} onSaved={saved}/> :
        detail.data ? 'project_id' in detail.data ? <SentDetails key={detail.data.id} row={detail.data} project={project} request={request} revision={revision} refresh={refresh} saved={saved}/> :
          <ReceivedDetails key={detail.data.id} project={project} row={detail.data}/> :
          <LibraryNotice pane loading={detail.loading} error={detail.error} empty="从左侧选择一条共享经验，查看脱敏内容与发布记录。" retry={refresh}/>}
    </KnowledgeBrowser>
  </div>
}

function ReceivedDetails({ project, row }: { project: number; row: SharedCopy }) {
  const [copy, setCopy] = useState('')
  const href = projectHref(project, '/knowledge/shares/received/' + row.series_id)
  const link = new URL(href, window.location.href).href
  const ref = row.id + ':' + row.digest
  async function copyValue(value: string, label: string) {
    try { await navigator.clipboard.writeText(value); setCopy(label + '已复制。') } catch { setCopy(value) }
  }
  return <article>
    <header className="kb-header">
      <div className="kb-header-eyebrow">
        <span className="kb-id-pill">副本 #{row.id}</span>
        <span className="kb-badge positive">已发布 · v{row.share_version}</span>
      </div>
      <div className="kb-header-bar">
        <h2>{row.title}</h2>
        <div className="knowledge-actions">
          <button className="button" onClick={() => void copyValue(ref, '版本引用')}>复制本版引用</button>
          <button className="button" onClick={() => void copyValue(link, '条目链接')}>复制条目链接</button>
        </div>
      </div>
    </header>
    {copy && <p className="detail-notice" role="status">{copy}</p>}

    <section className="kb-body-card" aria-label="共享正文">
      <div className="kb-section-label"><Icon name="BookOpen" size={14}/>共享经验正文</div>
      <MaterialReader markdown content={row.content}/>
    </section>

    <dl className="kb-meta-grid">
      {row.published_at && <div className="kb-meta-cell"><dt>发布时间</dt><dd>{formatTime(row.published_at)}</dd></div>}
      <div className="kb-meta-cell"><dt>来源类型</dt><dd><code>shared_experience</code></dd></div>
    </dl>

    <details className="kb-drawer">
      <summary>CLI 引用与副本摘要<Icon name="ChevronDown" size={14}/></summary>
      <div className="kb-drawer-body">
        <dl className="kb-meta-grid">
          <div className="kb-meta-cell"><dt>版本引用</dt><dd><code>{ref}</code></dd></div>
          <div className="kb-meta-cell"><dt>SHA-256</dt><dd><code>{row.digest}</code></dd></div>
        </dl>
      </div>
    </details>
  </article>
}

function SentDetails({ project, row, request, revision, refresh, saved }: {
  project: number; row: Receipt; request: ApiRequest; revision: number; refresh: () => void; saved: (row: SharedCopy) => void
}) {
  const [editing, setEditing] = useState(false), [revoking, setRevoking] = useState(false)
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(''), [error, setError] = useState('')
  const [showHistory, setShowHistory] = useState(false)
  const base = '/projects/' + project + '/shares/'
  const current = useKnowledgeDetail<Receipt>(request, editing && row.current_share_id && row.current_share_id !== row.id ? base + row.current_share_id : undefined, revision)

  async function publish(action: 'publish' | 'revoke') {
    setBusy(true); setError(''); setMessage('')
    try {
      await request<Receipt>(base + row.id + '/publication', { method: 'POST', body: JSON.stringify({ action, digest: row.digest }) })
      setMessage(action === 'publish' ? '已向接收项目发布该版本。' : '已撤销副本。'); setRevoking(false); refresh()
    } catch (err) {
      setError(err instanceof ApiError && err.status === 409 ? '发布状态或审核结果已变化，请核对当前版本。' : err instanceof Error ? err.message : '操作失败。')
      refresh()
    } finally { setBusy(false) }
  }

  return <article>
    <header className="kb-header">
      <div className="kb-header-eyebrow">
        <span className="kb-id-pill">发往项目 #{row.recipient}</span>
        <span className="kb-id-pill">v{row.share_version}</span>
        <span className={'kb-badge ' + shareTone(row)}>{status(row)}</span>
      </div>
      <div className="kb-header-bar">
        <h2>{row.title}</h2>
        <div className="knowledge-actions">
          {!editing && <button className="button" onClick={() => setEditing(true)}>起草新版</button>}
          {row.review?.verdict === 'approve' && !row.published_at && !row.revoked_at && !row.stale &&
            <button className="button primary" disabled={busy} onClick={() => void publish('publish')}>发布给项目 #{row.recipient}</button>}
          {!row.revoked_at && !row.superseded_at && <button className="button" disabled={busy} onClick={() => setRevoking(!revoking)}>撤销副本</button>}
        </div>
      </div>
    </header>

    {revoking && <div className="kb-confirm-box">
      <p>{row.published_at ? '撤销后，接收项目无法再读取或采用这个版本。' : '撤销后，这份草稿将无法发布。'}</p>
      <div className="knowledge-actions">
        <button className="button danger" disabled={busy} onClick={() => void publish('revoke')}>确认撤销</button>
        <button className="button" onClick={() => setRevoking(false)}>取消</button>
      </div>
    </div>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {message && <p className="detail-notice" role="status">{message}</p>}

    <section className="kb-body-card" aria-label="脱敏副本正文">
      <div className="kb-section-label"><Icon name="BookOpen" size={14}/>脱敏副本内容</div>
      <MaterialReader markdown content={row.content}/>
    </section>

    <dl className="kb-meta-grid">
      <div className="kb-meta-cell"><dt>来源条目</dt><dd>
        {row.experience_id
          ? <a href={projectHref(project, '/knowledge/experience/' + row.experience_id)}>项目经验 #{row.experience_id} · 修订 {row.experience_revision}</a>
          : <a href={projectHref(project, '/materials?key=' + encodeURIComponent(row.material_key ?? '') + '&revision=' + row.revision)}>{row.material_key} · 修订 {row.revision}</a>}
      </dd></div>
      <div className="kb-meta-cell"><dt>提交时间</dt><dd>{formatTime(row.proposed_at)}</dd></div>
      {row.published_at && <div className="kb-meta-cell"><dt>发布时间</dt><dd>{formatTime(row.published_at)}</dd></div>}
      {row.current_share_id && row.current_share_id !== row.id && <div className="kb-meta-cell"><dt>当前发布版</dt>
        <dd><a href={projectHref(project, '/knowledge/shares/sent/' + row.current_share_id)}>副本 #{row.current_share_id}</a></dd>
      </div>}
    </dl>

    {row.review && <details className="kb-drawer">
      <summary>审核依据 · {row.review.effective_level}<Icon name="ChevronDown" size={14}/></summary>
      <div className="kb-drawer-body">
        <p>{row.review.rationale}</p>
        <p className="muted">证据 · {row.review.evidence}</p>
        <p className="muted">{row.review.session_id.startsWith('api-') ? '模型 API 调用' : '原生会话'} · <code>{row.review.session_id}</code></p>
        {row.review.profile && <RolePermissions role="Management" profile={row.review.profile}/>}
      </div>
    </details>}

    <ManagementProcesses request={request} processes={row.review_processes}/>

    {editing && current.error && <p className="detail-problem" role="alert">当前发布版读取失败。 {current.error} <button className="button" onClick={refresh}>重试</button></p>}
    {editing && <ShareEditor project={project} row={row} current={row.current_share_id === row.id ? row : current.data} request={request}
      onCancel={() => setEditing(false)} onConflict={refresh} onSaved={item => { setEditing(false); saved(item) }}/>}

    <details className="kb-drawer" onToggle={e => setShowHistory(e.currentTarget.open)}>
      <summary>版本与发布记录<Icon name="ChevronDown" size={14}/></summary>
      <div className="kb-drawer-body">
        <p className="muted">本版摘要 · <code>{row.digest}</code></p>
        {showHistory && <ShareHistory project={project} row={row} request={request} revision={revision}/>}
      </div>
    </details>
  </article>
}

function ShareHistory({ project, row, request, revision }: { project: number; row: Receipt; request: ApiRequest; revision: number }) {
  const [tick, setTick] = useState(0)
  const list = useKnowledgeList<Receipt>(request, '/projects/' + project + '/shares/' + row.id + '/versions', revision + tick)
  return <div>{list.rows.map(item => <a className="knowledge-row" href={projectHref(project, '/knowledge/shares/sent/' + item.id)} key={item.id}>
    <strong>v{item.share_version} · {item.title}</strong><small className="knowledge-row-meta">{status(item)} · {formatTime(item.proposed_at)}</small>
  </a>)}<LibraryNotice {...list} empty="" retry={() => setTick(n => n + 1)}/></div>
}
