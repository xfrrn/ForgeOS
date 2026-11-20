import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { type WorkspaceProject } from './ProjectWorkspace'
import { ApiError, type ApiRequest } from './shared'
import './project-registration.css'

type CloneSettings = { root: string; directory: string; local_root: string }
type CloneJob = { id: number; status: 'queued' | 'cloning' | 'done' | 'failed'; path?: string; project?: WorkspaceProject; error?: string }
export type ProjectRegistrationMethod = 'clone' | 'local'

const cloneJobKey = 'forgeos.pending-project-clone'

export function pendingProjectClone() {
  try {
    const id = Number(sessionStorage.getItem(cloneJobKey))
    return Number.isSafeInteger(id) && id > 0 ? id : undefined
  } catch { return undefined }
}

function rememberClone(id?: number) {
  try {
    if (id) sessionStorage.setItem(cloneJobKey, String(id))
    else sessionStorage.removeItem(cloneJobKey)
  } catch { /* The active page can still monitor the clone. */ }
}

export function ProjectRegistration({ open, request, onRegistered }: {
  open: boolean; request: ApiRequest; onRegistered: (project: WorkspaceProject, method: ProjectRegistrationMethod) => void
}) {
  const [method, setMethod] = useState<ProjectRegistrationMethod>('clone')
  const [path, setPath] = useState('')
  const [url, setURL] = useState('')
  const [started, setStarted] = useState(false)
  const [registrationError, setRegistrationError] = useState('')
  const [cloneID, setCloneID] = useState(pendingProjectClone)
  const [job, setJob] = useState<CloneJob>()
  const [cloneError, setCloneError] = useState('')
  const [pollError, setPollError] = useState('')
  const [settings, setSettings] = useState<CloneSettings>()
  const [settingsDraft, setSettingsDraft] = useState<string>()
  const [settingsError, setSettingsError] = useState('')
  const [settingsRevision, setSettingsRevision] = useState(0)
  const [savingSettings, setSavingSettings] = useState(false)
  const lifetime = useRef<AbortController | null>(null)
  const settingsRead = useRef<AbortController | null>(null)
  const settingsWrite = useRef(false)
  const sourceField = useRef<HTMLInputElement>(null)
  const directoryField = useRef<HTMLInputElement>(null)
  const editingSettings = settingsDraft !== undefined
  const cloneUnavailable = settings?.root === ''

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [request])

  useEffect(() => { if (open) sourceField.current?.focus() }, [open, method])
  useEffect(() => { if (editingSettings) directoryField.current?.focus() }, [editingSettings])

  useEffect(() => {
    if (!open || settingsWrite.current) return
    const controller = new AbortController()
    settingsRead.current = controller
    setSettingsError('')
    void request<CloneSettings>('/projects/clone-settings', { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setSettings(value)
    }).catch(cause => {
      if (!controller.signal.aborted) setSettingsError(cause instanceof Error ? cause.message : '无法读取克隆目录。')
    })
    return () => controller.abort()
  }, [open, request, settingsRevision])

  const updateClone = useCallback((value: CloneJob) => {
    setJob(value)
    setPollError('')
    if (value.status === 'queued' || value.status === 'cloning') {
      setCloneID(value.id)
      rememberClone(value.id)
      return
    }
    setCloneID(undefined)
    rememberClone()
    if (value.status === 'done' && value.project) {
      setURL('')
      onRegistered(value.project, 'clone')
    } else {
      setCloneError(value.error || (value.status === 'done' ? '无法读取已克隆项目，请刷新项目列表。' : '克隆失败，请核对仓库链接和访问权限。'))
    }
  }, [onRegistered])

  useEffect(() => {
    if (!cloneID) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const value = await request<CloneJob>(`/projects/clone/jobs/${cloneID}`, { signal: controller.signal })
        if (controller.signal.aborted) return
        updateClone(value)
        if (value.status === 'queued' || value.status === 'cloning') timer = setTimeout(read, 1500)
      } catch (cause) {
        if (controller.signal.aborted) return
        if (cause instanceof ApiError && cause.status === 404) {
          rememberClone()
          setCloneID(undefined)
          setPollError('')
          setCloneError('找不到上次的克隆任务，请重新发起。')
          return
        }
        setPollError('暂时无法读取克隆进度，稍后自动重试。')
        timer = setTimeout(read, 4000)
      }
    }
    void read()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [cloneID, request, updateClone])

  async function register(event: FormEvent) {
    event.preventDefault()
    const signal = lifetime.current?.signal
    const value = (method === 'clone' ? url : path).trim()
    if (started || !value || !signal || (method === 'clone' && (cloneID || editingSettings || cloneUnavailable))) return
    setStarted(true)
    setRegistrationError('')
    setCloneError('')
    try {
      const body = JSON.stringify(method === 'clone' ? { url: value } : { path: value })
      const options = { method: 'POST', body, signal }
      if (method === 'clone') {
        const value = await request<CloneJob>('/projects/clone', options)
        if (!signal.aborted) updateClone(value)
      } else {
        const value = await request<WorkspaceProject>('/projects', options)
        if (signal.aborted) return
        setPath('')
        onRegistered(value, 'local')
      }
    } catch (cause) {
      if (signal.aborted) return
      const detail = cause instanceof Error ? cause.message : '接入项目失败。'
      if (method === 'clone') setCloneError(detail)
      else setRegistrationError(cause instanceof ApiError && cause.status === 400 ? '请填写控制面允许目录中的 Git 仓库根路径。' : detail)
    } finally {
      if (!signal.aborted) setStarted(false)
    }
  }

  async function saveDirectory(event: FormEvent) {
    event.preventDefault()
    const signal = lifetime.current?.signal
    if (savingSettings || !signal || settingsDraft === undefined) return
    settingsRead.current?.abort()
    settingsWrite.current = true
    setSavingSettings(true)
    setSettingsError('')
    try {
      const value = await request<CloneSettings>('/projects/clone-settings', {
        method: 'PUT', body: JSON.stringify({ directory: settingsDraft.trim() }), signal,
      })
      if (signal.aborted) return
      setSettings(value)
      setSettingsDraft(undefined)
    } catch (cause) {
      if (!signal.aborted) setSettingsError(cause instanceof Error ? cause.message : '无法保存克隆目录。')
    } finally {
      settingsWrite.current = false
      if (!signal.aborted) setSavingSettings(false)
    }
  }

  const progress = cloneID && <p role="status" className="detail-notice">{pollError || (job?.status === 'queued' ? '已加入克隆队列…' : job?.status === 'cloning' ? '正在克隆仓库…' : '正在查询克隆进度…')}</p>
  const failure = cloneError && <p role="alert" className="detail-problem">{cloneError}</p>
  if (!open) return <>{progress}{failure}</>

  return <section id="project-registration" className="panel detail-card project-registration" aria-labelledby="project-registration-heading">
    <h2 id="project-registration-heading">接入项目</h2>
    <div className="project-registration-method" role="group" aria-label="接入方式">
      <button type="button" className="button" aria-pressed={method === 'clone'} disabled={started || savingSettings} onClick={() => setMethod('clone')}>从链接克隆</button>
      <button type="button" className="button" aria-pressed={method === 'local'} disabled={started || savingSettings} onClick={() => setMethod('local')}>登记本地仓库</button>
    </div>
    <form onSubmit={register}>
      {method === 'clone' ? <>
        <label htmlFor="repo-url">Git 仓库链接</label>
        <p id="repo-url-hint" className="muted">ForgeOS 会在控制面机器上克隆仓库并登记为项目。</p>
        <div className="project-input">
          <input key="url" ref={sourceField} id="repo-url" aria-describedby="repo-url-hint" inputMode="url" autoComplete="off" required
            placeholder="https://github.com/owner/repo.git" disabled={started || Boolean(cloneID)} value={url} onChange={event => setURL(event.target.value)}/>
          <button className="button primary" disabled={started || Boolean(cloneID) || editingSettings || cloneUnavailable || !url.trim()}>{started ? '正在提交…' : '克隆并接入'}</button>
        </div>
      </> : <>
        <label htmlFor="repo-path">本地仓库路径</label>
        <p id="repo-path-hint" className="muted">填写控制面机器上的仓库根目录。{settings?.local_root ? <>也可以填写相对于 <code>{settings.local_root}</code> 的路径。</> : '也可以填写相对于 FORGEOS_REPO_ROOT 的路径。'}</p>
        <div className="project-input">
          <input key="path" ref={sourceField} id="repo-path" aria-describedby="repo-path-hint" autoComplete="off" required
            value={path} disabled={started} onChange={event => setPath(event.target.value)}/>
          <button className="button primary" disabled={started || !path.trim()}>{started ? '正在登记…' : '登记仓库'}</button>
        </div>
        {registrationError && <p className="detail-problem" role="alert">{registrationError}</p>}
      </>}
    </form>
    {method === 'clone' && <div className="project-clone-directory">
      {cloneUnavailable ? <p>当前服务尚未配置 ForgeOS 托管仓库根目录。</p> : settings ? editingSettings ? <form onSubmit={saveDirectory}>
        <label htmlFor="repo-clone-directory">ForgeOS 克隆目录</label>
        <p id="repo-clone-directory-hint" className="muted">填写 <code>{settings.root}</code> 下的绝对路径或子目录。留空使用默认目录。</p>
        <div className="project-input">
          <input ref={directoryField} id="repo-clone-directory" aria-describedby="repo-clone-directory-hint" autoComplete="off"
            disabled={savingSettings} value={settingsDraft} onChange={event => setSettingsDraft(event.target.value)}/>
          <button type="submit" className="button" disabled={savingSettings}>{savingSettings ? '正在保存…' : '保存目录'}</button>
          <button type="button" className="button" disabled={savingSettings} onClick={() => { setSettingsDraft(undefined); setSettingsError('') }}>取消</button>
        </div>
      </form> : <div className="project-clone-location">
        <p>ForgeOS 克隆目录 <code>{settings.directory}</code></p>
        <button type="button" className="button" disabled={started || Boolean(cloneID)} onClick={() => setSettingsDraft(settings.directory)}>更改目录</button>
      </div> : !settingsError && <p role="status" className="muted">正在读取克隆目录…</p>}
      {settingsError && <p className="detail-problem" role="alert">{settingsError}{!settings && <> <button type="button" className="button" onClick={() => setSettingsRevision(n => n + 1)}>重试</button></>}</p>}
    </div>}
    {progress}{failure}
  </section>
}
