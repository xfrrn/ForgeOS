import { useEffect, useRef, useState } from 'react'
import { ApiError, safeGitHubURL, type ApiRequest } from './shared'
import { workTypeName, workTypes } from './workTypes'
import { ClassificationAdvice, type IssueSuggestion } from './ClassificationAdvice'

type Discussion = { id: number; number: number; title: string; url: string; work_type?: string; source_digest?: string; suggestion?: IssueSuggestion; classification_id?: number }

export function ProjectInbox({ project, request }: { project: number; request: ApiRequest }) {
  const [rows, setRows] = useState<Discussion[]>([])
  const [before, setBefore] = useState(0)
  const [revision, setRevision] = useState(0)
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [converting, setConverting] = useState(0)
  const [issueNumber, setIssueNumber] = useState('')
  const [choices, setChoices] = useState<Record<number, string>>({})
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

  async function convert(number: number, workType?: string, sourceDigest?: string) {
    if (conversion.current) return
    const controller = new AbortController()
    conversion.current = controller
    setConverting(number); setError('')
    try {
      const input = { validation: 'auto', work_type: workType, source_digest: sourceDigest }
      const task = await request<{ id: number }>(`/projects/${project}/issues/${number}`, {
        method: 'POST', body: JSON.stringify(input), signal: controller.signal,
      })
      if (!controller.signal.aborted) location.hash = '/tasks/' + task.id
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof ApiError && e.status === 409 && sourceDigest ? '来源 Issue 或任务类型已变化。请刷新讨论并核对。' : e instanceof Error ? e.message : '无法转换讨论。')
    } finally {
      conversion.current = null
      if (!controller.signal.aborted) setConverting(0)
    }
  }

  return <section className="panel detail-card">
    <a href="#/projects">返回项目接入</a>
    <h1>项目 #{project} · 讨论收件箱</h1>
    <p>开发任务进入实现队列。审查、测试、调查和运维先进入只读规划，供 Owner 核对范围。</p>
    <form onSubmit={e => {
      e.preventDefault()
      const workType = new FormData(e.currentTarget).get('work_type')
      void convert(Number(issueNumber), typeof workType === 'string' && workType ? workType : undefined)
    }}>
      <fieldset disabled={!!converting}><legend>按编号派发 Issue</legend>
        <label>本项目 Issue 编号<input type="text" inputMode="numeric" pattern="[1-9][0-9]*" required value={issueNumber} onChange={e => {
          setIssueNumber(e.target.value)
          e.target.setCustomValidity(Number.isSafeInteger(Number(e.target.value)) ? '' : '请输入有效的 Issue 编号。')
        }}/></label>
        <label htmlFor="dispatch-work-type">工作类型</label>
        <select id="dispatch-work-type" key={issueNumber} name="work_type" defaultValue="">
          <option value="">沿用 Issue 类型</option>
          {Object.entries(workTypes).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
        </select>
        <button className="button" type="submit">派发 Issue</button>
      </fieldset>
    </form>
    <div><button className="button" disabled={loading || !!converting} onClick={() => { setChoices({}); setBefore(0); setRevision(v => v + 1) }}>刷新讨论</button></div>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {loading && <p role="status">正在读取讨论…</p>}
    {!loading && !error && !rows.length && <p>暂无待转换的讨论。</p>}
    {rows.map(row => {
      const classified = workTypeName(row.work_type)
      const choice = classified ? row.work_type! : choices[row.id] ?? (row.work_type === 'clarification' ? '' : 'development')
      return <article className="registered-project" key={row.id}>
      <div><h2>#{row.number} · {row.title}</h2>
        {safeGitHubURL(row.url) && <a href={safeGitHubURL(row.url)} target="_blank" rel="noreferrer">查看原 Issue #{row.number}</a>}</div>
      <div>{(row.suggestion || row.classification_id) && <ClassificationAdvice key={row.classification_id} suggestion={row.suggestion} classification={row.classification_id} project={project} request={request} />}
        {classified ? <p>任务类型 · {classified}</p> : row.source_digest && <label>Issue #{row.number} 任务类型
        <select value={choice} disabled={loading || !!converting} onChange={e => setChoices(old => ({ ...old, [row.id]: e.target.value }))}>
          <option value="">请选择并澄清类型</option>
          {Object.entries(workTypes).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
        </select></label>}
        <button className="button" disabled={loading || !!converting || !choice} onClick={() => void convert(row.number, row.source_digest ? choice : undefined, row.source_digest || undefined)}>
          {converting === row.number ? '正在转换…' : choice === 'development' ? '转为实现任务' : '转为规划任务'}</button>
      </div>
    </article>})}
    {more && rows.length > 0 && <div><button className="button" disabled={loading || !!converting} onClick={() => { setBefore(rows[rows.length - 1].number); setRevision(v => v + 1) }}>更多讨论</button></div>}
  </section>
}
