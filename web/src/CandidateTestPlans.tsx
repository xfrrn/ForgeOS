import { CandidateRuleEvidence, type CandidateContext } from './CandidateRuleEvidence'
import { useEffect, useState } from 'react'
import type { ApiRequest } from './shared'
import { CaseEvidence, resultLabels, type CaseAttempt } from './CaseEvidence'

type Case = { id: string; criterion_id: string; playbook_id: string; objective: string; environment: string; tester: string; preconditions: string; steps: string[]; expected_result: string; evidence: string[] }
type Receipt = {
  candidate_context?: CandidateContext
  execution_state: string; case_results: CaseAttempt[]
  revision: number; state: string; basis: { candidate_context_sha256?: string; criteria: { id: string; text: string }[] }
  plan?: { session_id: string; cases: Case[]; clarifications: { criterion_id: string; question: string }[] }
  coverage?: { session_id: string; plan_sha256: string; items: { criterion_id: string; case_ids: string[]; state: string; rationale: string }[] }
}
const labels: Record<string, string> = { awaiting_plan: '等待 Test Agent 生成用例', awaiting_review: '等待 Management 审查覆盖', covered: '覆盖审查通过', gap: '需要补充用例', changes_requested: '需要修订计划', needs_clarification: '需要澄清', plan_failed: '生成计划失败', review_failed: '覆盖审查失败' }

export function CandidateTestPlans({ task, execution, commit, latest, state, attempts, required, current, request, refresh }: {
  attempts: number; required: boolean
  task: number; execution: number; commit: string; latest: number; state: string; current: boolean; request: ApiRequest; refresh: (message: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [revision, setRevision] = useState(0)
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [criteria, setCriteria] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const path = `/tasks/${task}/test-plans`
  const selected = revision || latest
  useEffect(() => {
    if (!open || !selected) return
    const controller = new AbortController()
    setReceipt(null)
    setError('')
    request<Receipt>(`${path}?execution_id=${execution}&revision=${selected}`, { signal: controller.signal })
      .then(data => { if (!controller.signal.aborted) setReceipt(data) })
      .catch(() => { if (!controller.signal.aborted) setError('无法读取测试计划。') })
    return () => controller.abort()
  }, [open, selected, path, execution, state, attempts, request])
  async function submit() {
    setBusy(true)
    setError('')
    try {
      await request<Receipt>(path, { method: 'POST', body: JSON.stringify({ execution_id: execution, commit, previous_revision: latest,
        criteria: criteria.split('\n').map(c => c.trim()).filter(Boolean) }) })
      setRevision(0)
      setCriteria('')
      refresh('已请求生成候选测试计划。')
    } catch (e) { setError(e instanceof Error ? e.message : '测试计划请求失败。') }
    finally { setBusy(false) }
  }
  return <section className="detail-validation">
    <h3>候选测试计划</h3>
    {required && !state && <p>项目验收规则要求通过测试计划核对人工验收。</p>}
    {state && <p>{labels[state] ?? state} · 修订 {latest}</p>}
    <button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起测试计划' : '查看测试计划'}</button>
    {open && <>
      {latest > 1 && <div className="detail-actions"><button className="button" disabled={selected <= 1} onClick={() => setRevision(selected - 1)}>上一版测试计划</button>
        <span>修订 {selected}</span><button className="button" disabled={selected >= latest} onClick={() => setRevision(selected + 1)}>下一版测试计划</button></div>}
      {receipt && <>
        {receipt.candidate_context && <CandidateRuleEvidence context={receipt.candidate_context} title="测试计划使用的候选依据" digest={receipt.basis.candidate_context_sha256}/>}
        <dl>{receipt.basis.criteria.map(c => <div key={c.id}><dt>{c.id}</dt><dd>{c.text || '业务预期待澄清'}</dd></div>)}</dl>
        {receipt.plan && <><p>Test Agent 会话 <code>{receipt.plan.session_id}</code></p>
          <ol>{receipt.plan.cases.map(c => {
            const history = receipt.case_results.filter(r => r.case_id === c.id)
            return <li key={c.id}><strong>{c.id} · {c.objective}</strong><p>{c.criterion_id} · {c.playbook_id} · {c.environment} · {c.tester === 'human' ? '人工' : 'Runner'} · {resultLabels[history.at(-1)?.body.result ?? 'pending']}</p>
            <p>前置条件　{c.preconditions}</p><ol>{c.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>
            <p>预期结果　{c.expected_result}</p><p>待采集证据　{c.evidence.join('；')}</p>
            {receipt.coverage && <CaseEvidence key={`${receipt.revision}:${c.id}`} task={task} execution={execution} revision={receipt.revision} commit={commit} digest={receipt.coverage.plan_sha256}
              caseID={c.id} history={history} editable={current && selected === latest && receipt.state === 'covered' && c.tester === 'human'} request={request} refresh={refresh} />}
            </li>})}</ol>
          {receipt.plan.clarifications.map((q, i) => <p key={i}>待澄清 · {q.criterion_id} · {q.question}</p>)}</>}
        {receipt.coverage && <><h4>Management 覆盖审查</h4><p><code>{receipt.coverage.session_id}</code></p>
          <p>计划 SHA-256　<code>{receipt.coverage.plan_sha256}</code></p>
          <ul>{receipt.coverage.items.map(i => <li key={i.criterion_id}>{i.criterion_id} · {labels[i.state] ?? i.state} · {i.case_ids.join(', ')}<p>{i.rationale}</p></li>)}</ul></>}
      </>}
      {current && selected === latest && <><label>补充验收标准（每行一项）<textarea value={criteria} onChange={e => setCriteria(e.target.value)} /></label>
        <button className="button" disabled={busy} onClick={() => void submit()}>{latest ? '生成修订版测试计划' : '生成测试计划'}</button></>}
      {error && <p role="alert">{error}</p>}
    </>}
  </section>
}
