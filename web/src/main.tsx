import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import { NotificationCount, Notifications } from './Notifications'
import TaskDetail from './TaskDetail'
import { PreviewFeedback } from './PreviewFeedback'
import TaskList from './TaskList'
import { WorkerNodes } from './WorkerNodes'
import { DecisionQuestions } from './DecisionQuestions'
import { Projects } from './Projects'
import { ProjectShares } from './ProjectShares'
import { ProjectDecisions } from './ProjectDecisions'
import { ProjectMaterials } from './ProjectMaterials'
import { Icon, requestWith } from './shared'
import './style.css'

const keyName = 'forgeos.owner'
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
    <div className="connect-header"><Brand/><span>交付工作台</span></div>
    <main className="connect-main"><div className="connect-card">
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
  const [route, setRoute] = useState(() => location.hash.slice(1) || '/tasks')
  const [mobileNav, setMobileNav] = useState(false)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const queue = route === '/review'
  const [notificationRevision, setNotificationRevision] = useState(0)
  const notifications = route === '/notifications'
  const nodes = route === '/workers'
  const questions = route === '/questions'
  const projectRoute = /^\/projects\/([1-9]\d*)\/(materials|(?:decisions|shares)(?:\/[1-9]\d*)?)$/.exec(route)
  const projectID = projectRoute && Number.isSafeInteger(Number(projectRoute[1])) ? Number(projectRoute[1]) : undefined
  const materials = projectID && projectRoute?.[2] === 'materials'
  const projectPath = projectRoute?.[2].split('/')
  const recordID = projectPath?.[1] ? Number(projectPath[1]) : undefined
  const decisions = projectID && projectPath?.[0] === 'decisions' && (recordID === undefined || Number.isSafeInteger(recordID))
  const shares = projectID && projectPath?.[0] === 'shares' && (recordID === undefined || Number.isSafeInteger(recordID))
  const projects = route === '/projects' || Boolean(materials || decisions || shares)
  const idMatch = /^\/tasks\/([1-9]\d*)(?:\/previews\/([a-f0-9]{48})\/feedback(?:\?(.*))?|\/plans\/([1-9]\d{0,9}))?$/.exec(route)
  const id = idMatch && Number.isSafeInteger(Number(idMatch[1])) ? Number(idMatch[1]) : undefined
  const valid = Boolean(id || queue || notifications || nodes || questions || projects || route === '/tasks')
  const preview = idMatch?.[2]
  const title = preview ? '预览反馈' : id ? '任务详情' : queue ? '验收队列' : notifications ? '通知' : nodes ? '执行节点' : questions ? '待答问题' : shares ? '共享经验' : decisions ? '项目决策' : materials ? '项目资料' : projects ? '项目接入' : valid ? '任务列表' : '页面不存在'
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
    const change = () => { setRoute(location.hash.slice(1) || '/tasks'); setMobileNav(false); window.scrollTo(0, 0) }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  useEffect(() => { document.title = (owner ? title : '连接控制面') + ' · ForgeOS' }, [owner, title])
  if (!owner) return <Connect initialError={loginError} onConnect={value => {
    activeOwner.current = value; storeOwner(value); setOwner(value); setLoginError('')
  }}/>
  return <div className="shell">
    <a href="#content" className="skip-link" onClick={e => {e.preventDefault(); document.getElementById('content')?.focus()}}>跳到主要内容</a>
    <aside className={'sidebar' + (mobileNav ? ' open' : '')}>
      <a className="brand-link" aria-label="ForgeOS 任务列表" href="#/tasks"><Brand/></a>
      <div className="work-label">工作区</div>
      <nav aria-label="工作台导航">
        <a className={!queue && !notifications && !nodes && !questions && !projects ? 'active' : ''} aria-current={!queue && !notifications && !nodes && !questions && !projects ? 'page' : undefined} href="#/tasks" onClick={() => setMobileNav(false)}><Icon name="LayoutList"/>任务列表</a>
        <a className={projects ? 'active' : ''} aria-current={projects ? 'page' : undefined} href="#/projects" onClick={() => setMobileNav(false)}><Icon name="GitBranch"/>项目接入</a>
        <a className={queue ? 'active' : ''} aria-current={queue ? 'page' : undefined} href="#/review" onClick={() => setMobileNav(false)}><Icon name="ClipboardCheck"/>验收队列
          {counts.WAITING_ACCEPTANCE > 0 && <span className="nav-count">{counts.WAITING_ACCEPTANCE}</span>}</a>
        <a className={notifications ? 'active' : ''} aria-current={notifications ? 'page' : undefined} href="#/notifications"><Icon name="Inbox"/>通知<NotificationCount request={request} revision={notificationRevision}/></a>
        <a className={nodes ? 'active' : ''} aria-current={nodes ? 'page' : undefined} href="#/workers"><Icon name="Terminal"/>执行节点</a>
        <a className={questions ? 'active' : ''} aria-current={questions ? 'page' : undefined} href="#/questions" onClick={() => setMobileNav(false)}><Icon name="MessageSquare"/>待答问题
          {counts.WAITING_INPUT > 0 && <span className="nav-count">{counts.WAITING_INPUT}</span>}</a>
      </nav>
      <div className="sidebar-foot"><span className="owner-avatar">O</span><div><strong>Owner</strong><small>工作区维护者</small></div>
        <button className="logout" onClick={() => logout()} title="断开连接" aria-label="断开连接"><Icon name="LogOut" size={16}/></button></div>
    </aside>
    {mobileNav && <button className="nav-scrim" aria-label="关闭导航" onClick={() => setMobileNav(false)}/>}
    <div className="work-area"><header className="topbar">
      <button className="mobile-toggle" aria-label="切换导航" aria-expanded={mobileNav} onClick={() => setMobileNav(!mobileNav)}><Icon name="Menu"/></button>
      <div className="crumb"><Icon name="PanelLeft" size={16}/><span>工作区</span><b>/</b>
        {id ? <><a href="#/tasks">任务列表</a><b>/</b><strong className="mono">T-{id}</strong></> : <strong>{title}</strong>}</div>
      <div className="connected"><i/>本地控制面</div>
    </header>
    <main id="content" tabIndex={-1} className="content">
      {id && preview ? <PreviewFeedback key={route} task={id} preview={preview} captureQuery={idMatch?.[3]} request={request}/> :
        id ? <TaskDetail key={id} id={id} planRevision={idMatch?.[4] ? Number(idMatch[4]) : undefined} request={request} onChanged={refreshCounts}/> :
        notifications ? <Notifications request={request} onRead={() => setNotificationRevision(n => n + 1)}/> :
        nodes ? <WorkerNodes request={request}/> :
        shares && projectID ? <ProjectShares key={route} project={projectID} receipt={recordID} request={request}/> :
        decisions && projectID ? <ProjectDecisions key={route} project={projectID} decision={recordID} request={request}/> :
        materials && projectID ? <ProjectMaterials key={projectID} project={projectID} request={request}/> :
        projects ? <Projects request={request}/> :
        questions ? <DecisionQuestions request={request} onChanged={refreshCounts}/> :
        valid ? <TaskList key={queue ? 'review' : 'all'} queue={queue} request={request} onCounts={setCounts}/> :
        <div className="panel table-message"><h1>页面不存在</h1><a className="button" href="#/tasks">返回任务列表</a></div>}
    </main></div>
  </div>
}

createRoot(document.getElementById('root')!).render(<App/>)
