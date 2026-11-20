import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import { Overview } from './Overview'
import { Approvals } from './Approvals'
import { NotificationCount, Notifications } from './Notifications'
import TaskDetail from './TaskDetail'
import { PreviewFeedback } from './PreviewFeedback'
import TaskList from './TaskList'
import { WorkerNodes } from './WorkerNodes'
import { DecisionQuestions } from './DecisionQuestions'
import { ProjectInbox } from './ProjectInbox'
import { Projects } from './Projects'
import { ProjectShares } from './ProjectShares'
import { ProjectDecisions } from './ProjectDecisions'
import { ProjectMaterials } from './ProjectMaterials'
import { Icon, requestWith } from './shared'
import { EmptyState, PageHeading } from './ui'
import './style.css'

const keyName = 'forgeos.owner'
const navigation = [
  { path: '/overview', label: '总览', icon: 'Layers3' },
  { path: '/tasks', label: '任务列表', icon: 'LayoutList' },
  { path: '/projects', label: '项目接入', icon: 'GitBranch' },
  { path: '/review', label: '验收队列', icon: 'ClipboardCheck' },
  { path: '/approvals', label: '审批中心', icon: 'ListChecks' },
  { path: '/questions', label: '待答问题', icon: 'MessageSquare' },
  { path: '/workers', label: '执行节点', icon: 'Terminal' },
] as const

function storedOwner() { try { return sessionStorage.getItem(keyName) || '' } catch { return '' } }
function storeOwner(value: string) {
  try { if (value) sessionStorage.setItem(keyName, value); else sessionStorage.removeItem(keyName) } catch { /* The current page can still connect. */ }
}

function Brand() {
  return <span className="brand"><span className="brand-symbol" aria-hidden="true"><i/><i/><i/></span>Forge<span>OS</span></span>
}

function Connect({ onConnect, initialError }: { onConnect: (key: string) => void; initialError: string }) {
  const [value, setValue] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(initialError)
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!value.trim() || busy) return
    setBusy(true); setError('')
    try { await requestWith(value.trim(), () => {})('/tasks?limit=1'); onConnect(value.trim()) }
    catch (err) { setError(err instanceof Error ? err.message : '无法连接控制面') }
    finally { setBusy(false) }
  }
  return <div className="connect-page">
    <header className="connect-header frame"><Brand/><span>交付工作台</span></header>
    <main className="connect-main"><div className="panel connect-card">
      <div className="signin-icon"><Icon name="KeyRound" size={25}/></div>
      <h1>连接控制面</h1><p>使用 Owner 令牌查看项目任务、检查执行结果并验收候选提交。</p>
      <form onSubmit={submit}><label htmlFor="owner">Owner 令牌</label>
        <input id="owner" name="owner" type="password" required autoComplete="off" spellCheck={false}
          value={value} placeholder="输入 Owner 令牌" onChange={e => setValue(e.target.value)} />
        <small>使用控制面启动时设置的 <code>FORGEOS_OWNER_TOKEN</code>。</small>
        {error && <div className="auth-error" role="alert">{error}</div>}
        <button className="button primary" disabled={busy}>{busy ? '正在连接…' : '进入工作台'}<Icon name="ArrowRight"/></button>
      </form>
      <div className="connect-origin"><Icon name="Terminal" size={14}/>{location.host}</div>
    </div></main>
  </div>
}

function App() {
  const [owner, setOwner] = useState(storedOwner)
  const activeOwner = useRef(owner)
  const [loginError, setLoginError] = useState('')
  const [route, setRoute] = useState(() => location.hash.slice(1) || '/overview')
  const [mobileNav, setMobileNav] = useState(false)
  const menuToggle = useRef<HTMLButtonElement>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const overview = route === '/overview' || route === '/'
  const running = route === '/tasks?status=RUNNING'
  const approvals = route === '/approvals'
  const queue = route === '/review'
  const [notificationRevision, setNotificationRevision] = useState(0)
  const notifications = route === '/notifications'
  const nodes = route === '/workers'
  const questions = route === '/questions'
  const projectRoute = /^\/projects\/([1-9]\d*)\/(inbox|materials|(?:decisions|shares)(?:\/[1-9]\d*)?)$/.exec(route)
  const projectID = projectRoute && Number.isSafeInteger(Number(projectRoute[1])) ? Number(projectRoute[1]) : undefined
  const inbox = projectID && projectRoute?.[2] === 'inbox'
  const materials = projectID && projectRoute?.[2] === 'materials'
  const projectPath = projectRoute?.[2].split('/')
  const recordID = projectPath?.[1] ? Number(projectPath[1]) : undefined
  const decisions = projectID && projectPath?.[0] === 'decisions' && (recordID === undefined || Number.isSafeInteger(recordID))
  const shares = projectID && projectPath?.[0] === 'shares' && (recordID === undefined || Number.isSafeInteger(recordID))
  const projects = route === '/projects' || Boolean(inbox || materials || decisions || shares)
  const idMatch = /^\/tasks\/([1-9]\d*)(?:\/previews\/([a-f0-9]{48})\/feedback(?:\?(.*))?|\/plans\/([1-9]\d{0,9})|\/questions\/([1-9]\d*))?$/.exec(route)
  const id = idMatch && Number.isSafeInteger(Number(idMatch[1])) && (!idMatch[5] || Number.isSafeInteger(Number(idMatch[5]))) ? Number(idMatch[1]) : undefined
  const valid = Boolean(overview || running || id || approvals || queue || notifications || nodes || questions || projects || route === '/tasks')
  const preview = idMatch?.[2]
  const activePath = overview ? '/overview' : projects ? '/projects' : running || id ? '/tasks' : route
  const title = overview ? '工作台总览' : running ? '执行中任务' : preview ? '预览反馈' : id ? '任务详情' : approvals ? '审批中心' : queue ? '验收队列' : notifications ? '通知' : nodes ? '执行节点' : questions ? '待答问题' : inbox ? '讨论收件箱' : shares ? '共享经验' : decisions ? '项目决策' : materials ? '项目资料' : projects ? '项目接入' : valid ? '任务列表' : '页面不存在'
  const logout = useCallback((reason = '') => {
    activeOwner.current = ''; setOwner(''); storeOwner(''); setLoginError(reason); setCounts({}); setMobileNav(false)
  }, [])
  const request = useMemo(() => requestWith(owner, () => {
    if (activeOwner.current === owner) logout('Owner 令牌已失效，请重新连接。')
  }), [owner, logout])
  const refreshCounts = useCallback(() => {
    if (owner) void request<{counts: Record<string, number>}>('/tasks?limit=1')
      .then(r => { if (activeOwner.current === owner) setCounts(r.counts) }).catch(() => {})
  }, [owner, request])
  useEffect(() => { if (id) refreshCounts() }, [id, refreshCounts])
  useEffect(() => {
    const change = () => { setRoute(location.hash.slice(1) || '/overview'); setMobileNav(false); window.scrollTo(0, 0) }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  useEffect(() => {
    if (!mobileNav) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMobileNav(false)
        menuToggle.current?.focus()
      }
    }
    const wide = window.matchMedia('(min-width: 1080px)')
    const closeOnWide = (event: MediaQueryListEvent) => { if (event.matches) setMobileNav(false) }
    window.addEventListener('keydown', closeOnEscape)
    wide.addEventListener('change', closeOnWide)
    return () => {
      window.removeEventListener('keydown', closeOnEscape)
      wide.removeEventListener('change', closeOnWide)
    }
  }, [mobileNav])
  useEffect(() => { document.title = (owner ? title : '连接控制面') + ' · ForgeOS' }, [owner, title])
  if (!owner) return <Connect initialError={loginError} onConnect={value => {
    activeOwner.current = value; storeOwner(value); setOwner(value); setLoginError('')
  }}/>
  const navCounts: Record<string, number> = { '/review': counts.WAITING_ACCEPTANCE, '/questions': counts.WAITING_INPUT }
  return <div className="shell">
    <a href="#content" className="skip-link" onClick={e => {e.preventDefault(); document.getElementById('content')?.focus()}}>跳到主要内容</a>
    <header className="topbar frame">
      <a className="brand-link" aria-label="ForgeOS 工作台总览" href="#/overview" onClick={() => setMobileNav(false)}><Brand/></a>
      <button ref={menuToggle} type="button" className="icon-button mobile-toggle" aria-label={mobileNav ? '关闭工作台导航' : '打开工作台导航'}
        aria-controls="workbench-nav" aria-expanded={mobileNav} onClick={() => setMobileNav(open => !open)}>
        <Icon name={mobileNav ? 'X' : 'Menu'} size={20}/>
      </button>
      <nav id="workbench-nav" className={'workbench-nav' + (mobileNav ? ' open' : '')} aria-label="工作台导航">
        {navigation.map(item => <a key={item.path} href={'#' + item.path}
          aria-current={activePath === item.path ? (route === item.path || overview ? 'page' : 'location') : undefined}
          onClick={() => { if (mobileNav) { setMobileNav(false); menuToggle.current?.focus() } }}>
          <Icon name={item.icon} size={18}/>{item.label}
          {navCounts[item.path] > 0 && <span className="nav-count">{navCounts[item.path] > 99 ? '99+' : navCounts[item.path]}</span>}
        </a>)}
      </nav>
      <div className="header-actions">
        <a className="icon-button notification-button" href="#/notifications" title="通知" aria-current={notifications ? 'page' : undefined} onClick={() => setMobileNav(false)}>
          <span className="sr-only">通知</span><Icon name="Bell" size={19}/><NotificationCount request={request} revision={notificationRevision}/>
        </a>
        <div className="owner">
          <span className="owner-avatar" aria-hidden="true">O</span>
          <div className="owner-copy"><strong>Owner</strong><small><i/>本地控制面</small></div>
        </div>
        <button type="button" className="icon-button" onClick={() => logout()} title="断开连接" aria-label="断开连接"><Icon name="LogOut" size={18}/></button>
      </div>
    </header>
    {mobileNav && <div className="nav-scrim" aria-hidden="true" onClick={() => { setMobileNav(false); menuToggle.current?.focus() }}/>}
    <main id="content" tabIndex={-1} className="content frame">
      {id && preview ? <PreviewFeedback key={route} task={id} preview={preview} captureQuery={idMatch?.[3]} request={request}/> :
        id ? <TaskDetail key={id} id={id} questionID={idMatch?.[5] ? Number(idMatch[5]) : undefined} planRevision={idMatch?.[4] ? Number(idMatch[4]) : undefined} request={request} onChanged={refreshCounts}/> :
        overview ? <Overview request={request} onCounts={setCounts}/> :
        approvals ? <Approvals request={request}/> :
        notifications ? <Notifications request={request} onRead={() => setNotificationRevision(n => n + 1)}/> :
        nodes ? <WorkerNodes request={request}/> :
        inbox && projectID ? <ProjectInbox key={projectID} project={projectID} request={request}/> :
        shares && projectID ? <ProjectShares key={route} project={projectID} receipt={recordID} request={request}/> :
        decisions && projectID ? <ProjectDecisions key={route} project={projectID} decision={recordID} request={request}/> :
        materials && projectID ? <ProjectMaterials key={projectID} project={projectID} request={request}/> :
        projects ? <Projects request={request}/> :
        questions ? <DecisionQuestions request={request} onChanged={refreshCounts}/> :
        valid ? <TaskList key={route} queue={queue} running={running} request={request} onCounts={setCounts}/> :
        <><PageHeading title="页面不存在"/><section className="panel"><EmptyState icon="AlertCircle" title="找不到这条工作台路径">
          <a className="button" href="#/tasks">返回任务列表</a>
        </EmptyState></section></>}
    </main>
  </div>
}

createRoot(document.getElementById('root')!).render(<App/>)
