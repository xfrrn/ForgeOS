import { useEffect, useRef, useState } from 'react'
import { ApiError, safeGitHubURL, type ApiRequest } from './shared'
import { workTypeName, workTypes } from './workTypes'
import { ClassificationAdvice, type IssueSuggestion } from './ClassificationAdvice'

type Discussion = { id: number; number: number; title: string; url: string; work_type?: string; source_digest?: string; suggestion?: IssueSuggestion }

export function ProjectInbox({ project, request }: { project: number; request: ApiRequest }) {
  const [rows, setRows] = useState<Discussion[]>([])
  const [before, setBefore] = useState(0)
  const [revision, setRevision] = useState(0)
  const [more, setMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [converting, setConverting] = useState(0)
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

  async function convert(row: Discussion, workType: string) {
    if (conversion.current) return
    const controller = new AbortController()
    conversion.current = controller
    setConverting(row.number); setError('')
    try {
      const input = row.source_digest ? { validation: 'auto', work_type: workType, source_digest: row.source_digest } : { validation: 'auto' }
      const task = await request<{ id: number }>(`/projects/${project}/issues/${row.number}`, {
        method: 'POST', body: JSON.stringify(input), signal: controller.signal,
      })
      if (!controller.signal.aborted) location.hash = '/tasks/' + task.id
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof ApiError && e.status === 409 ? '来源 Issue 或任务类型已变化。请刷新讨论并核对。' : e instanceof Error ? e.message : '无法转换讨论。')
    } finally {
      conversion.current = null
      if (!controller.signal.aborted) setConverting(0)
    }
  }

  return <section className="panel detail-card">
    <a href="#/projects">返回项目接入</a>
    <h1>项目 #{project} · 讨论收件箱</h1>
    <p>开发任务进入实现队列。审查、测试、调查和运维先进入只读规划，供 Owner 核对范围。</p>
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
      <div>{row.suggestion && <ClassificationAdvice suggestion={row.suggestion} />}
        {classified ? <p>任务类型 · {classified}</p> : row.source_digest && <label>Issue #{row.number} 任务类型
        <select value={choice} disabled={loading || !!converting} onChange={e => setChoices(old => ({ ...old, [row.id]: e.target.value }))}>
          <option value="">请选择并澄清类型</option>
          {Object.entries(workTypes).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
        </select></label>}
        <button className="button" disabled={loading || !!converting || !choice} onClick={() => void convert(row, choice)}>
          {converting === row.number ? '正在转换…' : choice === 'development' ? '转为实现任务' : '转为规划任务'}</button>
      </div>
    </article>})}
    {more && rows.length > 0 && <div><button className="button" disabled={loading || !!converting} onClick={() => { setBefore(rows[rows.length - 1].number); setRevision(v => v + 1) }}>更多讨论</button></div>}
  </section>
}
