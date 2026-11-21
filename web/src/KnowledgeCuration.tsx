import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, formatTime, projectHref, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'

type Receipt = {
  extraction_processes?: ReviewProcess[]
  review_processes?: ReviewProcess[]
  id: number
  decision_id?: number
  event_id?: number
  source?: string
  goal?: {plan_revision: number; children: {task_id: number; key: string; pr_url: string; candidate: string; merge_sha: string}[]}
  document?: {path: string; commit: string; blob: string; part?: number; byte_offset?: number}
  content?: string
  delivery?: {pr_number: number; candidate: string; merge_sha: string; target_branch: string}
  revision: number
  content_sha256: string
  decisions?: {fact: number; id: number; status: string}[]
  extraction: { session: string; facts: { statement: string; quote: string }[]; profile?: StructuredProfile } | null
  review: { level: string; verdict: string; rationale: string; evidence: string; session_id: string; profile?: StructuredProfile } | null
  review_event?: { message_id: string; correlation_id: string; timestamp: string; from: { id: string }; to: { id: string } }
}

export function KnowledgeCuration({project, path, request}: {project: number; path: string; request: ApiRequest}) {
  const [open, setOpen] = useState(false)
  const [receipt, setReceipt] = useState<Receipt>()
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [draft, setDraft] = useState<{fact: number; key: string}>()
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  async function propose(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft || !receipt || saving) return
    const data = new FormData(event.currentTarget)
    const statement = String(data.get('statement')).trim(), validity = String(data.get('validity')).trim()
    if (!statement || !validity || new TextEncoder().encode(statement).length > 4096 || new TextEncoder().encode(validity).length > 1024) {
      setSaveError('请填写决定内容（最多 4096 字节）及适用条件（最多 1024 字节）。'); return
    }
    setSaving(true); setSaveError('')
    try {
      await request(`/projects/${project}/decisions`, { method: 'POST', body: JSON.stringify({
        request_key: draft.key, statement, validity, curation: {id: receipt.id, fact: draft.fact},
      }) })
      setDraft(undefined); setRefresh(n => n + 1)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : '无法保存决策提案。')
      if (e instanceof ApiError && e.status === 409) setRefresh(n => n + 1)
    } finally { setSaving(false) }
  }
  useEffect(() => {
    if (!open) return
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function read() {
      try {
        const result = await request<Receipt>(path, {signal: stop.signal})
        if (stop.signal.aborted) return
        setReceipt(result); setError('')
        if (!result.document && (!result.extraction || (result.extraction.facts.length && !result.review))) timer = setTimeout(read, 8000)
      } catch (e) { if (!stop.signal.aborted) setError(e instanceof Error ? e.message : '读取提取记录失败。') }
    }
    void read()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [open, path, request, refresh])
  const extraction = receipt?.extraction
  const review = receipt?.review
  const needsOwner = review?.verdict === 'needs_owner' || (review?.verdict === 'approve' && ['D2','D3'].includes(review.level))
  return <section>
    <button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>知识提取与审核</button>
    {open && <>
      {error && <p role="alert">{error}</p>}
      {receipt?.decision_id && <p>确认事件 #{receipt.revision} · SHA-256 · <code>{receipt.content_sha256}</code></p>}
      {receipt?.document && <><p>固定来源 · <code>{receipt.document.commit}:{receipt.document.path}</code></p>
        {receipt.document.part && <p>第 {receipt.document.part} 段 · 起始字节 {receipt.document.byte_offset}</p>}
        <p>正文 SHA-256 · <code>{receipt.content_sha256}</code></p>
        <details><summary>查看固定版本原文</summary><pre className="material-content">{receipt.content}</pre></details></>}
      {receipt?.goal && <><p>Goal 完成事件 #{receipt.event_id} · 计划修订 {receipt.goal.plan_revision}</p>
        <ul>{receipt.goal.children.map(c => <li key={c.task_id}><a href={projectHref(project, `/tasks/${c.task_id}`)}>{c.key}</a> · {c.pr_url}
          <p>候选 <code>{c.candidate}</code> · 合并 <code>{c.merge_sha}</code></p></li>)}</ul></>}
      {receipt?.delivery && <><p>合并事件 #{receipt.event_id} · PR #{receipt.delivery.pr_number} · {receipt.delivery.target_branch}</p>
        <p>候选 <code>{receipt.delivery.candidate}</code> · 合并 <code>{receipt.delivery.merge_sha}</code></p></>}
      {receipt?.event_id && <><p>来源 · {receipt.source}</p><p>正文 SHA-256 · <code>{receipt.content_sha256}</code></p></>}
      <p role="status">{!receipt ? '正在读取记录…' : receipt.document && !receipt.review ? '仓库文档变更记录，由 CLI 在工作区直接阅读。' : !extraction ? '等待 Curator 提取' : !extraction.facts.length ? '此修订没有提取到项目经验' :
        !review ? '建议等待 Management 审核' : review.verdict === 'approve' && review.level === 'D1' ? '此修订已通过项目经验审核' :
        review.verdict === 'revise' ? '建议已退回' : '需要 Owner 决定'}</p>
      <ManagementProcesses request={request} processes={receipt?.extraction_processes} title="Curator 提取进程"/>
      <ManagementProcesses request={request} processes={receipt?.review_processes} title="Management 知识审核进程"/>
      {extraction && <><p>{extraction.session.startsWith('api-') ? '提取模型 API 调用' : '提取会话'} · <code>{extraction.session}</code></p>
        {extraction.profile && <RolePermissions role="Curator" profile={extraction.profile}/>}
        {extraction.facts.map((f,i) => {
          const fact = i + 1
          return <div key={fact}><p>{f.statement}</p><blockquote>{f.quote}</blockquote>
            {receipt?.decisions?.filter(d => d.fact === fact).map(d => <p key={d.id}><a href={`#/projects/${project}/decisions/${d.id}`}>查看项目决策 #{d.id} · {d.status}</a></p>)}
            {needsOwner && (draft?.fact === fact ? <form onSubmit={propose} className="material-form"><fieldset disabled={saving}>
                <label htmlFor="curation-decision-statement">决定内容</label>
                <textarea id="curation-decision-statement" name="statement" defaultValue={f.statement} required rows={3}/>
                <label>适用条件<input name="validity" required/></label>
                <button className="button">保存为待确认提案</button>
                <button className="button" type="button" onClick={() => { setDraft(undefined); setSaveError('') }}>取消编辑</button>
              </fieldset></form> : <button className="button" disabled={Boolean(draft)}
                onClick={() => { setDraft({fact, key: 'proposal-' + crypto.getRandomValues(new Uint32Array(4)).join('-')}); setSaveError('') }}>从建议 {fact} 提出决策</button>)}
          </div>
        })}</>}
      {saveError && <p role="alert">{saveError}</p>}
      {review && <><p>Management 审核 · {review.level} · {review.session_id.startsWith('api-') ? '模型 API 调用' : '审核会话'} · <code>{review.session_id}</code></p>
        {receipt?.review_event && <details><summary>审核事件回执</summary>
          <p>消息编号 · <code>{receipt.review_event.message_id}</code></p>
          <p>关联编号 · <code>{receipt.review_event.correlation_id}</code></p>
          <p>{receipt.review_event.from.id} → {receipt.review_event.to.id} · <time dateTime={receipt.review_event.timestamp}>{formatTime(receipt.review_event.timestamp)}</time></p>
        </details>}
        {review.profile && <RolePermissions role="Management" profile={review.profile}/>}
        <p>{review.rationale}</p><p>{review.evidence}</p></>}
      <button className="button" onClick={() => setRefresh(n => n + 1)}>刷新审核记录</button>
    </>}
  </section>
}
