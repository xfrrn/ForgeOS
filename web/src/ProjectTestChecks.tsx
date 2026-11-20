import { useEffect, useState } from 'react'
import { testPlanLabels } from './CandidateTestPlans'
import { resultLabels } from './CaseEvidence'
import { formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import './project-work.css'

type TestCheck = {
  execution_id: number; task_id: number; title: string; worker: string; commit?: string
  ci_kind?: string; ci_state?: string; plan_revision?: number; plan_state?: string; test_case_state?: string; updated_at?: string
}
type TestPage = { items: TestCheck[]; next_before?: number }

const ciLabels: Record<string, [string, string]> = {
  passed: ['通过', 'done'], failed: ['失败', 'failed'], pending: ['等待验证', 'pending'], running: ['正在验证', 'running'],
  paused: ['已暂停', 'paused'], interrupted: ['执行已结束，未取得验证结果', 'paused'],
  lease_expired: ['执行租约已到期', 'paused'], missing_receipt: ['没有验证回执', 'paused'],
}

export function ProjectTestChecks({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const [cursors, setCursors] = useState<number[]>([])
  const [revision, setRevision] = useState(0)
  const [page, setPage] = useState<TestPage>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const before = cursors.at(-1)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError('')
    void request<TestPage>(`/projects/${projectId}/test-checks` + (before ? '?before=' + before : ''), { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setPage(result) })
      .catch(cause => { if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '无法读取测试与检查。') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [projectId, request, before, revision])
  function changePage(nextCursors: number[]) {
    setPage(undefined)
    setCursors(nextCursors)
  }
  return <>
    <PageHeading title="测试与检查" description="查看本地验证、测试计划、覆盖审核和用例结果。"
      actions={<button type="button" className="button" disabled={loading} onClick={() => setRevision(n => n + 1)}>
        <Icon name="RefreshCw" size={15}/>{loading ? '读取中…' : '刷新'}
      </button>}/>
    <section className="panel task-panel" aria-label="测试与检查记录">
      {error && <p className="table-message" role="alert">{error}</p>}
      {!page && !error && <p className="table-message" role="status">正在读取测试与检查…</p>}
      {page && (page.items.length ? <div className="table-scroll"><table className="task-table project-work-table">
        <thead><tr><th scope="col">任务和执行</th><th scope="col">测试与检查进度</th></tr></thead>
        <tbody>{page.items.map(item => {
          const ci = item.ci_state && ciLabels[item.ci_state]
          return <tr key={item.execution_id}>
            <td><a className="task-title" href={projectHref(projectId, `/tasks/${item.task_id}?execution=${item.execution_id}`)}>
              <span className="task-glyph"><Icon name="ClipboardCheck" size={16}/></span><span>{item.title}</span></a>
              <div className="task-sub"><span>T-{item.task_id} · 执行 #{item.execution_id}</span>
                {item.commit && <code title={item.commit}>{item.commit.slice(0, 12)}</code>}
                {item.updated_at && <time dateTime={item.updated_at}>{formatTime(item.updated_at)}</time>}
              </div>
            </td>
            <td>
              {ci && <div><strong>{item.ci_kind === 'startup' ? '本地启动检查' : '本地 CI Runner'}</strong>{' '}
                <span className={'status status-' + ci[1]}><i/>{ci[0]}</span>
              </div>}
              {item.plan_state && <div><strong>测试计划 · 修订 {item.plan_revision}</strong>{' · '}{testPlanLabels[item.plan_state] ?? item.plan_state}</div>}
              {item.plan_state === 'covered' && item.test_case_state && <div>用例结果 · {resultLabels[item.test_case_state] ?? item.test_case_state}</div>}
              <small className="muted">节点 {item.worker}</small>
            </td>
          </tr>
        })}</tbody>
      </table></div> : <EmptyState icon="ClipboardCheck" title="此页没有测试与检查记录"/>)}
      {Boolean(cursors.length || page?.next_before) && <div className="table-foot">
        <span>第 {cursors.length + 1} 页</span>
        <div><button className="pager" aria-label="上一页" disabled={!cursors.length || loading} onClick={() => changePage(cursors.slice(0, -1))}>
          <Icon name="ChevronRight" className="flip" size={15}/>
        </button><button className="pager" aria-label="下一页" disabled={!page?.next_before || loading} onClick={() => {
          if (page?.next_before) changePage([...cursors, page.next_before])
        }}><Icon name="ChevronRight" size={15}/></button></div>
      </div>}
    </section>
  </>
}
