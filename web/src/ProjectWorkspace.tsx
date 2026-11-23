import { useEffect, useState, type ReactNode } from 'react'
import { type ApiRequest } from './shared'
import { EmptyState, PageHeading, SkeletonCard } from './ui'
import { PlanningTaskLinks } from './PlanningTaskLinks'
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

export const workspaceTabs = [
  { key: 'overview', href: '', label: '概览' },
  { key: 'features', href: '/features', label: '功能' },
  { key: 'chat', href: '/chat', label: '聊天' },
  { key: 'agents', href: '/agents', label: 'Agent 管理' },
  { key: 'planning', href: '/planning', label: '项目规划' },
  { key: 'tasks', href: '/tasks', label: '任务' },
  { key: 'reviews', href: '/reviews', label: '审核与验收' },
  { key: 'materials', href: '/materials', label: '项目资料' },
  { key: 'knowledge', href: '/knowledge', label: '知识库' },
] as const

export const knowledgeSections = [
  { key: 'decisions', href: '/knowledge/decisions', label: '项目决策' },
  { key: 'experience', href: '/knowledge/experience', label: '项目经验' },
  { key: 'shares', href: '/knowledge/shares/received', label: '共享经验' },
  { key: 'model-settings', href: '/knowledge/model-settings', label: '模型设置' },
] as const

export type ProjectWorkspaceTab = typeof workspaceTabs[number]['key']

export function projectDisplayName(project: WorkspaceProject) {
  return (project.repository || project.path).split(/[\\/]/).filter(Boolean).at(-1) || '项目 #' + project.id
}

export function ProjectWorkspace({
  projectId,
  currentTab,
  taskId,
  section,
  taskQueue,
  compactSubnav = false,
  onProjectLoaded,
  request,
  children,
}: {
  projectId: number
  currentTab: ProjectWorkspaceTab
  taskId?: number
  section?: string
  taskQueue?: boolean
  taskList?: string
  compactSubnav?: boolean
  onProjectLoaded?: (project: WorkspaceProject) => void
  request: ApiRequest
  children: ReactNode
}) {
  const [loaded, setLoaded] = useState<{ id: number; project?: WorkspaceProject; error?: string }>()
  const [revision, setRevision] = useState(0)
  const project = loaded?.id === projectId ? loaded.project : undefined
  const loadError = loaded?.id === projectId ? loaded.error : undefined
  const base = `#/projects/${projectId}`

  useEffect(() => {
    const controller = new AbortController()
    request<WorkspaceProject>(`/projects/${projectId}`, { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) {
        setLoaded({ id: projectId, project: value })
        onProjectLoaded?.(value)
      }
    }).catch(err => {
      if (!controller.signal.aborted) setLoaded(old => ({
        id: projectId,
        project: old?.id === projectId ? old.project : undefined,
        error: err instanceof Error ? err.message : '无法读取项目。',
      }))
    })
    return () => controller.abort()
  }, [projectId, currentTab, request, revision, onProjectLoaded])

  const title = project ? projectDisplayName(project) : '项目 #' + projectId
  return <div className="project-workspace">
    {!project ? <>
      <PageHeading title={title}/>
      {loadError ? <div className="panel" role="alert">
        <EmptyState icon="AlertCircle" title="读取项目失败" description={loadError}>
          <button className="button" type="button" onClick={() => { setLoaded(undefined); setRevision(n => n + 1) }}>重试</button>
        </EmptyState>
      </div> : <SkeletonCard label="正在读取项目…" lines={4}/>}
    </> : <>
      {compactSubnav && currentTab === 'planning' && <nav className="project-secondary-links" aria-label="项目规划导航">
        <a href={base + '/planning'} aria-current={section === 'planning' ? 'page' : undefined}>规划</a>
        <a href={base + '/planning/retrospectives'} aria-current={section === 'retrospectives' ? 'page' : undefined}>复盘</a>
      </nav>}
      {compactSubnav && currentTab === 'knowledge' && <nav className="project-secondary-links" aria-label="知识库导航">
        {knowledgeSections.map(item => <a key={item.key} href={base + item.href}
          aria-current={section === item.key ? 'page' : undefined}>{item.label}</a>)}
      </nav>}
      {compactSubnav && currentTab === 'tasks' && !taskId && <nav className="project-secondary-links" aria-label="任务导航">
        <a href={base + '/tasks'} aria-current={!taskQueue && section !== 'inbox' ? 'page' : undefined}>任务列表</a>
        <a href={base + '/tasks?tab=queue'} aria-current={taskQueue ? 'page' : undefined}>执行队列</a>
        <a href={base + '/inbox'} aria-current={section === 'inbox' ? 'page' : undefined}>讨论收件箱</a>
      </nav>}
      {loadError && <p className="detail-problem" role="alert">刷新项目信息失败。 {loadError}
        <button className="button" type="button" onClick={() => setRevision(n => n + 1)}>重试</button>
      </p>}
      {taskId && <PlanningTaskLinks key={taskId} project={projectId} task={taskId} request={request}/>}
      {children}
    </>}
  </div>
}
