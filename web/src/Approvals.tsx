import { useEffect, useState } from 'react'
import { formatTime, type ApiRequest } from './shared'

export const approvalLabels = { plan: 'Goal 计划', decision: '项目决策', share: '跨项目共享', question: '业务决定', command: '原生命令', patch: '文件补丁' }
const verdicts = { approve: '建议通过', needs_owner: '请 Owner 决定', revise: '建议修订' }
type Kind = keyof typeof approvalLabels
export type PatchFile = { path: string; op: 'add'|'update'|'delete'; diff: string; move_to?: string; before_sha256?: string }
export function PatchPreview({files}: {files: PatchFile[]}) {
  return <>{files.map(f => <div key={f.path}>
    <strong>{{add:'新增', update:'修改', delete:'删除'}[f.op]} <code>{f.path}</code>{f.move_to && <> → <code>{f.move_to}</code></>}</strong>
    {f.before_sha256 && <p>修改前 SHA-256 · <code>{f.before_sha256}</code></p>}
    <pre className="material-content">{f.diff || '（空）'}</pre>
  </div>)}</>
}
export type ApprovalItem = { kind: Kind; id: number; project_id: number; title: string; task_id?: number; revision?: number; verdict?: keyof typeof verdicts; level?: string; recipient?: number; digest?: string; valid_until?: string; execution_id?: number; instance?: number; session_id?: string; turn_id?: string; item_id?: string; cwd?: string; base_commit?: string; patch?: PatchFile[] }
type Page = { items: ApprovalItem[]; next_before?: string }

export function approvalPath(item: ApprovalItem) {
  if (item.kind === 'command' || item.kind === 'patch') return '#/approvals'
  if (item.kind === 'question') return `#/tasks/${item.task_id}/questions/${item.id}`
  if (item.kind === 'plan') return `#/tasks/${item.id}/plans/${item.revision}`
  return `#/projects/${item.project_id}/${item.kind === 'decision' ? 'decisions' : 'shares'}/${item.id}`
}

export function Approvals({request}: {request: ApiRequest}) {
  const [kind, setKind] = useState('')
  const [cursors, setCursors] = useState<string[]>([])
  const [page, setPage] = useState<Page>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [busy, setBusy] = useState(false)
  const [receipt, setReceipt] = useState('')
  async function decide(item: ApprovalItem, decision: string, label: string) {
    setBusy(true); setError('')
    try {
      await request('/native-approvals/' + item.id, {method: 'POST', body: JSON.stringify({decision, digest:item.digest})})
      setReceipt(approvalLabels[item.kind] + '请求 #' + item.id + ' 已记录「' + label + '」。')
      setRevision(n => n + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '提交决定失败。') }
    finally { setBusy(false) }
  }
  const before = cursors.at(-1)
  useEffect(() => {
    const controller = new AbortController()
    const query = new URLSearchParams({include_questions: '1', include_commands: '1', include_patches: '1'})
    if (kind) query.set('kind', kind)
    if (before) query.set('before', before)
    setPage(undefined); setError('')
    request<Page>('/tasks/approvals?' + query, {signal: controller.signal})
      .then(p => { if (!controller.signal.aborted) setPage(p) })
      .catch(e => { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '读取待审批记录失败。') })
    return () => controller.abort()
  }, [request, kind, before, revision])
  return <section className="panel detail-card"><h1>审批中心</h1>
    <div className="detail-actions">
      <label>操作　<select value={kind} onChange={e => { setKind(e.target.value); setCursors([]) }}>
        <option value="">全部操作</option>{Object.entries(approvalLabels).map(([k,label]) => <option key={k} value={k}>{label}</option>)}
      </select></label>
      <button className="button" onClick={() => setRevision(n => n + 1)}>刷新审批列表</button>
    </div>
    {error && <p role="alert">{error}</p>}
    {receipt && <p role="status">{receipt}</p>}
    {!page && !error && <p role="status">正在读取待审批记录…</p>}
    {page?.items.length === 0 && <p>本页没有待审批记录。</p>}
    {page?.items.map(item => <article className="notification-row" key={item.kind + item.id}>
      <h2>{approvalLabels[item.kind]} · {item.kind === 'command' ? <code style={{whiteSpace:'pre-wrap', overflowWrap:'anywhere'}}>{item.title}</code> : item.title}</h2>
      <p>项目 #{item.project_id} · {item.kind === 'plan' ? '任务 T-' : '记录 #'}{item.id}
        {item.level && item.kind !== 'decision' && ' · ' + item.level}{item.revision && ' · 修订 ' + item.revision}
        {item.recipient && ' · 接收项目 #' + item.recipient}</p>
      {item.verdict && <p>{item.kind === 'decision' ? '来源知识审核' + (item.level ? ' · ' + item.level : '') : 'Management 审核'} · {verdicts[item.verdict]}</p>}
      {item.digest && <p>{item.kind === 'patch' ? '补丁' : '副本'} SHA-256 · <code>{item.digest}</code></p>}
      {item.patch && <PatchPreview files={item.patch}/>}
      {item.valid_until && <p>有效期至 <time dateTime={item.valid_until}>{formatTime(item.valid_until)}</time></p>}
      {item.kind === 'command' || item.kind === 'patch' ? <>
        <p><a href={'#/tasks/' + item.task_id}>任务 T-{item.task_id}</a> · 执行 #{item.execution_id} · 进程 #{item.instance}</p>
        <p>目录 <code>{item.cwd}</code> · 基线 <code>{item.base_commit}</code></p>
        <p>会话 <code>{item.session_id}</code> · 回合 <code>{item.turn_id}</code> · 调用 <code>{item.item_id}</code></p>
        <div className="detail-actions">{Object.entries({accept:'仅允许本次', decline:'拒绝本次', cancel:'取消回合'}).map(([decision,label]) =>
          <button key={decision} className="button" disabled={busy} onClick={() => decide(item,decision,label)}>{label}</button>)}</div>
      </> : <a href={approvalPath(item)}>查看{approvalLabels[item.kind]}</a>}
    </article>)}
    <div className="detail-actions">
      {before && <button className="button" onClick={() => setCursors(c => c.slice(0,-1))}>上一页</button>}
      {page?.next_before && <button className="button" onClick={() => setCursors(c => [...c,page.next_before!])}>下一页</button>}
    </div>
  </section>
}
