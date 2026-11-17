import { useEffect, useRef, useState } from 'react'
import { KnowledgeCuration } from './KnowledgeCuration'
import { type ApiRequest } from './shared'

type Change = {id: number; path: string; commit_sha: string; blob: string; previous_blob: string; problem: string}
type History = {head: string | null; items: Change[]}
const reasons: Record<string,string> = {deleted: '已删除', empty: '空文档', non_text: '非 UTF-8 文本', size_limit: '超过 256 KiB，待提取'}

export function RepositoryDocumentChanges({project, request}: {project: number; request: ApiRequest}) {
  const [history, setHistory] = useState<History>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [more, setMore] = useState(false)
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => active.current?.abort(), [project, request])
  const base = `/projects/${project}/document-changes`
  async function load(sync: boolean, before = 0) {
    active.current?.abort()
    const controller = new AbortController()
    active.current = controller
    setBusy(true); setError('')
    try {
      const page = await request<History>(base + (before ? '?before=' + before : ''), {method: sync ? 'POST' : 'GET', signal: controller.signal})
      if (controller.signal.aborted) return
      setHistory(old => ({head: page.head, items: before && old ? [...old.items, ...page.items] : page.items}))
      setMore(page.items.length === 30)
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '无法读取文档变更。')
    } finally { if (!controller.signal.aborted) setBusy(false) }
  }
  return <section className="panel detail-card"><h2>仓库文档变更</h2>
    <div className="detail-actions"><button className="button" disabled={busy} onClick={() => void load(false)}>查看变更记录</button>
      <button className="button" disabled={busy} onClick={() => void load(true)}>同步当前提交</button></div>
    {error && <p role="alert">{error}</p>}
    {history && <>{history.head ? <p>最近扫描的提交 · <code>{history.head}</code></p> : <p>尚未记录文档基线。</p>}
      {history.items.length === 0 && <p>暂无已记录的文档变更。</p>}
      {history.items.map(c => <details key={c.id} className="inventory-file"><summary><code>{c.path}</code> · {reasons[c.problem] || (c.previous_blob ? '修改' : '新增')}</summary>
        <p>提交 · <code>{c.commit_sha}</code></p>
        {c.previous_blob && <p>原 blob · <code>{c.previous_blob}</code></p>}
        {c.blob && <p>新 blob · <code>{c.blob}</code></p>}
        {!c.problem && <KnowledgeCuration project={project} path={`${base}/${c.id}/curation`} request={request}/>}
      </details>)}
      {more && <button className="button" disabled={busy} onClick={() => void load(false, history.items[history.items.length - 1].id)}>更早的文档变更</button>}
    </>}
  </section>
}
