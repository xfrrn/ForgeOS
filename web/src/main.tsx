import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ComponentProps, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import { Overview } from './Overview'
import { Approvals } from './Approvals'
import { NotificationCount, Notifications } from './Notifications'
import TaskDetail from './TaskDetail'
import { PreviewFeedback } from './PreviewFeedback'
import TaskList from './TaskList'
import { WorkerNodes } from './WorkerNodes'
import { DecisionQuestions } from './DecisionQuestions'
import { GlobalRules } from './GlobalRules'
import { ProjectInbox } from './ProjectInbox'
import { Projects } from './Projects'
import { ProjectCodeReviews } from './ProjectCodeReviews'
import { knowledgeSections, projectDisplayName, ProjectWorkspace, type WorkspaceProject } from './ProjectWorkspace'
import { SkillLibrary } from './SkillLibrary'
import { RepositoryCredentials } from './RepositoryCredentials'
import { AgentSettings } from './AgentSettings'
import { FeatureTopics } from './FeatureTopics'
import { ProjectChat } from './ProjectChat'
import { ProjectExecutionQueue } from './ProjectExecutionQueue'
import { ProjectTestChecks } from './ProjectTestChecks'
import { ApiError, Icon, projectHref, requestWith, type ApiRequest } from './shared'
import { EmptyState, PageHeading, SkeletonCard } from './ui'
import { credentialsHref, workbenchRoute, workbenchTitle, type ProjectRoute, type WorkbenchRoute } from './workbenchRoutes'
import './style.css'

const ProjectPlanning = lazy(() => import('./ProjectPlanning'))
const ProjectMaterials = lazy(() => import('./ProjectMaterials').then(module => ({ default: module.ProjectMaterials })))
const ProjectDecisions = lazy(() => import('./ProjectDecisions').then(module => ({ default: module.ProjectDecisions })))
const ProjectExperience = lazy(() => import('./ProjectExperience').then(module => ({ default: module.ProjectExperience })))
const ProjectShares = lazy(() => import('./ProjectShares').then(module => ({ default: module.ProjectShares })))
const KnowledgeModelSettings = lazy(() => import('./KnowledgeModelSettings').then(module => ({ default: module.KnowledgeModelSettings })))

type OwnerSession = { email: string }
type IconName = ComponentProps<typeof Icon>['name']

async function authRequest(path: '/auth/session' | '/auth/login' | '/auth/logout', init: RequestInit = {}) {
  let response: Response
  try { response = await fetch(path, { ...init, credentials: 'same-origin', cache: 'no-store', redirect: 'error' }) }
  catch (err) {
    if (init.signal?.aborted) throw err
    throw new Error('无法连接控制面，请检查服务后重试。')
  }
  if (!response.ok) {
    const message = response.status === 401 ? '邮箱或密码不正确。' :
      response.status === 429 ? '尝试次数过多，请稍后再试。' :
      response.status === 503 ? '工作台账号尚未配置，请联系部署者。' : '请求失败，请稍后重试。'
    throw new ApiError(response.status, message)
  }
  return response
}

async function readOwnerSession(response: Response): Promise<OwnerSession> {
  const session: unknown = await response.json().catch(() => null)
  if (!session || typeof session !== 'object' || !('email' in session) || typeof session.email !== 'string' || !session.email) {
    throw new Error('无法读取工作台登录状态，请重试。')
  }
  return { email: session.email }
}

function Brand() {
  return <span className="brand">
    <span className="brand-symbol" aria-hidden="true"><i/><i/><i/></span>
    <span className="brand-text">Forge<span>OS</span></span>
  </span>
}

function Login({ checking, onLogin, initialError }: { checking: boolean; onLogin: (session: OwnerSession) => void; initialError: string }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (checking || !email.trim() || !password || busy) return
    setBusy(true); setError('')
    try {
      const response = await authRequest('/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim(), password }),
      })
      onLogin(await readOwnerSession(response))
    }
    catch (err) { setError(err instanceof Error ? err.message : '登录失败，请重试。') }
    finally { setBusy(false) }
  }
  const message = error ?? initialError
  return <div className="connect-page">
    <header className="connect-header frame"><Brand/><span>AI 工程交付工作台</span></header>
    <main className="connect-main"><div className="panel connect-card">
      <div className="signin-icon"><Icon name="Sparkles" size={23}/></div>
      <h1>登录工作台</h1>
      <p className="connect-subtitle">统一协作、多智能体编排与自动化交付中台</p>
      {checking ? <p className="connect-subtitle" role="status">正在检查登录状态…</p> : <form onSubmit={submit} aria-busy={busy}>
        <label htmlFor="email">邮箱</label>
        <input id="email" name="email" type="email" required autoComplete="username" spellCheck={false}
          placeholder="owner@example.com"
          aria-describedby={message ? 'login-error' : undefined} value={email} onChange={e => setEmail(e.target.value)} />
        <label htmlFor="password">密码</label>
        <input id="password" name="password" type="password" required autoComplete="current-password"
          placeholder="输入访问密码"
          aria-describedby={message ? 'login-error' : undefined} value={password} onChange={e => setPassword(e.target.value)} />
        {message && <div id="login-error" className="auth-error" role="alert">{message}</div>}
        <button type="submit" className="button primary" disabled={busy}>{busy ? '正在登录…' : '进入工作台'}<Icon name="ArrowRight" size={16}/></button>
      </form>}
      <div className="connect-origin"><Icon name="Terminal" size={14}/><span>{location.host}</span></div>
    </div></main>
  </div>
}

function ReviewNav({ projectId, selected }: { projectId: number; selected: 'approvals' | 'acceptance' | 'questions' | 'code-reviews' | 'checks' }) {
  return <nav className="project-review-nav" aria-label="审核与验收">
    <a className="button" aria-current={selected === 'approvals' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=approvals')}>待审批</a>
    <a className="button" aria-current={selected === 'acceptance' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=acceptance')}>待验收</a>
    <a className="button" aria-current={selected === 'code-reviews' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=code-reviews')}>代码审查</a>
    <a className="button" aria-current={selected === 'checks' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=checks')}>测试与检查</a>
    <a className="button" aria-current={selected === 'questions' ? 'page' : undefined} href={projectHref(projectId, '/questions')}>待答问题</a>
  </nav>
}

function ProjectContent({ page, request, onChanged }: { page: ProjectRoute; request: ApiRequest; onChanged: () => void }) {
  const { projectId } = page
  switch (page.view) {
    case 'overview': return <Overview projectId={projectId} request={request}/>
    case 'agents': return <AgentSettings projectId={projectId} request={request}/>
    case 'features': return <FeatureTopics key={projectId} projectId={projectId} selected={page.feature} request={request}/>
    case 'chat': return <ProjectChat projectId={projectId} request={request}/>
    case 'tasks': return page.queue ? <ProjectExecutionQueue projectId={projectId} request={request}/> :
      <TaskList key={page.category + '/' + (page.status ?? '')} projectId={projectId}
        category={page.category} initialStatus={page.status} request={request}/>
    case 'reviews': return <>
      <ReviewNav projectId={projectId} selected={page.tab}/>
      {page.tab === 'acceptance' ? <TaskList projectId={projectId} queue request={request}/> :
        page.tab === 'code-reviews' ? <ProjectCodeReviews projectId={projectId} request={request}/> :
        page.tab === 'checks' ? <ProjectTestChecks projectId={projectId} request={request}/> : <Approvals projectId={projectId} request={request}/>}
    </>
    case 'questions': return <>
      <ReviewNav projectId={projectId} selected="questions"/>
      <DecisionQuestions projectId={projectId} request={request} onChanged={onChanged}/>
    </>
    case 'notifications': return <Notifications projectId={projectId} request={request} onRead={onChanged}/>
    case 'inbox': return <ProjectInbox project={projectId} request={request}/>
    case 'model-settings': return <Suspense fallback={<SkeletonCard label="正在打开模型设置…"/>}>
      <KnowledgeModelSettings projectId={projectId} request={request}/></Suspense>
    case 'planning': case 'retrospectives': return <Suspense fallback={<SkeletonCard label="正在打开项目规划…"/>}>
      <ProjectPlanning key={page.view} project={projectId} retrospectives={page.view === 'retrospectives'} selected={page.selected} selectedItem={page.item} request={request}/></Suspense>
    case 'materials': return <Suspense fallback={<SkeletonCard label="正在打开资料库…"/>}>
      <ProjectMaterials project={projectId}
        initialKey={page.key} initialRevision={page.revision} initialPath={page.path} request={request}/></Suspense>
    case 'decisions':
    case 'experience':
    case 'shares': return <Suspense fallback={<SkeletonCard label="正在打开知识库…"/>}>
      {page.view === 'decisions' ? <ProjectDecisions key={page.record} project={projectId} decision={page.record} request={request}/> :
        page.view === 'experience' ? <ProjectExperience project={projectId} entry={page.record} request={request}/> :
        page.view === 'shares' && <ProjectShares key={String(page.received)} project={projectId} receipt={page.record} received={page.received} request={request}/>}
    </Suspense>
    case 'task': return page.task.preview ?
      <PreviewFeedback key={page.task.preview + page.captureQuery} projectId={projectId} task={page.task.id} preview={page.task.preview} captureQuery={page.captureQuery} request={request}/> :
      <TaskDetail key={page.task.id} projectId={projectId} id={page.task.id} planRevision={page.task.planRevision} questionID={page.task.questionId}
        executionID={page.task.executionId} category={page.task.category} request={request} onChanged={onChanged}/>
  }
}

function MissingPage({ message = '找不到这条工作台路径' }: { message?: string }) {
  return <><PageHeading title="页面不存在"/><section className="panel"><EmptyState icon="AlertCircle" title={message}>
    <a className="button" href="#/projects">返回项目</a>
  </EmptyState></section></>
}

function LegacyTaskLink({ page, request, onProjectLoaded }: {
  page: Extract<WorkbenchRoute, { kind: 'legacy-task' }>
  request: ApiRequest
  onProjectLoaded: (project: WorkspaceProject) => void
}) {
  const [projectId, setProjectId] = useState<number>()
  const [failed, setFailed] = useState(false)
  const { task, path } = page
  useEffect(() => {
    const controller = new AbortController()
    void request<{ project_id: number }>(`/tasks/${task.id}`, { signal: controller.signal }).then(result => {
      if (controller.signal.aborted) return
      if (!Number.isSafeInteger(result.project_id) || result.project_id < 1) {
        setFailed(true)
      } else if (task.preview) {
        // Existing browser extensions match the original capture hash when handing off media.
        setProjectId(result.project_id)
      } else {
        location.replace(projectHref(result.project_id, path))
      }
    }).catch(() => { if (!controller.signal.aborted) setFailed(true) })
    return () => controller.abort()
  }, [path, request, task.id, task.preview])
  if (failed) return <MissingPage message="找不到此任务或无法读取所属项目"/>
  if (projectId && task.preview) return <ProjectWorkspace projectId={projectId} currentTab="tasks" taskId={task.id} section="task" onProjectLoaded={onProjectLoaded} request={request}>
    <PreviewFeedback projectId={projectId} task={task.id} preview={task.preview} captureQuery={page.captureQuery} request={request}/>
  </ProjectWorkspace>
  return <SkeletonCard label="正在定位任务所属项目…" lines={3}/>
}

function AppSidebar({
  page,
  activeProject,
  collapsed,
  mobileOpen,
  onToggleCollapse,
  onCloseMobile,
}: {
  page: WorkbenchRoute
  activeProject?: WorkspaceProject
  collapsed: boolean
  mobileOpen: boolean
  onToggleCollapse: () => void
  onCloseMobile: () => void
}) {
  const inProject = page.kind === 'project'
  const projectId = inProject ? page.projectId : undefined
  const base = projectId ? `#/projects/${projectId}` : ''
  const project = inProject && activeProject?.id === projectId ? activeProject : undefined
  const projectName = project ? projectDisplayName(project) : projectId ? '项目 #' + projectId : ''

  const globalItems: { kind: WorkbenchRoute['kind']; href: string; label: string; icon: IconName }[] = [
    { kind: 'projects', href: '#/projects', label: '项目', icon: 'FolderKanban' },
    { kind: 'skills', href: '#/skills', label: 'Skill 库', icon: 'Sparkles' },
    { kind: 'agents', href: '#/agents', label: 'Agent 管理', icon: 'Bot' },
    { kind: 'system', href: '#/workers', label: '运行环境', icon: 'Server' },
    { kind: 'credentials', href: '#/settings/credentials', label: '密钥与凭据', icon: 'KeyRound' },
  ]

  return <>
    <div className={'sidebar-backdrop' + (mobileOpen ? ' open' : '')} onClick={onCloseMobile} aria-hidden="true"/>
    <aside className={'app-sidebar' + (collapsed ? ' collapsed' : '') + (mobileOpen ? ' mobile-open' : '')} aria-label="主导航">
      <div className="sidebar-header">
        <a className="brand-link" aria-label="ForgeOS 项目首页" href="#/projects" onClick={onCloseMobile}><Brand/></a>
        <button type="button" className="icon-button sidebar-close-mobile" aria-label="关闭导航" onClick={onCloseMobile}>
          <Icon name="X" size={18}/>
        </button>
      </div>

      <div className="sidebar-body">
        {inProject ? <>
          <div className="sidebar-project-card">
            <a className="sidebar-back-projects" href="#/projects" title="返回所有项目" onClick={onCloseMobile}>
              <Icon name="ArrowLeft" size={13}/><span>所有项目</span>
            </a>
            <a className="sidebar-project-title" href={base} title={projectName} onClick={onCloseMobile}>
              <Icon name="Layers3" size={17}/><span>{projectName}</span>
            </a>
            {project && <div className="sidebar-project-repo" title={project.repository || project.path}>
              <Icon name="GitBranch" size={12}/><span>{project.repository || project.path}</span>
            </div>}
          </div>

          <nav className="sidebar-group" aria-label="协作与智能">
            <span className="sidebar-group-label">协作与智能</span>
            <a className="sidebar-nav-item" href={base} title="项目概览" onClick={onCloseMobile}
              aria-current={page.projectTab === 'overview' && page.view !== 'notifications' ? 'page' : undefined}>
              <Icon name="Compass" size={17}/><span>概览</span>
            </a>
            <a className="sidebar-nav-item ai-accent" href={base + '/features'} title="功能话题" onClick={onCloseMobile}
              aria-current={page.projectTab === 'features' ? 'page' : undefined}>
              <Icon name="Sparkles" size={17}/><span>功能话题</span><em className="sidebar-ai-pill">AI</em>
            </a>
            <a className="sidebar-nav-item ai-accent" href={base + '/chat'} title="项目聊天" onClick={onCloseMobile}
              aria-current={page.projectTab === 'chat' ? 'page' : undefined}>
              <Icon name="MessageSquare" size={17}/><span>项目聊天</span>
            </a>
          </nav>

          <nav className="sidebar-group" aria-label="规划与交付">
            <span className="sidebar-group-label">规划与交付</span>
            <a className="sidebar-nav-item" href={base + '/planning'} title="项目规划" onClick={onCloseMobile}
              aria-current={page.projectTab === 'planning' ? 'page' : undefined}>
              <Icon name="Layers3" size={17}/><span>项目规划</span>
            </a>
            {page.projectTab === 'planning' && <div className="sidebar-subnav" aria-label="项目规划子项">
              <a href={base + '/planning'} aria-current={page.view === 'planning' ? 'page' : undefined} onClick={onCloseMobile}>规划时间线</a>
              <a href={base + '/planning/retrospectives'} aria-current={page.view === 'retrospectives' ? 'page' : undefined} onClick={onCloseMobile}>阶段复盘</a>
            </div>}

            <a className="sidebar-nav-item" href={base + '/tasks'} title="项目任务" onClick={onCloseMobile}
              aria-current={page.projectTab === 'tasks' ? 'page' : undefined}>
              <Icon name="LayoutList" size={17}/><span>项目任务</span>
            </a>
            {page.projectTab === 'tasks' && <div className="sidebar-subnav" aria-label="任务子项">
              <a href={base + '/tasks'} aria-current={page.view === 'tasks' && !page.queue ? 'page' : undefined} onClick={onCloseMobile}>任务列表</a>
              <a href={base + '/tasks?tab=queue'} aria-current={page.view === 'tasks' && page.queue ? 'page' : undefined} onClick={onCloseMobile}>执行队列</a>
              <a href={base + '/inbox'} aria-current={page.view === 'inbox' ? 'page' : undefined} onClick={onCloseMobile}>讨论收件箱</a>
            </div>}

            <a className="sidebar-nav-item" href={base + '/reviews'} title="审核与验收" onClick={onCloseMobile}
              aria-current={page.projectTab === 'reviews' ? 'page' : undefined}>
              <Icon name="ShieldCheck" size={17}/><span>审核与验收</span>
            </a>
            {page.projectTab === 'reviews' && <div className="sidebar-subnav" aria-label="审核子项">
              <a href={base + '/reviews?tab=approvals'} aria-current={page.view === 'reviews' && page.tab === 'approvals' ? 'page' : undefined} onClick={onCloseMobile}>待审批</a>
              <a href={base + '/reviews?tab=acceptance'} aria-current={page.view === 'reviews' && page.tab === 'acceptance' ? 'page' : undefined} onClick={onCloseMobile}>待验收</a>
              <a href={base + '/reviews?tab=code-reviews'} aria-current={page.view === 'reviews' && page.tab === 'code-reviews' ? 'page' : undefined} onClick={onCloseMobile}>代码审查</a>
              <a href={base + '/reviews?tab=checks'} aria-current={page.view === 'reviews' && page.tab === 'checks' ? 'page' : undefined} onClick={onCloseMobile}>测试与检查</a>
              <a href={base + '/questions'} aria-current={page.view === 'questions' ? 'page' : undefined} onClick={onCloseMobile}>待答问题</a>
            </div>}
          </nav>

          <nav className="sidebar-group" aria-label="知识与配置">
            <span className="sidebar-group-label">知识与配置</span>
            <a className="sidebar-nav-item" href={base + '/materials'} title="项目资料" onClick={onCloseMobile}
              aria-current={page.projectTab === 'materials' ? 'page' : undefined}>
              <Icon name="BookOpen" size={17}/><span>项目资料</span>
            </a>
            <a className="sidebar-nav-item" href={base + '/knowledge'} title="知识库" onClick={onCloseMobile}
              aria-current={page.projectTab === 'knowledge' ? 'page' : undefined}>
              <Icon name="BrainCircuit" size={17}/><span>知识库</span>
            </a>
            {page.projectTab === 'knowledge' && <div className="sidebar-subnav" aria-label="知识库子项">
              {knowledgeSections.map(item => <a key={item.key} href={base + item.href}
                aria-current={page.view === item.key ? 'page' : undefined} onClick={onCloseMobile}>{item.label}</a>)}
            </div>}
            <a className="sidebar-nav-item" href={base + '/agents'} title="Agent 管理" onClick={onCloseMobile}
              aria-current={page.projectTab === 'agents' ? 'page' : undefined}>
              <Icon name="Bot" size={17}/><span>Agent 管理</span>
            </a>
          </nav>
        </> : <nav className="sidebar-group" aria-label="工作台导航">
          <span className="sidebar-group-label">工作台</span>
          {globalItems.map(item => <a key={item.kind} className="sidebar-nav-item" href={item.href} title={item.label} onClick={onCloseMobile}
            aria-current={page.kind === item.kind ? 'page' : item.kind === 'projects' && page.kind === 'legacy-task' ? 'location' : undefined}>
            <Icon name={item.icon} size={17}/><span>{item.label}</span>
          </a>)}
        </nav>}
      </div>

      <div className="sidebar-footer">
        {inProject && <div className="sidebar-quick-links" aria-label="全局入口">
          <a className="sidebar-quick-link" href="#/skills" title="Skill 库" onClick={onCloseMobile}><Icon name="Sparkles" size={15}/><span>Skills</span></a>
          <a className="sidebar-quick-link" href="#/workers" title="运行环境" onClick={onCloseMobile}><Icon name="Server" size={15}/><span>环境</span></a>
          <a className="sidebar-quick-link" href={credentialsHref(projectId)} title="密钥与凭据" onClick={onCloseMobile}><Icon name="KeyRound" size={15}/><span>凭据</span></a>
        </div>}
        <button type="button" className="sidebar-collapse-btn" onClick={onToggleCollapse}
          aria-expanded={!collapsed} title={collapsed ? '展开侧边栏' : '收起侧边栏'}>
          <Icon name={collapsed ? 'PanelLeftOpen' : 'PanelLeftClose'} size={16}/>
          <span>{collapsed ? '展开侧边栏' : '收起侧边栏'}</span>
        </button>
      </div>
    </aside>
  </>
}

function readRoute() {
  return location.hash.slice(1) || '/projects'
}

function readCollapsed() {
  try { return localStorage.getItem('forgeos.sidebar.collapsed') === '1' } catch { return false }
}

function App() {
  const [owner, setOwner] = useState<OwnerSession | null>()
  const activeOwner = useRef<OwnerSession | null>(null)
  const [loginError, setLoginError] = useState('')
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutError, setLogoutError] = useState('')
  const [route, setRoute] = useState(readRoute)
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [activeProject, setActiveProject] = useState<WorkspaceProject>()
  const ownerMenu = useRef<HTMLDetailsElement>(null)
  const [notificationRevision, setNotificationRevision] = useState(0)
  const page = workbenchRoute(route)
  const projectId = page.kind === 'project' ? page.projectId : undefined
  const title = workbenchTitle(page)

  const openSession = useCallback((session: OwnerSession) => {
    activeOwner.current = session; setOwner(session); setLoginError(''); setLogoutError('')
  }, [])
  const closeSession = useCallback((reason = '') => {
    activeOwner.current = null; setOwner(null); setLoginError(reason); setLogoutError('')
  }, [])
  const handleProjectLoaded = useCallback((project: WorkspaceProject) => {
    setActiveProject(project)
  }, [])
  const toggleCollapse = useCallback(() => {
    setCollapsed(prev => {
      const next = !prev
      try { localStorage.setItem('forgeos.sidebar.collapsed', next ? '1' : '0') } catch { /* ignore */ }
      return next
    })
  }, [])

  async function logout() {
    if (!owner || loggingOut) return
    setLoggingOut(true); setLogoutError('')
    try {
      await authRequest('/auth/logout', { method: 'POST' })
      if (activeOwner.current === owner) closeSession()
    } catch {
      if (activeOwner.current === owner) setLogoutError('退出登录失败，请重试。')
    } finally { setLoggingOut(false) }
  }

  useEffect(() => {
    try { sessionStorage.removeItem('forgeos.owner') } catch { /* A previous version may have saved an Owner token. */ }
    const controller = new AbortController()
    void authRequest('/auth/session', { signal: controller.signal }).then(readOwnerSession)
      .then(session => { if (!controller.signal.aborted) openSession(session) })
      .catch(err => {
        if (!controller.signal.aborted) {
          const message = err instanceof ApiError && err.status === 401 ? '' :
            err instanceof Error ? err.message : '无法读取登录状态，请重试。'
          closeSession(message)
        }
      })
    return () => controller.abort()
  }, [openSession, closeSession])

  const request = useMemo(() => requestWith(() => {
    if (activeOwner.current === owner) closeSession('登录已失效，请重新登录。')
  }), [owner, closeSession])
  const onChanged = useCallback(() => setNotificationRevision(n => n + 1), [])

  useEffect(() => {
    const change = () => {
      setRoute(readRoute())
      setMobileNavOpen(false)
      if (ownerMenu.current) ownerMenu.current.open = false
      document.querySelector('.app-scroll')?.scrollTo(0, 0)
      window.scrollTo(0, 0)
    }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])

  useEffect(() => {
    if (!mobileNavOpen) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setMobileNavOpen(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [mobileNavOpen])

  useEffect(() => {
    if (owner && page.kind === 'projects' && route !== '/projects') location.replace('#/projects')
  }, [owner, page.kind, route])
  useEffect(() => { document.title = (owner ? title : '登录工作台') + ' · ForgeOS' }, [owner, title])

  if (!owner) return <Login checking={owner === undefined} initialError={loginError} onLogin={openSession}/>

  const currentProjectName = projectId
    ? (activeProject?.id === projectId ? projectDisplayName(activeProject) : '项目 #' + projectId)
    : undefined

  const viewStageKey = page.kind === 'project'
    ? `p-${page.projectId}-${page.view}-${page.view === 'reviews' ? page.tab : page.view === 'tasks' ? (page.queue ? 'q' : page.category) : page.view === 'task' ? page.task.id : ''}`
    : page.kind

  return <div className="app-shell">
    <a href="#content" className="skip-link" onClick={e => { e.preventDefault(); document.getElementById('content')?.focus() }}>跳到主要内容</a>
    <AppSidebar
      page={page}
      activeProject={activeProject}
      collapsed={collapsed}
      mobileOpen={mobileNavOpen}
      onToggleCollapse={toggleCollapse}
      onCloseMobile={() => setMobileNavOpen(false)}
    />
    <div className="app-main">
      <header className="app-topbar">
        <div className="topbar-left">
          <button type="button" className="icon-button mobile-nav-toggle" aria-label="打开导航菜单"
            aria-expanded={mobileNavOpen} onClick={() => setMobileNavOpen(open => !open)}>
            <Icon name="Menu" size={19}/>
          </button>
          <nav className="topbar-breadcrumb" aria-label="当前位置">
            <ol>
              {page.kind === 'project' ? <>
                <li><a href="#/projects">项目</a></li>
                <li>{page.view === 'overview'
                  ? <span aria-current="page">{currentProjectName}</span>
                  : <a href={projectHref(page.projectId)}>{currentProjectName}</a>}</li>
                {page.view === 'task' ? <>
                  <li><a href={projectHref(page.projectId, page.task.category ? '/tasks?category=' + page.task.category : '/tasks')}>任务</a></li>
                  <li><span aria-current="page">#{page.task.id}</span></li>
                </> : page.view !== 'overview' && <li><span aria-current="page">{title}</span></li>}
              </> : <li><span aria-current="page">{title}</span></li>}
            </ol>
          </nav>
        </div>

        <div className="header-actions">
          <a className="icon-button notification-button" href={projectId ? projectHref(projectId, '/notifications') : '#/notifications'}
            title={projectId ? '项目通知' : '通知'}
            aria-current={page.kind === 'notifications' || (page.kind === 'project' && page.view === 'notifications') ? 'page' : undefined}>
            <span className="sr-only">{projectId ? '项目通知' : '通知'}</span><Icon name="Bell" size={18}/>
            <NotificationCount key={projectId ?? 'all'} projectId={projectId} request={request} revision={notificationRevision}/>
          </a>
          <details className="owner-menu" ref={ownerMenu} onKeyDown={event => {
            if (event.key === 'Escape' && event.currentTarget.open) {
              event.currentTarget.open = false
              event.currentTarget.querySelector('summary')?.focus()
            }
          }}>
            <summary className="icon-button" aria-label="账号与系统" title="账号与系统">
              <span className="owner-avatar" aria-hidden="true">{owner.email.slice(0, 1).toUpperCase()}</span>
            </summary>
            <div className="owner-menu-panel panel">
              <div className="owner-menu-identity">
                <strong>Owner</strong>
                <small>{owner.email}</small>
              </div>
              <a className="button" href={credentialsHref(projectId)} onClick={() => { if (ownerMenu.current) ownerMenu.current.open = false }}>
                <Icon name="KeyRound" size={15}/>密钥与凭据
              </a>
              <a className="button" href="#/workers" onClick={() => { if (ownerMenu.current) ownerMenu.current.open = false }}>
                <Icon name="Server" size={15}/>执行节点与全局规则
              </a>
              <button type="button" className="button" disabled={loggingOut} onClick={() => void logout()}>
                <Icon name="LogOut" size={15}/>退出登录
              </button>
            </div>
          </details>
        </div>
      </header>

      <div className="app-scroll">
        <main id="content" tabIndex={-1} className={'content' + (page.kind === 'project' && (page.view === 'features' || page.view === 'chat') ? ' content-studio' : '')}>
          {logoutError && <p className="auth-error logout-error" role="alert">{logoutError}</p>}
          <div key={viewStageKey} className="view-stage">
          {page.kind === 'project' ? <ProjectWorkspace
            key={page.projectId}
            projectId={page.projectId}
            currentTab={page.projectTab}
            section={page.view}
            taskList={page.view === 'task' && page.task.category ? '/tasks?category=' + page.task.category : undefined}
            taskId={page.view === 'task' ? page.task.id : undefined}
            taskQueue={page.view === 'tasks' && page.queue}
            compactSubnav={collapsed}
            onProjectLoaded={handleProjectLoaded}
            request={request}>
            <ProjectContent key={page.view === 'materials' ? route : undefined} page={page} request={request} onChanged={onChanged}/>
          </ProjectWorkspace> :
            page.kind === 'projects' ? <Projects request={request}/> :
            page.kind === 'legacy-task' ? <LegacyTaskLink key={route} page={page} request={request} onProjectLoaded={handleProjectLoaded}/> :
            page.kind === 'notifications' ? <Notifications request={request} onRead={onChanged}/> :
            page.kind === 'skills' ? <SkillLibrary request={request}/> :
            page.kind === 'credentials' ? <RepositoryCredentials request={request} projectId={page.projectId} fromKnowledge={page.fromKnowledge}/> :
            page.kind === 'agents' ? <AgentSettings request={request}/> :
            page.kind === 'system' ? <><WorkerNodes request={request}/><GlobalRules request={request}/></> :
            <MissingPage/>}
          </div>
        </main>
      </div>
    </div>
  </div>
}

createRoot(document.getElementById('root')!).render(<App/>)
