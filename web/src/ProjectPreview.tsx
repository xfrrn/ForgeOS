import { useEffect, useState } from 'react'
import { Icon, type ApiRequest } from './shared'

type Action = 'start' | 'stop' | 'status'
type Preview = {
  enabled: boolean
  state: 'stopped' | 'running' | 'unknown'
  phase?: 'READY' | 'RUNNING' | 'DONE' | 'FAILED' | 'EXPIRED'
  action?: Action
  url?: string
  worker?: string
  report?: { commit?: string; error?: { stage: string; message: string } }
}

const descriptions = {
  start: '启动',
  stop: '停止',
  status: '检查',
}

export function ProjectPreview({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const [preview, setPreview] = useState<Preview>()
  const [error, setError] = useState('')
  const [loadError, setLoadError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const path = `/projects/${projectId}/preview`

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      let delay = 2500
      try {
        const next = await request<Preview>(path, { signal: controller.signal })
        if (next.phase !== 'READY' && next.phase !== 'RUNNING') delay = 5000
        if (!controller.signal.aborted) {
          setPreview(next)
          setLoadError('')
        }
      } catch (e) {
        if (!controller.signal.aborted) setLoadError(e instanceof Error ? e.message : '读取项目预览失败。')
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(read, delay)
      }
    }
    void read()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [path, request])

  async function send(action: Action) {
    setSubmitting(true)
    setError('')
    try {
      setPreview(await request<Preview>(path, { method: 'POST', body: JSON.stringify({ action, request_id: crypto.randomUUID() }) }))
    } catch (e) {
      setError(e instanceof Error ? e.message : '提交预览操作失败。')
    } finally {
      setSubmitting(false)
    }
  }

  const working = preview?.phase === 'READY' || preview?.phase === 'RUNNING'
  const busy = working || submitting
  const operation = descriptions[preview?.action ?? 'status']
  const status = preview?.phase === 'READY' ? `等待 Worker ${operation}` :
    preview?.phase === 'RUNNING' ? `预览 Agent 正在${operation}` :
    preview?.phase === 'EXPIRED' ? '操作回报超时，可检查当前实例' :
    preview?.state === 'running' ? '运行中' :
    preview?.state === 'unknown' ? '待核对' : '已停止'

  if (preview && !preview.enabled) return null
  return <section className="project-preview-bar" aria-labelledby="project-preview-heading">
    <div className="project-preview-info">
      <h2 id="project-preview-heading"><Icon name="Zap" size={16}/> 项目预览</h2>
      <span className={'project-preview-status' + (preview?.state === 'running' && !working ? ' running' : '')} role="status">
        {preview ? status : '正在读取…'}
      </span>
      {preview?.report?.commit && <span className="project-preview-commit">版本 <code>{preview.report.commit.slice(0, 12)}</code></span>}
    </div>
    {preview && <div className="detail-actions">
      {preview.url
        ? <a className="button primary" href={preview.url} target="_blank" rel="noreferrer"><Icon name="ExternalLink" size={14}/>打开预览</a>
        : <button className="button primary" type="button" disabled={busy} onClick={() => void send('start')}>启动预览</button>}
      {(preview.state !== 'stopped' || preview.phase === 'FAILED') &&
        <button className="button" type="button" disabled={busy} onClick={() => void send('stop')}>停止</button>}
      <button className="button" type="button" disabled={busy} onClick={() => void send('status')}>检查实例</button>
    </div>}
    {(error || loadError) && <p className="detail-problem" role="alert">{error || loadError}</p>}
    {!working && preview?.report?.error && <p className="detail-problem" role="alert">{preview.report.error.message}</p>}
  </section>
}
