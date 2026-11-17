import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, type ApiRequest } from './shared'

type Receipt = {
  id: number
  decision_id?: number
  event_id?: number
  source?: string
  goal?: {plan_revision: number; children: {task_id: number; key: string; pr_url: string; candidate: string; merge_sha: string}[]}
  delivery?: {pr_number: number; candidate: string; merge_sha: string; target_branch: string}
  revision: number
  content_sha256: string
  decisions?: {fact: number; id: number; status: string}[]
  extraction: { session: string; facts: { statement: string; quote: string }[] } | null
  review: { level: string; verdict: string; rationale: string; evidence: string; session_id: string } | null
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
        if (!result.extraction || (result.extraction.facts.length && !result.review)) timer = setTimeout(read, 8000)
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
      {receipt?.goal && <><p>Goal 完成事件 #{receipt.event_id} · 计划修订 {receipt.goal.plan_revision}</p>
        <ul>{receipt.goal.children.map(c => <li key={c.task_id}><a href={`#/tasks/${c.task_id}`}>{c.key}</a> · {c.pr_url}
          <p>候选 <code>{c.candidate}</code> · 合并 <code>{c.merge_sha}</code></p></li>)}</ul></>}
      {receipt?.delivery && <><p>合并事件 #{receipt.event_id} · PR #{receipt.delivery.pr_number} · {receipt.delivery.target_branch}</p>
        <p>候选 <code>{receipt.delivery.candidate}</code> · 合并 <code>{receipt.delivery.merge_sha}</code></p></>}
      {receipt?.event_id && <><p>来源 · {receipt.source}</p><p>正文 SHA-256 · <code>{receipt.content_sha256}</code></p></>}
      <p role="status">{!receipt ? '正在读取记录…' : !extraction ? '等待 Curator 提取' : !extraction.facts.length ? '此修订没有提取到项目经验' :
        !review ? '建议等待 Management 审核' : review.verdict === 'approve' && review.level === 'D1' ? '此修订已通过项目经验审核' :
        review.verdict === 'revise' ? '建议已退回' : '需要 Owner 决定'}</p>
      {extraction && <><p>提取会话 · <code>{extraction.session}</code></p>
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
      {review && <><p>Management 审核 · {review.level} · <code>{review.session_id}</code></p><p>{review.rationale}</p><p>{review.evidence}</p></>}
      <button className="button" onClick={() => setRefresh(n => n + 1)}>刷新审核记录</button>
    </>}
  </section>
}
