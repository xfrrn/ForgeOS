import { useState } from 'react'
import { formatTime, type ApiRequest } from './shared'

export type CaseAttempt = { case_id: string; attempt: number; actor: string; tester: string; recorded_at: string; body: { operations: string[]; actual_result: string; evidence: string[]; result: string } }
export const resultLabels: Record<string, string> = { pending: '待执行', passed: '已通过', blocked: '受阻', failed: '未通过' }

export function CaseEvidence({ task, execution, revision, commit, digest, caseID, history, editable, request, refresh }: {
  task: number; execution: number; revision: number; commit: string; digest: string; caseID: string; history: CaseAttempt[]; editable: boolean
  request: ApiRequest; refresh: (message: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [operations, setOperations] = useState('')
  const [actual, setActual] = useState('')
  const [evidence, setEvidence] = useState('')
  const [result, setResult] = useState('passed')
  const [error, setError] = useState('')
  async function save() {
    setBusy(true)
    setError('')
    try {
      const lines = (s: string) => s.split('\n').map(v => v.trim()).filter(Boolean)
      await request(`/tasks/${task}/test-results`, { method: 'POST', body: JSON.stringify({ execution_id: execution, revision, commit,
        plan_sha256: digest, case_id: caseID, previous_attempt: history.length, operations: lines(operations), actual_result: actual.trim(), evidence: lines(evidence), result }) })
      setOpen(false)
      setOperations('')
      setActual('')
      setEvidence('')
      refresh('已记录本次人工实测。')
    } catch (e) { setError(e instanceof Error ? e.message : '结果写入失败。') }
    finally { setBusy(false) }
  }
  return <>{history.map(a => <details key={a.attempt}><summary>第 {a.attempt} 次实测 · {resultLabels[a.body.result]} · {a.actor}</summary>
    <p>{formatTime(a.recorded_at)}</p><ol>{a.body.operations.map((s, i) => <li key={i}>{s}</li>)}</ol>
    <p>实际结果　{a.body.actual_result}</p><ul>{a.body.evidence.map((s, i) => <li key={i}>{s}</li>)}</ul>
  </details>)}
    {editable && <><button className="button" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起记录表' : '记录人工实测'}</button>
      {open && <fieldset><legend>{caseID} 实测记录</legend>
        <label>实际操作（每行一步）<textarea value={operations} onChange={e => setOperations(e.target.value)} /></label>
        <label>观察到的结果<textarea value={actual} onChange={e => setActual(e.target.value)} /></label>
        <label>证据（每行一项）<textarea value={evidence} onChange={e => setEvidence(e.target.value)} /></label>
        <label>实测结论<select value={result} onChange={e => setResult(e.target.value)}><option value="passed">通过</option><option value="failed">未通过</option><option value="blocked">受阻</option></select></label>
        <button className="button" disabled={busy || !operations.trim() || !actual.trim() || !evidence.trim()} onClick={() => void save()}>保存实测结果</button>
        {error && <p role="alert">{error}</p>}
      </fieldset>}</>}
  </>
}
