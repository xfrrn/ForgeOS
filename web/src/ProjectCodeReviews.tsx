import { useEffect, useState } from 'react'
import { formatTime, Icon, projectHref, Status, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'

type Review = {
  task_id: number
  execution_id: number
  title: string
  commit_sha: string
  state: 'pending' | 'failed' | 'needs_check' | 'clear'
  requested_at: string
}
type ReviewsPage = { items: Review[]; next_before?: number }
const reviewStatus = { pending: 'REVIEW_PENDING', failed: 'REVIEW_FAILED', needs_check: 'REVIEW_NEEDS_CHECK', clear: 'REVIEW_CLEAR' } as const

export function ProjectCodeReviews({projectId, request}: {projectId: number; request: ApiRequest}) {
  const [cursors, setCursors] = useState<number[]>([])
  const [revision, setRevision] = useState(0)
  const [page, setPage] = useState<ReviewsPage>()
  const [error, setError] = useState('')
  const before = cursors.at(-1)
  useEffect(() => {
    const controller = new AbortController()
    setPage(undefined); setError('')
    void request<ReviewsPage>(`/projects/${projectId}/code-reviews` + (before ? '?before=' + before : ''), {signal: controller.signal})
      .then(result => { if (!controller.signal.aborted) setPage(result) })
      .catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '无法读取代码审查。') })
    return () => controller.abort()
  }, [before, projectId, request, revision])
  return <>
    <PageHeading title="代码审查" actions={<button className="button refresh" onClick={() => setRevision(n => n + 1)} disabled={!page && !error}>
      <Icon name="RefreshCw" size={15}/>刷新
    </button>}/>
    <section className="panel task-panel" aria-label="代码审查记录">
      {error ? <div role="alert"><EmptyState icon="AlertCircle" title="读取代码审查失败" description={error}>
        <button className="button" onClick={() => setRevision(n => n + 1)}>重试</button>
      </EmptyState></div> : !page ? <p className="table-message" role="status">正在读取代码审查…</p> :
        page.items.length === 0 ? <EmptyState icon="ClipboardCheck" title="本项目暂无已发起的代码审查"/> :
        <div className="table-scroll"><table className="task-table">
          <thead><tr><th scope="col">任务 / 候选提交</th><th scope="col">审查状态</th><th scope="col" className="time-cell">发起时间</th><th scope="col" aria-label="进入任务"/></tr></thead>
          <tbody>{page.items.map(review => {
            const destination = projectHref(projectId, '/tasks/' + review.task_id)
            return <tr key={review.execution_id}>
              <td><a className="task-title" href={destination}><span className="task-glyph"><Icon name="FileCode2" size={16}/></span><span>{review.title}</span></a>
                <div className="task-sub"><span>T-{review.task_id} · 执行 #{review.execution_id}</span>
                  <code title={review.commit_sha}>{review.commit_sha.slice(0, 12)}</code></div>
              </td>
              <td><Status value={reviewStatus[review.state]}/></td>
              <td className="time-cell"><time dateTime={review.requested_at}>{formatTime(review.requested_at)}</time></td>
              <td><a className="row-link" href={destination} aria-label={'查看任务 ' + review.task_id + ' 的代码审查'}><Icon name="ChevronRight" size={16}/></a></td>
            </tr>
          })}</tbody>
        </table></div>}
      {Boolean(cursors.length || page?.next_before) && <div className="table-foot">
        <span>第 {cursors.length + 1} 页</span>
        <div><button className="pager" aria-label="上一页" disabled={!cursors.length || !page} onClick={() => setCursors(prev => prev.slice(0, -1))}>
          <Icon name="ChevronRight" className="flip" size={15}/>
        </button><button className="pager" aria-label="下一页" disabled={!page?.next_before} onClick={() => {
          if (page?.next_before) setCursors(prev => [...prev, page.next_before!])
        }}><Icon name="ChevronRight" size={15}/></button></div>
      </div>}
    </section>
  </>
}
