import { useState } from 'react'
import { ApiError, Fact, type ApiRequest } from './shared'

export type ProcessArchive = { central?: boolean; records: number; bytes: number; dropped_records: number; dropped_bytes: number; chain_sha256?: string }

export function ArchiveFacts({instance, archive}: {instance: number; archive: ProcessArchive}) {
  return <><Fact label="节点进程归档">实例 {instance} · {archive.records} 条 · {archive.bytes} 字节</Fact>
    {archive.chain_sha256 && <Fact label="归档链 SHA-256" code>{archive.chain_sha256}</Fact>}
    {archive.dropped_records > 0 && <Fact label="超出归档容量">{archive.dropped_records} 条 · {archive.dropped_bytes} 字节</Fact>}</>
}

export async function downloadFile(request: ApiRequest, path: string, file: string) {
  const blob = await request<Blob>(path, undefined, 'blob')
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = file
  link.click()
  setTimeout(() => URL.revokeObjectURL(link.href), 1000)
}

export function ArchiveDownload({ path, file, request, label = '下载进程日志 JSONL' }: { path: string; file: string; request: ApiRequest; label?: string }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function download() {
    setBusy(true); setError('')
    try { await downloadFile(request, path, file) }
    catch (e) {
      setError(e instanceof ApiError && e.status === 409 ? '节点尚未完成日志传输，请稍后重试。' :
        e instanceof ApiError && e.status === 410 ? '中央日志已到期，归档摘要仍可查阅。' :
        e instanceof Error ? e.message : '无法下载日志归档')
    }
    finally { setBusy(false) }
  }
  return <div><button disabled={busy} onClick={() => void download()}>{busy ? '正在下载…' : label}</button>
    {error && <p role="alert">{error}</p>}</div>
}
