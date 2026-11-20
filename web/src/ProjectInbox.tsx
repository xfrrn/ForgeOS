import { useEffect, useRef, useState } from 'react'
import { ApiError, projectHref, safeGitHubURL, type ApiRequest } from './shared'
import { workTypeName, workTypes } from './workTypes'
import { ClassificationAdvice, type IssueSuggestion } from './ClassificationAdvice'
import { BackLink, EmptyState, PageHeading } from './ui'
import { ThemedSelect } from './ThemedSelect'

type Discussion = { id: number; number: number; title: string; url: string; work_type?: string; source_digest?: string; suggestion?: IssueSuggestion; classification_id?: number }
type IssueDetails = Discussion & { body: string; updated_at: string }
const typeOptions = Object.entries(workTypes).map(([value, label]) => ({value, label}))
const issueOptions = [{value: '', label: '沿用 Issue 类型'}, ...typeOptions]
const discussionOptions = [{value: '', label: '请选择并澄清类型'}, ...typeOptions]

function IssuePreview({ project, number, request, onRead }: {
  project: number; number: number; request: ApiRequest; onRead: (issue: IssueDetails) => void
}) {
  const [issue, setIssue] = useState<IssueDetails>()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => active.current?.abort(), [])

  async function read() {
    if (active.current) return
    const controller = new AbortController()
    active.current = controller
    setLoading(true); setError('')
    try {
      const result = await request<IssueDetails>(`/projects/${project}/issues/${number}`, {signal: controller.signal})
      if (!controller.signal.aborted) { setIssue(result); onRead(result) }
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '无法读取 Issue。')
    } finally {
      active.current = null
      if (!controller.signal.aborted) setLoading(false)
    }
  }

  return <details onToggle={event => { if (event.currentTarget.open) void read() }}>
    <summary>查看 Issue #{number} 内容</summary>
    {loading && <p role="status">正在读取 Issue…</p>}
    {error && <p className="detail-problem" role="alert">{error} <button type="button" className="button" onClick={() => void read()}>重试</button></p>}
    {issue && <div>
      <p><strong>{issue.title}</strong></p>
      {issue.body ? <pre className="material-content">{issue.body}</pre> : <p className="muted">这条 Issue 没有填写正文。</p>}
      {safeGitHubURL(issue.url) && <a href={safeGitHubURL(issue.url)} target="_blank" rel="noreferrer">去 GitHub 继续讨论</a>}
    </div>}
  </details>
}

export function ProjectInbox({ project, request }: { project: number; request: ApiRequest }) {
  const [rows, setRows] = useState<Discussion[]>([])
  const [before, setBefore] = useState(0)
  const [revision, setRevision] = useState(0)
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [converting, setConverting] = useState(0)
  const [issueNumber, setIssueNumber] = useState('')
  const [issueType, setIssueType] = useState('')
  const [choices, setChoices] = useState<Record<number, string>>({})
  const [details, setDetails] = useState<Record<number, IssueDetails>>({})
  const conversion = useRef<AbortController | null>(null)
  useEffect(() => () => conversion.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError('')
    request<Discussion[]>(`/projects/${project}/inbox` + (before ? '?before=' + before : ''), { signal: controller.signal })
      .then(items => {
        if (controller.signal.aborted) return
        setRows(old => before ? [...old, ...items.filter(i => !old.some(r => r.id === i.id))] : items)
        setMore(items.length === 100)
      })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [before, project, request, revision])

  async function convert(number: number, selection: { work_type?: string; source_digest?: string; require_work_type?: boolean }) {
    if (conversion.current) return
    const controller = new AbortController()
    conversion.current = controller
    setConverting(number); setError('')
    try {
      const task = await request<{ id: number }>(`/projects/${project}/issues/${number}`, {
        method: 'POST', body: JSON.stringify({ validation: 'auto', ...selection }), signal: controller.signal,
      })
      if (!controller.signal.aborted) location.hash = projectHref(project, '/tasks/' + task.id)
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof ApiError && e.status === 409 && selection.source_digest ? '来源 Issue 或任务类型已变化。请刷新讨论并核对。' : e instanceof Error ? e.message : '无法转换讨论。')
    } finally {
      conversion.current = null
      if (!controller.signal.aborted) setConverting(0)
    }
  }

  return <div className="project-setup">
    <BackLink href={projectHref(project, '/tasks')}>返回项目任务</BackLink>
    <PageHeading title="讨论收件箱" description="查看同步的 GitHub Issue，选择继续讨论或派发任务。"
      actions={<button className="button" disabled={loading || !!converting} onClick={() => { setChoices({}); setDetails({}); setBefore(0); setRevision(v => v + 1) }}>刷新讨论</button>}/>
    <details className="panel detail-card">
    <summary>按编号接入 Issue</summary>
    <form className="material-form" onSubmit={e => {
      e.preventDefault()
      const workType = new FormData(e.currentTarget).get('work_type')
      void convert(Number(issueNumber), { require_work_type: true, source_digest: details[Number(issueNumber)]?.source_digest,
        work_type: typeof workType === 'string' && workType ? workType : undefined })
    }}>
      <fieldset disabled={!!converting}><legend>按编号派发 Issue</legend>
        <label>本项目 Issue 编号<input type="text" inputMode="numeric" pattern="[1-9][0-9]*" required value={issueNumber} onChange={e => {
          setIssueNumber(e.target.value)
          setIssueType('')
          e.target.setCustomValidity(Number.isSafeInteger(Number(e.target.value)) ? '' : '请输入有效的 Issue 编号。')
        }}/></label>
        {/^[1-9]\d*$/.test(issueNumber) && Number.isSafeInteger(Number(issueNumber)) &&
          <IssuePreview key={issueNumber} project={project} number={Number(issueNumber)} request={request}
            onRead={issue => setDetails(old => ({ ...old, [issue.number]: issue }))}/>}
        <ThemedSelect label="工作类型" name="work_type" value={issueType} options={issueOptions} disabled={!!converting} onChange={setIssueType}/>
        <button className="button primary" type="submit">派发 Issue</button>
      </fieldset>
    </form></details>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    <section className="panel detail-card"><h2>待处理 Issue</h2>
    {loading && <p role="status" className="table-message">正在读取讨论…</p>}
    {!loading && !error && !rows.length && <EmptyState icon="Inbox" title="暂无待转换的讨论"/>}
    {rows.map(entry => {
      const row = details[entry.number] ? { ...entry, ...details[entry.number] } : entry
      const classified = workTypeName(row.work_type)
      const choice = classified ? row.work_type! : choices[row.id] ?? ''
      const currentSuggestion = !details[entry.number] || details[entry.number].source_digest === entry.source_digest
      return <article className="registered-project" key={row.id}>
      <div><h2>#{row.number} · {row.title}</h2>
        <IssuePreview project={project} number={row.number} request={request}
          onRead={issue => setDetails(old => ({ ...old, [issue.number]: issue }))}/></div>
      <div>{currentSuggestion && (row.suggestion || row.classification_id) && <ClassificationAdvice key={row.classification_id} suggestion={row.suggestion} classification={row.classification_id} project={project} request={request} />}
        {classified ? <p>任务类型 · {classified}</p> : row.source_digest && <ThemedSelect label={`Issue #${row.number} 任务类型`}
          value={choice} options={discussionOptions} disabled={loading || !!converting}
          onChange={value => setChoices(old => ({ ...old, [row.id]: value }))}/>}
        <button className="button" disabled={loading || !!converting || !choice} onClick={() => void convert(row.number, { require_work_type: true, work_type: choice, source_digest: row.source_digest })}>
          {converting === row.number ? '正在派发…' : choice === 'development' ? '进入执行队列' : '进入规划队列'}</button>
      </div>
    </article>})}
    {more && rows.length > 0 && <div><button className="button" disabled={loading || !!converting} onClick={() => { setBefore(rows[rows.length - 1].number); setRevision(v => v + 1) }}>更多讨论</button></div>}
    </section>
  </div>
}
