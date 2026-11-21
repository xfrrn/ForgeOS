import { useEffect, useState, type FormEvent } from 'react'
import { ThemedSelect } from './ThemedSelect'
import { type ApiRequest } from './shared'
import { ProjectModelSettings } from './ProjectChatProvider'
import { PageHeading } from './ui'
import './repository-credentials.css'

const endpoint = '/projects/repository-credentials'
type Credential = { id: number; name: string; url: string; username: string; enabled: boolean; version: number; projects?: number[] }
type Catalog = { items: Credential[]; configured: boolean }

export function useRepositoryCredentials(request: ApiRequest, open = true) {
  const [catalog, setCatalog] = useState<Catalog>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setError('')
    void request<Catalog>(endpoint, {signal: controller.signal}).then(value => {
      if (!controller.signal.aborted) setCatalog(value)
    }).catch(cause => { if (!controller.signal.aborted) setError(cause.message || '无法读取凭据。') })
    return () => controller.abort()
  }, [open, request, revision])
  return {catalog, error, refresh: () => setRevision(n => n + 1)}
}

function canonicalURL(value: string) {
  try {
    const url = new URL(value.trim().replace(/^git@([^:]+):/, 'https://$1/'))
    if (!url.hostname || url.password || url.search || url.hash) return ''
    if (url.protocol === 'ssh:') {
      if (url.username !== 'git' || (url.port && url.port !== '22')) return ''
    } else if (url.protocol !== 'https:' || url.username || url.port) return ''
    const path = url.pathname.replace(/^\/+|\/+$/g, '').replace(/\.git$/, '')
    return 'https://' + url.hostname + '/' + (url.hostname === 'github.com' ? path.toLowerCase() : path) + '.git'
  } catch { return '' }
}

function isAccountCredential(credential: Credential) {
  return new URL(credential.url).origin === credential.url
}

export function IdentitySelect({catalog, value, onChange, url, disabled}: {
  catalog?: Catalog; value: number; onChange: (id: number) => void; url?: string; disabled?: boolean
}) {
  const repository = url && canonicalURL(url)
  const platform = repository && new URL(repository).origin
  const items = (catalog?.items ?? []).filter(c => c.id === value || (catalog?.configured && c.enabled &&
    (url === undefined || c.url === repository || c.url === platform)))
  const options = [{value: '0', label: '本机身份（默认）'}, ...items.map(c => ({
    value: String(c.id), label: c.name + ' · ' + (url === undefined ? c.url : c.username) +
      (isAccountCredential(c) ? '（账号凭据）' : '') + (c.enabled ? '' : '（已停用）')
  }))]
  if (value && !items.some(c => c.id === value)) options.push({value: String(value), label: '已绑定凭据 #' + value})
  return <ThemedSelect label="仓库访问身份" value={String(value)} onChange={next => onChange(Number(next))} options={options} disabled={disabled}/>
}

export function CredentialForm({request, initialURL = '', credential, onSaved, onCancel, onError}: {
  request: ApiRequest; initialURL?: string; credential?: Credential; onSaved: (id: number, url: string) => void; onCancel: () => void; onError?: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [scope, setScope] = useState('repository')
  const initialRepo = canonicalURL(initialURL)
  const initialHost = initialRepo ? new URL(initialRepo).hostname : ''
  const account = credential ? isAccountCredential(credential) : scope === 'account'
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy) return
    const fields = new FormData(event.currentTarget)
    const body = credential ? {id: credential.id, action: 'replace', token: fields.get('token')} : {...Object.fromEntries(fields), scope}
    setBusy(true); setError('')
    try {
      const result = await request<{id: number}>(endpoint, {method: credential ? 'PUT' : 'POST', body: JSON.stringify(body)})
      onSaved(credential?.id ?? result.id, account ? initialURL : credential?.url ?? String(fields.get('url')))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存凭据失败。')
      onError?.()
    } finally { setBusy(false) }
  }
  return <form className="credential-form" onSubmit={submit}>
    <h3>{credential ? '设置令牌 · ' + credential.name : '添加仓库凭据'}</h3>
    <fieldset disabled={busy}>
      {!credential && <>
        <ThemedSelect label="适用范围" value={scope} onChange={setScope} disabled={busy} options={[
          {value: 'repository', label: '单个仓库'}, {value: 'account', label: 'Git 账号（同一平台的仓库）'}
        ]}/>
        <label>凭据名称<input name="name" required maxLength={100} placeholder="例如：团队仓库只读账号"/></label>
        {account ? <label>Git 平台域名<input key="platform" name="url" defaultValue={initialHost || 'github.com'} readOnly={Boolean(initialHost)}
          required placeholder="github.com" autoComplete="off" spellCheck={false}/></label> :
          <label>适用仓库<input key="repository" name="url" defaultValue={initialURL} required placeholder="https://github.com/owner/repo.git" autoComplete="off" spellCheck={false}/></label>}
        <label>Git 用户名<input name="username" required maxLength={256} autoComplete="off" spellCheck={false}/></label>
      </>}
      <label>访问令牌<input name="token" type="password" required maxLength={4096} autoComplete="new-password"/></label>
      <p className="muted">{account ? '填写该平台签发的个人访问令牌。可读取的仓库由账号和令牌的授权范围决定。' : '请使用具有该仓库读取权限的访问令牌。'}</p>
      <div className="detail-actions"><button className="button primary">{busy ? '正在保存…' : '保存并启用'}</button>
        <button type="button" className="button" onClick={onCancel}>取消</button></div>
    </fieldset>
    {error && <p role="alert" className="detail-problem">{error}</p>}
  </form>
}

export function RepositoryCredentials({request, projectId}: {request: ApiRequest; projectId?: number}) {
  const {catalog, error, refresh} = useRepositoryCredentials(request)
  const [editing, setEditing] = useState<number>()
  const [confirm, setConfirm] = useState<number>()
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [problem, setProblem] = useState('')
  function saved() { setEditing(undefined); setMessage('凭据已保存并启用。'); setProblem(''); refresh() }
  async function disable(id: number) {
    setBusy(true); setMessage(''); setProblem('')
    try {
      await request(endpoint, {method: 'PUT', body: JSON.stringify({id, action: 'disable'})})
      setConfirm(undefined); setMessage('已停用凭据。'); refresh()
    } catch (cause) { setProblem(cause instanceof Error ? cause.message : '停用失败。') }
    finally { setBusy(false) }
  }
  return <div className="credential-page">
    <PageHeading title="密钥与凭据" actions={<button className="button primary" disabled={!catalog?.configured || editing !== undefined || busy}
      onClick={() => {setEditing(0); setConfirm(undefined); setMessage('')}}>添加仓库凭据</button>}/>
    <ProjectModelSettings request={request} projectId={projectId}/>
    <section className="panel detail-card"><h2>本机身份</h2>
      <p>默认沿用 ForgeOS 控制面服务账号的身份。访问 GitHub 时使用该账号的 gh 登录。</p></section>
    <section className="panel detail-card"><h2>仓库访问凭据</h2>
      {!catalog && !error && <p role="status">正在读取凭据…</p>}
      {error && <p role="alert">{error} <button className="button" onClick={refresh}>重试</button></p>}
      {catalog && !catalog.configured && <p>控制面尚未配置 OpenBao 仓库凭据存储。本机身份可继续使用。</p>}
      {catalog && !catalog.items.length && editing === undefined && <p className="muted">尚未添加仓库凭据。</p>}
      {editing === 0 && <CredentialForm request={request} onSaved={saved} onCancel={() => setEditing(undefined)} onError={refresh}/>}
      {catalog?.items.map(c => <article key={c.id} className="credential-row">
        <h3>{c.name}</h3><p>{isAccountCredential(c) ? 'Git 账号' : '单个仓库'} · {c.username} · {c.url}</p>
        <p className="muted">{c.enabled ? '已启用' : c.version ? '已停用' : '待设置令牌'}
          {c.projects?.map(id => <a key={id} href={'#/projects/' + id}>项目 #{id}</a>)}</p>
        {editing === c.id ? <CredentialForm request={request} credential={c} onSaved={saved} onCancel={() => setEditing(undefined)}/> :
          confirm === c.id ? <div><p>停用后，已绑定项目下次同步时需要切换身份或重新启用此凭据。</p>
            <div className="detail-actions"><button className="button" disabled={busy} onClick={() => void disable(c.id)}>确认停用</button>
              <button className="button" disabled={busy} onClick={() => setConfirm(undefined)}>取消</button></div></div> :
            <div className="detail-actions"><button className="button" disabled={busy || editing !== undefined || !catalog.configured}
              onClick={() => {setEditing(c.id); setConfirm(undefined)}}>{c.enabled ? '更换令牌' : '设置并启用'}</button>
              {c.enabled && <button className="button" disabled={busy || editing !== undefined} onClick={() => setConfirm(c.id)}>停用</button>}</div>}
      </article>)}
      {message && <p className="detail-notice" role="status">{message}</p>}
      {problem && <p className="detail-problem" role="alert">{problem}</p>}
    </section>
  </div>
}
