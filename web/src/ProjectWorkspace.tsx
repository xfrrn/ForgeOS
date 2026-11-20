import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon, Status, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import './project-workspace.css'

export type WorkspaceProject = {
  id: number
  path: string
  repository?: string
  repository_id?: number
  check_id?: number
  check_status?: string
  metrics?: { running: number; pending_approvals: number; awaiting_acceptance: number }
}

const workspaceTabs = [
  { key: 'overview', href: '', label: '概览' },
  { key: 'tasks', href: '/tasks', label: '任务' },
  { key: 'reviews', href: '/reviews', label: '审核与验收' },
  { key: 'materials', href: '/materials', label: '项目资料' },
] as const

const materialSections = [
  { href: '/materials', label: '项目资料' },
  { href: '/decisions', label: '项目决策' },
  { href: '/shares', label: '共享经验' },
] as const

export type ProjectWorkspaceTab = typeof workspaceTabs[number]['key']

export function projectDisplayName(project: WorkspaceProject) {
  return (project.repository || project.path).split(/[\\/]/).filter(Boolean).at(-1) || '项目 #' + project.id
}

export function ProjectWorkspace({ projectId, currentTab, taskId, section, request, children }: {
  projectId: number
  currentTab: ProjectWorkspaceTab
  taskId?: number
  section?: string
  request: ApiRequest
  children: ReactNode
}) {
  const [loaded, setLoaded] = useState<{ id: number; project?: WorkspaceProject; error?: string }>()
  const [revision, setRevision] = useState(0)
  const [checking, setChecking] = useState(false)
  const [checkError, setCheckError] = useState('')
  const lifetime = useRef<AbortController | null>(null)
  const project = loaded?.id === projectId ? loaded.project : undefined
  const loadError = loaded?.id === projectId ? loaded.error : undefined
  const base = `#/projects/${projectId}`

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    setChecking(false); setCheckError('')
    return () => controller.abort()
  }, [projectId, request])

  useEffect(() => {
    const controller = new AbortController()
    request<WorkspaceProject>(`/projects/${projectId}`, { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setLoaded({ id: projectId, project: value })
    }).catch(err => {
      if (!controller.signal.aborted) setLoaded(old => ({
        id: projectId,
        project: old?.id === projectId ? old.project : undefined,
        error: err instanceof Error ? err.message : '无法读取项目。',
      }))
    })
    return () => controller.abort()
  }, [projectId, currentTab, request, revision])

  async function check(value: WorkspaceProject) {
    const signal = lifetime.current?.signal
    if (checking || !signal) return
    setChecking(true); setCheckError('')
    try {
      const result = await request<{ id: number }>(`/projects/${value.id}/check`, {
        method: 'POST', body: JSON.stringify(value.check_id ? { retry_task_id: value.check_id } : {}), signal,
      })
      if (!signal.aborted) location.hash = `/projects/${value.id}/tasks/${result.id}`
    } catch (err) {
      if (!signal.aborted) setCheckError(err instanceof Error ? err.message : '无法派发检查。')
    } finally {
      if (!signal.aborted) setChecking(false)
    }
  }

  const title = project ? projectDisplayName(project) : '项目 #' + projectId
  const overview = currentTab === 'overview' && (!section || section === 'overview')
  const canCheck = project && (!project.check_id || project.check_status === 'DONE' || project.check_status === 'FAILED')
  return <div className="project-workspace">
    <nav className="project-breadcrumb" aria-label="项目路径">
      <ol>
        <li><a href="#/projects">项目</a></li>
        <li>{overview ? <span aria-current="page">{title}</span> : <a href={base}>{title}</a>}</li>
        {taskId && <><li><a href={base + '/tasks'}>任务</a></li><li><span aria-current="page">#{taskId}</span></li></>}
      </ol>
    </nav>
    {!project ? <>
      <PageHeading title={title}/>
      <div className="panel" role={loadError ? 'alert' : 'status'}>
        <EmptyState icon={loadError ? 'AlertCircle' : 'Layers3'} title={loadError ? '读取项目失败' : '正在读取项目…'} description={loadError}>
          {loadError && <button className="button" type="button" onClick={() => { setLoaded(undefined); setRevision(n => n + 1) }}>重试</button>}
        </EmptyState>
      </div>
    </> : <>
      <header className="project-workspace-heading">
        <strong className="project-workspace-name"><Icon name="Layers3" size={26}/><span>{title}</span></strong>
        <p className="project-repository"><Icon name="GitBranch" size={16}/><span>{project.repository || project.path}</span></p>
      </header>
      <nav className="project-tabs" aria-label={title + '导航'}>
        {workspaceTabs.map(tab => <a key={tab.key} href={base + tab.href}
          aria-current={currentTab === tab.key ? 'page' : undefined}>{tab.label}</a>)}
      </nav>
      {currentTab === 'materials' && <nav className="project-secondary-links" aria-label="项目资料导航">
        {materialSections.map(item => <a key={item.href} href={base + item.href}
          aria-current={(section || 'materials') === item.href.slice(1) ? 'page' : undefined}>{item.label}</a>)}
      </nav>}
      {currentTab === 'tasks' && !taskId && section !== 'inbox' && <div className="project-secondary-links">
        <a className="button" href={base + '/inbox'}><Icon name="Inbox" size={16}/>讨论收件箱</a>
      </div>}
      {loadError && <p className="detail-problem" role="alert">刷新项目信息失败。 {loadError}
        <button className="button" type="button" onClick={() => setRevision(n => n + 1)}>重试</button>
      </p>}
      {children}
      {overview && <section className="panel detail-card project-operations">
        <h2>仓库操作</h2>
        {project.repository_id && <p>GitHub 仓库 ID · {project.repository_id}</p>}
        {project.check_status && <p>最近的 HEAD 检查 <Status value={project.check_status}/></p>}
        <div className="detail-actions">
          <a className="button" href={base + '/inbox'}><Icon name="Inbox" size={16}/>讨论收件箱</a>
          {project.check_id && <a className="button" href={base + '/tasks/' + project.check_id}>查看检查 #{project.check_id}</a>}
          {canCheck && <button className="button" type="button" disabled={checking} onClick={() => void check(project)}>
            {project.check_status === 'DONE' ? '检查当前 HEAD' : project.check_id ? '重新检查' : '运行 HEAD 检查'}
          </button>}
        </div>
        {checkError && <p className="detail-problem" role="alert">{checkError}</p>}
      </section>}
    </>}
  </div>
}
