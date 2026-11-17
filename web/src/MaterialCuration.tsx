import { useEffect, useState } from 'react'
import type { ApiRequest } from './shared'

type Receipt = {
  extraction: { session: string; facts: { statement: string; quote: string }[] } | null
  review: { level: string; verdict: string; rationale: string; evidence: string; session_id: string } | null
}

export function MaterialCuration({path, request}: {path: string; request: ApiRequest}) {
  const [open, setOpen] = useState(false)
  const [receipt, setReceipt] = useState<Receipt>()
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
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
  return <section>
    <button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>知识提取与审核</button>
    {open && <>
      {error && <p role="alert">{error}</p>}
      <p role="status">{!receipt ? '正在读取记录…' : !extraction ? '等待 Curator 提取' : !extraction.facts.length ? '此修订没有提取到项目经验' :
        !review ? '建议等待 Management 审核' : review.verdict === 'approve' && review.level === 'D1' ? '此修订已通过项目经验审核' :
        review.verdict === 'revise' ? '建议已退回' : '需要 Owner 决定'}</p>
      {extraction && <><p>提取会话 · <code>{extraction.session}</code></p>
        {extraction.facts.map((f,i) => <div key={i}><p>{f.statement}</p><blockquote>{f.quote}</blockquote></div>)}</>}
      {review && <><p>Management 审核 · {review.level} · <code>{review.session_id}</code></p><p>{review.rationale}</p><p>{review.evidence}</p></>}
      <button className="button" onClick={() => setRefresh(n => n + 1)}>刷新审核记录</button>
    </>}
  </section>
}
