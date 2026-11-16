import { useEffect, useState } from 'react'
import type { ApiRequest } from './shared'
import type { FeedbackArtifactReceipt } from './FeedbackScreenshots'

export function FeedbackRecordings({ api, artifacts, request, saved }: {
  api: string; artifacts: FeedbackArtifactReceipt[]; request: ApiRequest; saved: () => void
}) {
  const [open, setOpen] = useState(false)
  const [clip, setClip] = useState<Blob | null>(null)
  const [playable, setPlayable] = useState(false)
  const [url, setUrl] = useState('')
  const [archived, setArchived] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    setPlayable(false)
    if (!clip) { setUrl(''); return }
    const source = URL.createObjectURL(clip)
    setUrl(source)
    return () => URL.revokeObjectURL(source)
  }, [clip])
  async function view(id: number) {
    setBusy(true); setError('')
    try { const blob = await request<Blob>(api + '/' + id, {}, 'blob'); setArchived(id); setClip(blob) }
    catch (e) { setError(e instanceof Error ? e.message : '无法回放录屏。') }
    finally { setBusy(false) }
  }
  async function upload() {
    if (!clip || archived || busy) return
    setBusy(true); setError('')
    try {
      await request(api, {method: 'POST', headers: {'Content-Type': 'video/webm'}, body: clip})
      setClip(null); saved()
    } catch (e) { setError(e instanceof Error ? e.message : '无法归档录屏。') }
    finally { setBusy(false) }
  }
  return <div><button className="detail-text-button" disabled={busy} aria-expanded={open}
    onClick={() => { setClip(null); setError(''); setOpen(!open) }}>{open ? '收起录屏' : `录屏证据（${artifacts.length}）`}</button>
    {open && <fieldset disabled={busy}><legend>录屏证据</legend>
      {artifacts.map(a => <p key={a.id}>{a.expired ? `录屏 #${a.id} 已到期` :
        <button className="detail-text-button" onClick={() => void view(a.id)}>回放录屏 #{a.id}</button>}
        {' '}{a.bytes} 字节 · SHA-256 <code>{a.sha256}</code></p>)}
      <label>选择 WebM 录屏<input type="file" accept="video/webm,.webm" onChange={e => {
        const file = e.target.files?.[0]; e.target.value = ''
        if (!file) return
        setError(''); setClip(null); setArchived(0)
        if (!file.size || file.size > 64 * 1024 * 1024) { setError('请选择 64 MiB 以内的 WebM 录屏。'); return }
        setClip(file)
      }}/></label>
      {url && clip && <><video key={url} aria-label={archived ? '已归档录屏' : '待确认录屏'} src={url}
        controls playsInline muted preload="auto" style={{display:'block', maxWidth:'100%'}}
        onLoadedData={() => setPlayable(true)} onError={() => { setPlayable(false); setError('此浏览器无法回放该录屏。') }}/>
        {!archived && <button className="button" disabled={busy || !playable} onClick={() => void upload()}>确认并提交录屏</button>}</>}
      {error && <p role="alert">{error}</p>}
    </fieldset>}</div>
}
