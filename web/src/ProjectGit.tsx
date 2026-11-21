import { useEffect, useRef, useState } from 'react'
import { formatTime, Icon, type ApiRequest } from './shared'
import { IdentitySelect, useRepositoryCredentials } from './RepositoryCredentials'

type Commit = { sha: string; branch: string; subject?: string; date?: string; remote?: string }
type Pull = { local: Commit; remote: Commit; updated: boolean }
type Action = 'remote' | 'local' | 'pull'

function ProjectIdentity({projectId, request, close}: {projectId: number; request: ApiRequest; close: () => void}) {
  const {catalog, error: catalogError, refresh} = useRepositoryCredentials(request)
  const [identity, setIdentity] = useState<number>()
  const [url, setURL] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [revision, setRevision] = useState(0)
  const path = `/projects/${projectId}/git/identity`
  useEffect(() => {
    const controller = new AbortController()
    setError('')
    void request<{credential_id: number; url: string}>(path, {signal: controller.signal}).then(value => {
      if (!controller.signal.aborted) {setIdentity(value.credential_id); setURL(value.url)}
    }).catch(cause => { if (!controller.signal.aborted) setError(cause.message || '读取身份失败。') })
    return () => controller.abort()
  }, [request, path, revision])
  async function save() {
    setSaving(true); setError('')
    try {
      await request(path, {method: 'PUT', body: JSON.stringify({credential_id: identity})})
      close()
    } catch (cause) { setError(cause instanceof Error ? cause.message : '无法保存访问身份。') }
    finally { setSaving(false) }
  }
  return <div>
    {identity === undefined && !error && <p role="status">正在读取当前身份…</p>}
    <div className="credential-controls">
      {identity !== undefined && <IdentitySelect catalog={catalog} url={url} value={identity} onChange={setIdentity} disabled={saving}/>}
      <button className="button" disabled={saving || identity === undefined} onClick={() => void save()}>{saving ? '正在保存…' : '保存身份'}</button>
      <button className="button" disabled={saving} onClick={close}>关闭</button>
    </div>
    {(error || catalogError) && <p role="alert">{error || catalogError} <button className="button" onClick={() => {setRevision(n => n + 1); refresh()}}>重新读取</button></p>}
  </div>
}

function CommitDetails({ commit }: { commit?: Commit }) {
  if (!commit) return <p className="muted">等待读取提交</p>
  return <div className="project-git-commit">
    <strong>{[commit.remote, commit.branch].filter(Boolean).join(' / ') || '分离 HEAD'}</strong>
    <code>{commit.sha}</code>
    {commit.subject && <span>{commit.subject}</span>}
    {commit.date && <time className="muted" dateTime={commit.date}>{formatTime(commit.date)}</time>}
  </div>
}

export function ProjectGit({ projectId, request, onUpdated }: { projectId: number; request: ApiRequest; onUpdated?: () => void }) {
  const [local, setLocal] = useState<Commit>()
  const [remote, setRemote] = useState<Commit>()
  const [pending, setPending] = useState<Action>()
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [editingIdentity, setEditingIdentity] = useState(false)
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => active.current?.abort(), [])

  async function run(action: Action) {
    if (active.current) return
    const controller = new AbortController()
    active.current = controller
    setPending(action); setError(''); setNotice('')
    const path = `/projects/${projectId}/git/${action}`
    try {
      if (action === 'pull') {
        const value = await request<Pull>(path, {method: 'POST', signal: controller.signal})
        if (controller.signal.aborted) return
        setLocal(value.local); setRemote(value.remote)
        if (value.updated) onUpdated?.()
        setNotice(value.updated ? '本地分支已更新。' : value.local.sha === value.remote.sha
          ? '本地和远端已在同一提交。' : '本地分支领先远端，已保留本地提交。')
      } else {
        const value = await request<Commit>(path, {signal: controller.signal})
        if (!controller.signal.aborted) (action === 'local' ? setLocal : setRemote)(value)
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Git 操作失败。')
    } finally {
      active.current = null
      if (!controller.signal.aborted) setPending(undefined)
    }
  }

  return <section className="panel detail-card" aria-labelledby="project-git-heading">
    <h2 id="project-git-heading">仓库提交与同步</h2>
    <div className="project-git-grid">
      <div>
        <h3>远端提交</h3>
        <CommitDetails commit={remote}/>
        <button className="button" type="button" disabled={Boolean(pending) || editingIdentity} onClick={() => void run('remote')}>
          {pending === 'remote' ? '正在读取远端…' : '查看远端提交'}
        </button>
      </div>
      <div>
        <h3>本地提交</h3>
        <CommitDetails commit={local}/>
        <button className="button" type="button" disabled={Boolean(pending)} onClick={() => void run('local')}>
          {pending === 'local' ? '正在读取本地…' : '查看本地提交'}
        </button>
      </div>
    </div>
    <div className="detail-actions">
      <button className="button primary" type="button" disabled={Boolean(pending) || editingIdentity} onClick={() => void run('pull')}>
        <Icon name="GitBranch" size={16}/>{pending === 'pull' ? '正在拉取…' : '拉取远端更新'}
      </button>
      <button className="button" type="button" aria-expanded={editingIdentity} disabled={Boolean(pending) || editingIdentity}
        onClick={() => setEditingIdentity(true)}>更改访问身份</button>
    </div>
    {editingIdentity && <ProjectIdentity projectId={projectId} request={request} close={() => setEditingIdentity(false)}/>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {notice && <p className="detail-notice" role="status">{notice}</p>}
  </section>
}
