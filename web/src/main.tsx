import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
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
import { ProjectWorkspace } from './ProjectWorkspace'
import { SkillLibrary } from './SkillLibrary'
import { RepositoryCredentials } from './RepositoryCredentials'
import { AgentSettings } from './AgentSettings'
import { ProjectChat } from './ProjectChat'
import { ProjectExecutionQueue } from './ProjectExecutionQueue'
import { ProjectTestChecks } from './ProjectTestChecks'
import { ApiError, Icon, projectHref, requestWith, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import { credentialsHref, workbenchRoute, workbenchTitle, type ProjectRoute, type WorkbenchRoute } from './workbenchRoutes'
import './style.css'

const ProjectPlanning = lazy(() => import('./ProjectPlanning'))
const ProjectMaterials = lazy(() => import('./ProjectMaterials').then(module => ({default:module.ProjectMaterials})))
const ProjectDecisions = lazy(() => import('./ProjectDecisions').then(module => ({default:module.ProjectDecisions})))
const ProjectExperience = lazy(() => import('./ProjectExperience').then(module => ({default:module.ProjectExperience})))
const ProjectShares = lazy(() => import('./ProjectShares').then(module => ({default:module.ProjectShares})))
const KnowledgeModelSettings = lazy(() => import('./KnowledgeModelSettings').then(module => ({default:module.KnowledgeModelSettings})))

type OwnerSession = { email: string }

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
  return <span className="brand"><span className="brand-symbol" aria-hidden="true"><i/><i/><i/></span>Forge<span>OS</span></span>
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
      const response = await authRequest('/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim(), password }) })
      onLogin(await readOwnerSession(response))
    }
    catch (err) { setError(err instanceof Error ? err.message : '登录失败，请重试。') }
    finally { setBusy(false) }
  }
  const message = error ?? initialError
  return <div className="connect-page">
    <header className="connect-header frame"><Brand/><span>交付工作台</span></header>
    <main className="connect-main"><div className="panel connect-card">
      <div className="signin-icon"><Icon name="KeyRound" size={25}/></div>
      <h1>登录工作台</h1>
      {checking ? <p role="status">正在检查登录状态…</p> : <form onSubmit={submit} aria-busy={busy}>
        <label htmlFor="email">邮箱</label>
        <input id="email" name="email" type="email" required autoComplete="username" spellCheck={false}
          aria-describedby={message ? 'login-error' : undefined} value={email} onChange={e => setEmail(e.target.value)} />
        <label htmlFor="password">密码</label>
        <input id="password" name="password" type="password" required autoComplete="current-password"
          aria-describedby={message ? 'login-error' : undefined} value={password} onChange={e => setPassword(e.target.value)} />
        {message && <div id="login-error" className="auth-error" role="alert">{message}</div>}
        <button type="submit" className="button primary" disabled={busy}>{busy ? '正在登录…' : '登录'}<Icon name="ArrowRight"/></button>
      </form>}
      <div className="connect-origin"><Icon name="Terminal" size={14}/>{location.host}</div>
    </div></main>
  </div>
}

function ReviewNav({projectId, selected}: {projectId: number; selected: 'approvals' | 'acceptance' | 'questions' | 'code-reviews' | 'checks'}) {
  return <nav className="project-review-nav" aria-label="审核与验收">
    <a className="button" aria-current={selected === 'approvals' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=approvals')}>待审批</a>
    <a className="button" aria-current={selected === 'acceptance' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=acceptance')}>待验收</a>
    <a className="button" aria-current={selected === 'code-reviews' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=code-reviews')}>代码审查</a>
    <a className="button" aria-current={selected === 'checks' ? 'page' : undefined} href={projectHref(projectId, '/reviews?tab=checks')}>测试与检查</a>
    <a className="button" aria-current={selected === 'questions' ? 'page' : undefined} href={projectHref(projectId, '/questions')}>待答问题</a>
  </nav>
}

function ProjectContent({page, request, onChanged}: {page: ProjectRoute; request: ApiRequest; onChanged: () => void}) {
  const { projectId } = page
  switch (page.view) {
    case 'overview': return <Overview projectId={projectId} request={request}/>
    case 'agents': return <AgentSettings projectId={projectId} request={request}/>
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
    case 'model-settings': return <Suspense fallback={<p role="status">正在打开模型设置…</p>}>
      <KnowledgeModelSettings projectId={projectId} request={request}/></Suspense>
    case 'planning': case 'retrospectives': return <Suspense fallback={<p role="status">正在打开项目规划…</p>}>
      <ProjectPlanning key={page.view} project={projectId} retrospectives={page.view==='retrospectives'} selected={page.selected} request={request}/></Suspense>
    case 'materials': return <Suspense fallback={<p role="status">正在打开资料库…</p>}>
      <ProjectMaterials project={projectId}
        initialKey={page.key} initialRevision={page.revision} initialPath={page.path} request={request}/></Suspense>
    case 'decisions':
    case 'experience':
    case 'shares': return <Suspense fallback={<p role="status">正在打开知识库…</p>}>
      {page.view==='decisions' ? <ProjectDecisions key={page.record} project={projectId} decision={page.record} request={request}/> :
        page.view==='experience' ? <ProjectExperience project={projectId} entry={page.record} request={request}/> :
        page.view==='shares' && <ProjectShares key={String(page.received)} project={projectId} receipt={page.record} received={page.received} request={request}/>}
    </Suspense>
    case 'task': return page.task.preview ?
      <PreviewFeedback key={page.task.preview + page.captureQuery} projectId={projectId} task={page.task.id} preview={page.task.preview} captureQuery={page.captureQuery} request={request}/> :
      <TaskDetail key={page.task.id} projectId={projectId} id={page.task.id} planRevision={page.task.planRevision} questionID={page.task.questionId}
        executionID={page.task.executionId} category={page.task.category} request={request} onChanged={onChanged}/>
  }
}

function MissingPage({message = '找不到这条工作台路径'}: {message?: string}) {
  return <><PageHeading title="页面不存在"/><section className="panel"><EmptyState icon="AlertCircle" title={message}>
    <a className="button" href="#/projects">返回项目</a>
  </EmptyState></section></>
}

function LegacyTaskLink({page, request}: {page: Extract<WorkbenchRoute, {kind: 'legacy-task'}>; request: ApiRequest}) {
  const [projectId, setProjectId] = useState<number>()
  const [failed, setFailed] = useState(false)
  const { task, path } = page
  useEffect(() => {
    const controller = new AbortController()
    void request<{project_id: number}>(`/tasks/${task.id}`, {signal: controller.signal}).then(result => {
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
  if (projectId && task.preview) return <ProjectWorkspace projectId={projectId} currentTab="tasks" taskId={task.id} section="task" request={request}>
    <PreviewFeedback projectId={projectId} task={task.id} preview={task.preview} captureQuery={page.captureQuery} request={request}/>
  </ProjectWorkspace>
  return <p className="panel table-message" role="status">正在定位任务所属项目…</p>
}

function readRoute() {
  return location.hash.slice(1) || '/projects'
}

function App() {
  const [owner, setOwner] = useState<OwnerSession | null>()
  const activeOwner = useRef<OwnerSession | null>(null)
  const [loginError, setLoginError] = useState('')
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutError, setLogoutError] = useState('')
  const [route, setRoute] = useState(readRoute)
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
      if (ownerMenu.current) ownerMenu.current.open = false
      window.scrollTo(0, 0)
    }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  useEffect(() => {
    if (owner && page.kind === 'projects' && route !== '/projects') location.replace('#/projects')
  }, [owner, page.kind, route])
  useEffect(() => { document.title = (owner ? title : '登录工作台') + ' · ForgeOS' }, [owner, title])
  if (!owner) return <Login checking={owner === undefined} initialError={loginError} onLogin={openSession}/>
  return <div className="shell">
    <a href="#content" className="skip-link" onClick={e => {e.preventDefault(); document.getElementById('content')?.focus()}}>跳到主要内容</a>
    <header className="topbar frame">
      <a className="brand-link" aria-label="ForgeOS 项目首页" href="#/projects"><Brand/></a>
      <nav className="workbench-nav" aria-label="工作台导航">
        <a href="#/projects" aria-current={page.kind === 'projects' ? 'page' : page.kind === 'project' || page.kind === 'legacy-task' ? 'location' : undefined}>项目</a>
        <a href="#/skills" aria-current={page.kind === 'skills' ? 'page' : undefined}>Skill 库</a>
        <a href="#/agents" aria-current={page.kind === 'agents' ? 'page' : undefined}>Agent 管理</a>
        <a href="#/workers" aria-current={page.kind === 'system' ? 'page' : undefined}>运行环境</a>
      </nav>
      <div className="header-actions">
        <a className="icon-button notification-button" href={projectId ? projectHref(projectId, '/notifications') : '#/notifications'} title={projectId ? '项目通知' : '通知'}
          aria-current={page.kind === 'notifications' || (page.kind === 'project' && page.view === 'notifications') ? 'page' : undefined}>
          <span className="sr-only">{projectId ? '项目通知' : '通知'}</span><Icon name="Bell" size={19}/>
          <NotificationCount key={projectId ?? 'all'} projectId={projectId} request={request} revision={notificationRevision}/>
        </a>
        <details className="owner-menu" ref={ownerMenu} onKeyDown={event => {
          if (event.key === 'Escape' && event.currentTarget.open) {
            event.currentTarget.open = false
            event.currentTarget.querySelector('summary')?.focus()
          }
        }}>
          <summary className="icon-button" aria-label="账号与系统" title="账号与系统"><span className="owner-avatar" aria-hidden="true">O</span></summary>
          <div className="owner-menu-panel panel">
            <strong>Owner</strong><small>{owner.email}</small>
            <a className="button" href={credentialsHref(projectId)} onClick={() => { if (ownerMenu.current) ownerMenu.current.open = false }}>密钥与凭据</a>
            <a className="button" href="#/workers" onClick={() => { if (ownerMenu.current) ownerMenu.current.open = false }}>执行节点与全局规则</a>
            <button type="button" className="button" disabled={loggingOut} onClick={() => void logout()}><Icon name="LogOut" size={16}/>退出登录</button>
          </div>
        </details>
      </div>
    </header>
    <main id="content" tabIndex={-1} className="content frame">
      {logoutError && <p className="auth-error logout-error" role="alert">{logoutError}</p>}
      {page.kind === 'project' ? <ProjectWorkspace key={page.projectId} projectId={page.projectId} currentTab={page.projectTab} section={page.view}
        taskList={page.view === 'task' && page.task.category ? '/tasks?category=' + page.task.category : undefined}
        taskId={page.view === 'task' ? page.task.id : undefined} taskQueue={page.view === 'tasks' && page.queue} request={request}>
        <ProjectContent key={page.view === 'materials' ? route : undefined} page={page} request={request} onChanged={onChanged}/>
      </ProjectWorkspace> :
        page.kind === 'projects' ? <Projects request={request}/> :
        page.kind === 'legacy-task' ? <LegacyTaskLink key={route} page={page} request={request}/> :
        page.kind === 'notifications' ? <Notifications request={request} onRead={onChanged}/> :
        page.kind === 'skills' ? <SkillLibrary request={request}/> :
        page.kind === 'credentials' ? <RepositoryCredentials request={request} projectId={page.projectId} fromKnowledge={page.fromKnowledge}/> :
        page.kind === 'agents' ? <AgentSettings request={request}/> :
        page.kind === 'system' ? <><WorkerNodes request={request}/><GlobalRules request={request}/></> :
        <MissingPage/>}
    </main>
  </div>
}

createRoot(document.getElementById('root')!).render(<App/>)
