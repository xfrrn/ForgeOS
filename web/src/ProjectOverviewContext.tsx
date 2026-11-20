import { useEffect, useState } from 'react'
import { ciLabels } from './ProjectTestChecks'
import { Fact, formatTime, Icon, projectHref, safeGitHubURL, type ApiRequest } from './shared'

const githubLabels = { disabled: '未配置', pending: '等待首次同步', ok: '最近轮询通过', failed: '最近轮询失败', rate_limited: '触发 GitHub 限流' }
const activityLabels = {
  execution_failed: '执行失败', execution_retry_requested: '已请求重试', acceptance_approved: '验收通过',
  acceptance_rejected: '退回修改', pr_merged: 'PR 已合并', delivery_completed: '交付完成',
}
type Activity = { id: number; task_id: number; execution_id?: number; title: string; event: keyof typeof activityLabels; at: string; pull_request?: string }
type Context = {
  repository?: { branch: string; sha: string; subject?: string; date?: string; dirty?: boolean }
  check?: { task_id: number; execution_id: number; title: string; ci_state?: string }
  check_failed?: boolean
  activity: Activity[] | null
  activity_failed?: boolean
  github: keyof typeof githubLabels
  poll?: { checked_at: string; last_success?: string }
}

export function ProjectOverviewContext({ projectId, request, revision }: { projectId: number; request: ApiRequest; revision: number }) {
  const [data, setData] = useState<Context>()
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const result = await request<Context>(`/projects/${projectId}/overview-context`, { signal: stop.signal })
        if (!stop.signal.aborted) { setData(result); setFailed(false) }
      } catch {
        if (!stop.signal.aborted) setFailed(true)
      }
      if (!stop.signal.aborted) timer = setTimeout(refresh, 30000)
    }
    void refresh()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [projectId, request, revision])

  const repo = data?.repository
  const check = data?.check
  return <>
    {failed && <p className="detail-problem" role="alert">{data ? '仓库与活动刷新失败，显示上次结果。' : '读取仓库与项目活动失败。'}</p>}
    <div className="overview-grid project-overview-work">
      <section className="panel overview-card project-overview-context" aria-labelledby="repository-context-heading">
        <div className="overview-heading">
          <h2 id="repository-context-heading">仓库与检查</h2>
          <a className="overview-link" href={projectHref(projectId, '/materials')}>项目资料<Icon name="ChevronRight" size={14}/></a>
        </div>
        {data ? <dl className="detail-facts">
          <Fact label="本地分支与 HEAD">
            {repo ? <>{repo.branch || '分离 HEAD'} · <code title={repo.sha}>{repo.sha.slice(0, 12)}</code></> : '本地仓库暂不可读'}
          </Fact>
          {repo?.subject && <Fact label="最近提交">{repo.subject}{repo.date &&
            <> · <time dateTime={repo.date}>{formatTime(repo.date)}</time></>}</Fact>}
          {repo && <Fact label="工作区">{repo.dirty === undefined ? '状态未知' : repo.dirty ?
            <span className="runtime-warning">有未提交改动</span> : '干净'}</Fact>}
          <Fact label="GitHub Issue 同步">
            <span className={data.github === 'failed' || data.github === 'rate_limited' ? 'runtime-warning' : ''}>{githubLabels[data.github]}</span>
            {data.poll && <> · <time dateTime={data.poll.checked_at}>{formatTime(data.poll.checked_at)}</time></>}
            {data.poll?.last_success && data.github !== 'ok' &&
              <> · 上次成功 <time dateTime={data.poll.last_success}>{formatTime(data.poll.last_success)}</time></>}
          </Fact>
          <Fact label="最近检查记录">
            {data.check_failed ? '读取失败' : check ? <>
              <a href={projectHref(projectId, `/tasks/${check.task_id}?execution=${check.execution_id}`)}>{check.title}</a>
              {check.ci_state && ciLabels[check.ci_state] && <> · {ciLabels[check.ci_state][0]}</>}
            </> : '暂无检查记录'}
          </Fact>
        </dl> : !failed && <p className="muted" role="status">正在读取仓库与检查…</p>}
      </section>
      <section className="panel overview-card" aria-labelledby="project-activity-heading">
        <div className="overview-heading"><h2 id="project-activity-heading">近期活动</h2></div>
        {data?.activity_failed ? <p className="detail-problem" role="alert">读取项目活动失败。</p>
          : data?.activity?.length ? <ul className="project-overview-rows">{data.activity.map(item => {
            const pr = safeGitHubURL(item.pull_request)
            return <li key={item.id}>
              <div className="project-overview-row">
                <a className="task-title" href={projectHref(projectId, `/tasks/${item.task_id}${item.execution_id ? '?execution=' + item.execution_id : ''}`)}>{item.title}</a>
                <strong>{activityLabels[item.event]}</strong>
              </div>
              <p className="project-overview-row-meta">
                T-{item.task_id} · <time dateTime={item.at}>{formatTime(item.at)}</time>
                {pr && <> · <a href={pr} target="_blank" rel="noreferrer">查看 PR <Icon name="ExternalLink" size={12}/></a></>}
              </p>
            </li>
          })}</ul> : data ? <p className="muted">暂无失败、重试、验收或交付记录</p>
            : !failed && <p className="muted" role="status">正在读取项目活动…</p>}
      </section>
    </div>
  </>
}
