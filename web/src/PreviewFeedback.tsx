import { useEffect, useState, type FormEvent } from 'react'
import { formatTime, type ApiRequest } from './shared'
import { FeedbackRecordings } from './FeedbackRecordings'
import { FeedbackScreenshots, type FeedbackArtifactReceipt } from './FeedbackScreenshots'

import { RolePermissions, type StructuredProfile } from './RolePermissions'
import { CandidateRuleEvidence, type CandidateContext } from './CandidateRuleEvidence'

type FeedbackContext = {
  candidate: { execution_id: number; base_commit: string | null; commit: string }
  requirement: { revision: number; issue: { title: string; body: string } } | null
  assigned_item: { goal_id: number; plan_revision: number; item: { key: string; objective: string; scope: string; acceptance_criteria: string; deliverables: string } } | null
  rules: CandidateContext | null
}

function ClassificationContext({ context }: { context: FeedbackContext }) {
  return <details><summary>分类使用的候选依据</summary>
    <p>预览执行 #{context.candidate.execution_id} · 候选 <code>{context.candidate.commit}</code></p>
    {context.candidate.base_commit && <p>比较基线 · <code>{context.candidate.base_commit}</code></p>}
    {context.requirement && <><p>需求修订 {context.requirement.revision} · {context.requirement.issue.title}</p><pre className="material-content">{context.requirement.issue.body}</pre></>}
    {context.assigned_item && <><p>Goal #{context.assigned_item.goal_id} · 计划修订 {context.assigned_item.plan_revision} · {context.assigned_item.item.key}</p>
      <p>{context.assigned_item.item.objective}</p><p>范围 · {context.assigned_item.item.scope}</p>
      <p>验收 · {context.assigned_item.item.acceptance_criteria}</p><p>交付物 · {context.assigned_item.item.deliverables}</p></>}
    {context.rules && <CandidateRuleEvidence context={context.rules} title="候选使用的规则依据"/>}
  </details>
}

type Note = { id: number; artifacts: FeedbackArtifactReceipt[]; created_at: string; state: string; work_item?: { id: number; number: number }; requirement?: { revision: number; task_id: number }; review?: { decision: string; note: string }; question?: { id: number; question: string; answer?: string }; classification?: { category: string; rationale: string; evidence: string; session_id: string; profile?: StructuredProfile; context?: FeedbackContext }; body: { path: string; note: string; element?: string; frames?: string[]; canvas_point?: string; browser?: string; width?: number; height?: number; case_id?: string; plan_revision?: number } }
const categories: Record<string, string> = { acceptance: '验收通过', bug: 'Bug', ui_change: 'UI 修改', requirement_change: '需求变更', new_work_item: '新工作项', blocked: '阻塞或澄清' }
function FramePath({ frames }: { frames?: string[] }) {
  return frames?.length ? <div>所属 iframe（从外到内）<ol>{frames.map((frame, i) => <li key={i}><code>{frame}</code></li>)}</ol></div> : null
}
type Page = { repository: string; latest_requirement: number | null; at_acceptance: boolean; preview: { project_id: number; execution_id: number; commit: string; directory: string; environment: string; state: string }; feedback: Note[]; plan: { revision: number; cases: { id: string; objective: string }[] } | null }

export function PreviewFeedback({ task, preview, captureQuery, request }: { task: number; preview: string; captureQuery?: string; request: ApiRequest }) {
  const capture = new URLSearchParams(captureQuery)
  const [page, setPage] = useState<Page | null>(null)
  const [path, setPath] = useState(() => capture.get('path') || '/')
  const [note, setNote] = useState('')
  const [caseID, setCaseID] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [version, setVersion] = useState(0)
  const [asking, setAsking] = useState(0)
  const [question, setQuestion] = useState('')
  const [dispatching, setDispatching] = useState(0)
  const [issueNumber, setIssueNumber] = useState('')
  const [validation, setValidation] = useState('auto')
  const [harness, setHarness] = useState('')
  const [draft, setDraft] = useState<{ feedback: number; revision: number; original: string; body: string } | null>(null)
  const kind = capture.has('recording') ? 'recording' : 'screenshot'
  const mediaName = kind === 'recording' ? '录屏' : '截图'
  const nonce = capture.get(kind) || ''
  const extension = capture.get('extension') || ''
  const [media, setMedia] = useState<Blob | null>(null)
  const [captureFeedback, setCaptureFeedback] = useState(0)
  const [key, setKey] = useState<string>(() => nonce || crypto.randomUUID())
  const api = `/tasks/${task}/previews/${preview}/feedback`
  const mismatch = Boolean(captureQuery && page && (capture.get('commit') !== page.preview.commit || capture.get('execution') !== String(page.preview.execution_id)))
  useEffect(() => {
    const controller = new AbortController()
    request<Page>(api, { signal: controller.signal }).then(result => { if (!controller.signal.aborted) setPage(result) })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [api, request, version])
  const candidate = page?.preview.commit
  useEffect(() => {
    if (!candidate || !nonce || mismatch || media) return
    if (!window.opener || !/^chrome-extension:\/\/[a-p]{32}$/.test(extension) || !/^[a-f0-9-]{36}$/.test(nonce)) {
      setError(mediaName + '来源不可用，请从原扩展标签页重新打开工作台。'); return
    }
    const channel = new MessageChannel()
    const timer = window.setTimeout(() => setError('未收到' + mediaName + '，请从原扩展标签页重新打开工作台。'), 8000)
    channel.port1.onmessage = event => {
      clearTimeout(timer)
      if (!(event.data instanceof Blob) || event.data.type !== (kind === 'recording' ? 'video/webm' : 'image/png') ||
          event.data.size > (kind === 'recording' ? 64 : 16) * 1024 * 1024) {
        setError('扩展传入的' + mediaName + '格式或大小无效。'); return
      }
      setMedia(event.data); setError('')
    }
    window.opener.postMessage({type: 'forgeos-' + kind + '-ready', nonce, route: window.location.hash}, extension, [channel.port2])
    return () => { clearTimeout(timer); channel.port1.close() }
  }, [candidate, extension, kind, media, mismatch, nonce, mediaName])
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!page || busy || mismatch || (nonce && !media)) return
    setBusy(true); setError(''); setMessage('')
    try {
      const result = await request<{ id: number }>(api, { method: 'POST', body: JSON.stringify({ request_id: key,
        execution_id: page.preview.execution_id, commit: page.preview.commit, path: path.trim(), note: note.trim(),
        case_id: caseID, plan_revision: caseID ? page.plan?.revision : 0,
        element: capture.get('element') || '', browser: capture.get('browser') || '',
        frames: capture.getAll('frame'),
        canvas_point: capture.get('canvas_point') || '',
        width: Number(capture.get('width')), height: Number(capture.get('height')) }) })
      if (media && !captureFeedback) setCaptureFeedback(result.id)
      setMessage(`反馈 #${result.id} 已登记。`); setNote(''); setKey(crypto.randomUUID()); setVersion(n => n + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '反馈提交失败。') }
    finally { setBusy(false) }
  }
  async function ask(f: Note) {
    if (!page?.at_acceptance || !f.classification || busy || !question.trim()) return
    setBusy(true); setError(''); setMessage('')
    try {
      await request(`/tasks/${task}/questions`, { method: 'POST', body: JSON.stringify({ feedback_id: f.id,
        execution_id: page.preview.execution_id, commit_sha: page.preview.commit, request_key: 'feedback-' + f.id,
        question: question.trim(), evidence: f.classification.evidence }) })
      setAsking(0); setVersion(n => n + 1); setMessage('澄清问题已进入任务的待回答队列。')
    } catch (e) { setError(e instanceof Error ? e.message : '发起澄清失败。') }
    finally { setBusy(false) }
  }
  async function editRequirement(f: Note) {
    if (!page?.latest_requirement || busy) return
    setBusy(true); setError(''); setMessage('')
    try {
      const v = await request<{ revision: number; issue: { body: string } }>(`/tasks/${task}/requirements/${page.latest_requirement}`)
      setDraft({ feedback: f.id, revision: v.revision, original: v.issue.body, body: v.issue.body })
    } catch (e) { setError(e instanceof Error ? e.message : '无法读取需求正文。') }
    finally { setBusy(false) }
  }
  async function revise() {
    if (!page || !draft || busy) return
    setBusy(true); setError(''); setMessage('')
    try {
      const v = await request<{ revision: number }>(api + '/requirements', { method: 'POST', body: JSON.stringify({
        feedback_id: draft.feedback, execution_id: page.preview.execution_id, commit: page.preview.commit,
        previous_revision: draft.revision, body: draft.body }) })
      setDraft(null); setVersion(n => n + 1); setMessage(`已保存需求修订 ${v.revision}。`)
    } catch (e) { setVersion(n => n + 1); setError(e instanceof Error ? e.message : '无法保存需求修订。') }
    finally { setBusy(false) }
  }
  async function dispatch(e: FormEvent) {
    e.preventDefault()
    if (!page || busy || !dispatching || !/^[1-9]\d*$/.test(issueNumber)) return
    setBusy(true); setError(''); setMessage('')
    try {
      const item = await request<{ id: number }>(`/projects/${page.preview.project_id}/issues/${issueNumber}`, { method: 'POST',
        body: JSON.stringify({ validation, harness, feedback_source: { feedback_id: dispatching, task_id: task, preview,
          execution_id: page.preview.execution_id, commit: page.preview.commit } }) })
      setDispatching(0); setVersion(n => n + 1); setMessage(`已关联工作项 T-${item.id}。`)
    } catch (e) { setError(e instanceof Error ? e.message : '派发失败。') }
    finally { setBusy(false) }
  }
  return <section className="panel"><a href={`#/tasks/${task}`}>返回任务 T-{task}</a><h1>预览反馈</h1>
    {error && <p role="alert">{error}</p>}{message && <p role="status">{message}</p>}
    {page && <><p>执行 {page.preview.execution_id} · {page.preview.environment === 'local-mock' ? '应用预览（local-mock）' : page.preview.directory} · <code>{page.preview.commit}</code></p>
      <form onSubmit={submit}><fieldset><legend>记录观察</legend>
        {mismatch && <p role="alert">捕获页面的候选或执行与此预览不一致，请返回预览重新选择。</p>}
        {captureQuery && <div><p>采集环境　{capture.get('browser')} · {capture.get('width')} × {capture.get('height')}</p>
          <FramePath frames={capture.getAll('frame')}/>{capture.get('element') && <p>所选元素　<code>{capture.get('element')}</code></p>}
          {capture.get('canvas_point') && <p>画布位置（左、上）　{capture.get('canvas_point')}</p>}</div>}
        {nonce && !mismatch && <p role="status">{media ? (captureFeedback ? `${mediaName}已关联反馈 #${captureFeedback}。` : kind === 'recording' ? '已收到录屏。登记观察后可回放并确认上传。' : '已收到截图。登记观察后可遮挡、标注并确认上传。') : `正在从扩展读取${mediaName}…`}</p>}
        <label>页面路径（省略查询参数和片段）<input readOnly={Boolean(nonce)} required maxLength={2048} value={path} onChange={e => setPath(e.target.value)} /></label>
        <label>关联测试用例<select value={caseID} onChange={e => setCaseID(e.target.value)}><option value="">无关联用例</option>
          {page.plan?.cases.map(c => <option key={c.id} value={c.id}>{c.id} · {c.objective}</option>)}</select></label>
        <label>观察结果或修改意见<textarea required maxLength={8192} value={note} onChange={e => setNote(e.target.value)} /></label>
        <button className="button" disabled={busy || mismatch || !note.trim() || Boolean(nonce && !media)}>提交反馈</button>
      </fieldset></form><h2>该预览的反馈</h2>
      <button className="detail-text-button" onClick={() => { setError(''); setVersion(n => n + 1) }}>刷新反馈分类</button>
      {!page.feedback.length && <p>尚无反馈。</p>}
      {page.feedback.map(f => <article key={f.id}><h3>反馈 #{f.id}</h3><p>{f.classification ? 'Management 分类 · ' + categories[f.classification.category] : f.state === 'in_review' ? 'Management 正在分类' : '待 Management 分类'} · {formatTime(f.created_at)} · <code>{f.body.path}</code>
        {f.body.case_id && ` · 计划 v${f.body.plan_revision} · ${f.body.case_id}`}</p><p>{f.body.note}</p>
        <FramePath frames={f.body.frames}/>{f.body.element && <p>元素 <code>{f.body.element}</code></p>}
        {f.body.canvas_point && <p>画布位置（左、上）　{f.body.canvas_point}</p>}
        <FeedbackScreenshots api={`${api}/${f.id}/artifacts`} artifacts={(f.artifacts ?? []).filter(a => a.mime === 'image/png')} request={request} captured={kind === 'screenshot' && captureFeedback === f.id ? media : null}
          saved={() => { setMessage(`已保存反馈 #${f.id} 的截图。`); setVersion(n => n + 1) }}/>
        <FeedbackRecordings api={`${api}/${f.id}/artifacts`} artifacts={(f.artifacts ?? []).filter(a => a.mime === 'video/webm')} request={request} captured={kind === 'recording' && captureFeedback === f.id ? media : null}
          saved={() => { setMessage(`已保存反馈 #${f.id} 的录屏。`); setVersion(n => n + 1) }}/>
        {f.review ? <p>Owner 已{f.review.decision === 'approve' ? '通过验收' : '退回修改'}　{f.review.note}</p> :
          page.at_acceptance && ['acceptance', 'bug', 'ui_change'].includes(f.classification?.category ?? '') &&
          <a href={`#/tasks/${task}`}>在候选验收中引用</a>}
        {f.body.browser && <p>{f.body.browser} · {f.body.width} × {f.body.height}</p>}
        {f.classification && <details><summary>分类依据</summary><p>{f.classification.rationale}</p><blockquote>{f.classification.evidence}</blockquote>
          <p>Management 会话　<code>{f.classification.session_id}</code></p>
          {f.classification.context && <ClassificationContext context={f.classification.context}/>}
          {f.classification.profile && <RolePermissions role="Management" profile={f.classification.profile}/>}</details>}
        {f.work_item ? <a href={`#/tasks/${f.work_item.id}`}>查看独立工作项 T-{f.work_item.id} · Issue #{f.work_item.number}</a> :
          f.classification?.category === 'new_work_item' && (dispatching === f.id ?
            <form onSubmit={dispatch}><fieldset disabled={busy}><legend>按独立 Issue 派发</legend>
              <a href={`https://github.com/${page.repository}/issues/new`} target="_blank" rel="noreferrer">在 {page.repository} 新建 Issue</a>
              <label>独立 Issue 编号<input required inputMode="numeric" pattern="[1-9][0-9]*" value={issueNumber} onChange={e => setIssueNumber(e.target.value)} /></label>
              {/^[1-9]\d*$/.test(issueNumber) && <a href={`https://github.com/${page.repository}/issues/${issueNumber}`} target="_blank" rel="noreferrer">核对 Issue #{issueNumber}</a>}
              <label>执行 Harness<select value={harness} onChange={e => setHarness(e.target.value)}>
                <option value="">项目默认</option><option value="codex">Codex</option><option value="claude">Claude Code</option></select></label>
              <label>验证方式<select value={validation} onChange={e => setValidation(e.target.value)}>
                {['auto','docs','go-test','V1','V2','V3','V4'].map(v => <option key={v}>{v}</option>)}</select></label>
              <button className="button" disabled={!issueNumber}>确认派发</button>
              <button type="button" className="detail-text-button" onClick={() => setDispatching(0)}>取消</button></fieldset></form> :
            <button className="button" disabled={busy} onClick={() => { setDispatching(f.id); setIssueNumber(''); setValidation('auto'); setHarness('') }}>派发独立工作项</button>)}
        {f.requirement ? <a href={`#/tasks/${f.requirement.task_id}`}>到任务查看需求修订 {f.requirement.revision}</a> :
          f.classification?.category === 'requirement_change' && Boolean(page.latest_requirement) && (draft?.feedback === f.id ?
            <fieldset><legend>基于需求修订 {draft.revision} 建立新版本</legend>
              <label>修订后的完整正文<textarea value={draft.body} onChange={e => setDraft({ ...draft, body: e.target.value })} /></label>
              {draft.revision !== page.latest_requirement && <p role="alert">已有更新版本，请取消后重新读取。</p>}
              <button className="button" disabled={busy || draft.revision !== page.latest_requirement || !draft.body.trim() || draft.body === draft.original} onClick={() => void revise()}>保存需求修订</button>
              <button className="detail-text-button" disabled={busy} onClick={() => setDraft(null)}>取消</button></fieldset> :
            <button className="button" disabled={busy} onClick={() => void editRequirement(f)}>从反馈修订需求</button>)}
        {f.question ? <div><p>澄清问题 #{f.question.id} · {f.question.question}</p>
          {f.question.answer ? <p>Owner 答复　{f.question.answer}</p> : <a href={`#/tasks/${task}`}>到任务中回答</a>}</div> :
          page.at_acceptance && f.classification?.category === 'blocked' && (asking === f.id ?
            <fieldset><legend>提交澄清问题</legend><label>需要 Owner 决定的问题<textarea value={question} onChange={e => setQuestion(e.target.value)} /></label>
              <button className="button" disabled={busy || !question.trim()} onClick={() => void ask(f)}>提交澄清问题</button>
              <button className="detail-text-button" disabled={busy} onClick={() => setAsking(0)}>取消</button></fieldset> :
            <button className="button" onClick={() => { setAsking(f.id); setQuestion(f.body.note) }}>转入澄清</button>)}</article>)}
    </>}
  </section>
}
