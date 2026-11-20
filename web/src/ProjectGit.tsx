import { useEffect, useRef, useState } from 'react'
import { formatTime, Icon, type ApiRequest } from './shared'

type Commit = { sha: string; branch: string; subject?: string; date?: string; remote?: string }
type Pull = { local: Commit; remote: Commit; updated: boolean }
type Action = 'remote' | 'local' | 'pull'

function CommitDetails({ commit }: { commit?: Commit }) {
  if (!commit) return <p className="muted">等待读取提交</p>
  return <div className="project-git-commit">
    <strong>{[commit.remote, commit.branch].filter(Boolean).join(' / ') || '分离 HEAD'}</strong>
    <code>{commit.sha}</code>
    {commit.subject && <span>{commit.subject}</span>}
    {commit.date && <time className="muted" dateTime={commit.date}>{formatTime(commit.date)}</time>}
  </div>
}

export function ProjectGit({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const [local, setLocal] = useState<Commit>()
  const [remote, setRemote] = useState<Commit>()
  const [pending, setPending] = useState<Action>()
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
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
        <button className="button" type="button" disabled={Boolean(pending)} onClick={() => void run('remote')}>
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
      <button className="button primary" type="button" disabled={Boolean(pending)} onClick={() => void run('pull')}>
        <Icon name="GitBranch" size={16}/>{pending === 'pull' ? '正在拉取…' : '拉取远端更新'}
      </button>
    </div>
    {error && <p className="detail-problem" role="alert">{error}</p>}
    {notice && <p className="detail-notice" role="status">{notice}</p>}
  </section>
}
