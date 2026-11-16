import { useEffect, useState } from 'react'
import { formatTime, type ApiRequest } from './shared'

type Preview = { id: string; directory: string; state: 'active' | 'stopped' | 'expired'; url: string; expires_at: string }
type Listing = { enabled: boolean; gateway?: string; previews: Preview[] }

export function CandidatePreviews({ task, execution, commit, request }: { task: number; execution: number; commit: string; request: ApiRequest }) {
  const [open, setOpen] = useState(false)
  const [listing, setListing] = useState<Listing | null>(null)
  const [directory, setDirectory] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [revision, setRevision] = useState(0)
  const api = `/tasks/${task}/previews`
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    request<Listing>(api + '?execution_id=' + execution, { signal: controller.signal })
      .then(l => { if (!controller.signal.aborted) setListing(l) })
      .catch(() => { if (!controller.signal.aborted) setError('无法读取本机预览。') })
    return () => controller.abort()
  }, [api, execution, open, request, revision])
  async function change(id?: string) {
    setBusy(true)
    setError('')
    try {
      if (id) await request(api + '/' + id, { method: 'DELETE' })
      else await request(api, { method: 'POST', body: JSON.stringify({ execution_id: execution, commit, directory: directory.trim() }) })
      setRevision(n => n + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '预览操作失败。') }
    finally { setBusy(false) }
  }
  return <section><button type="button" className="detail-disclosure" aria-expanded={open} onClick={() => setOpen(!open)}>本机静态预览</button>
    {open && <div>
      {error && <p role="alert">{error}</p>}
      {listing?.enabled ? <><label>包含 index.html 的公开目录<input value={directory} onChange={e => setDirectory(e.target.value)} placeholder="例如 public" /></label>
        <button className="button" disabled={busy || !directory.trim()} onClick={() => void change()}>启动预览</button></>
        : listing && <p>控制面尚未配置本机预览端口。</p>}
      {listing?.previews.map(p => <div key={p.id}><strong>{p.directory}</strong> · {p.state === 'active' ? '运行中' : p.state === 'stopped' ? '已停止' : '已到期'} · 到期时间 {formatTime(p.expires_at)}
        {p.state === 'active' && <> <a href={p.url} target="_blank" rel="noreferrer">打开预览</a>
          {listing.gateway && <> · <a href={`https://p-${p.id}.${listing.gateway}/`} target="_blank" rel="noreferrer">打开私网预览</a></>}
          {' '}<button className="button" disabled={busy} onClick={() => void change(p.id)}>停止预览</button></>}
      </div>)}
      <button className="detail-text-button" onClick={() => { setError(''); setRevision(n => n + 1) }}>刷新预览状态</button>
    </div>}
  </section>
}
