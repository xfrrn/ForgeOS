import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'
import { useDiscussionModels } from './ProjectChatProvider'
import { credentialsHref } from './workbenchRoutes'
import { EmptyState } from './ui'
import { workTypeName, workTypes } from './workTypes'
import { FeatureNotes, RecordFeatureNote } from './FeatureNotes'
import { FeatureBrief } from './FeatureBrief'
import { FeatureProposals } from './FeatureProposals'
import { ChatMarkdown } from './ChatMarkdown'
import './project-chat.css'

type ChatSource = { key: string; label: string; path: string }
type ChatActivity = ChatSource & { role: string; at: string }
type ChatAction = {
  id: number; kind: 'dispatch_issue' | 'check_head' | 'create_issue'; body?: string; creation_started_at?: string; label: string; repository?: string
  issue_number?: number; work_type?: string; task_id?: number; status: 'pending' | 'confirmed' | 'stale'
}
type IssueDraft = {title: string; body: string}
type ChatAttachment = { id: number; name: string; mime: string; bytes: number }
type ChatTurn = {
  id: number; client_id: string; text: string; created_at: string; model_profile_name?: string; requested_model?: string; requested_harness?: string; native_backend?: string
  cancel_requested?: boolean; session_missing?: boolean; recovery_of?: number
  status: 'queued' | 'running' | 'completed' | 'failed'; answer?: string; error?: string
  sources?: ChatSource[]; action?: ChatAction; activities?: ChatActivity[]; attachments?: ChatAttachment[]
}
type ChatPage = { harness?: 'codex' | 'claude'; enabled: boolean; messages: ChatTurn[]; next_before?: number }
type ChatDraft = { text: string; clientID: string; attachments: ChatAttachment[] }

const blankDraft: ChatDraft = {text: '', clientID: '', attachments: []}
const maxAttachments = 6
function attachmentURL(projectId: number, id: number) {
  return `/projects/${projectId}/chat/attachments/${id}`
}
const issueOptions = [{value: '', label: '请选择工作类型'}, ...Object.entries(workTypes).map(([value, label]) => ({value, label}))]
const quickPrompts = ['查看本项目的任务队列', '有哪些任务待验收？', '列出可查阅的项目资料']
const linkedProjectPages = [
  /^\/tasks(?:\?status=[A-Z_]+|\?tab=queue)?$/,
  /^\/tasks\/[1-9]\d*(?:\/(?:plans\/[1-9]\d*|questions\/[1-9]\d*|previews\/[a-f0-9]{48}\/feedback)|\?execution=[1-9]\d*)?$/,
  /^\/reviews(?:\?tab=(?:approvals|acceptance|code-reviews|checks))?$/,
  /^\/materials(?:\?key=[\w.%+-]+(?:&revision=[1-9]\d*)?)?$/,
  /^\/(?:decisions|shares)(?:\/[1-9]\d*)?$/,
  /^\/(?:questions|inbox)$/,
]
const roleLabels: Record<string, string> = {
  owner: 'Owner', management: 'Management Agent', planner: 'Planner Agent', worker: 'Worker Agent',
  reviewer: 'Reviewer', test: 'Test Agent', 'test-agent': 'Test Agent', curator: 'Knowledge Curator',
  runner: 'Runner', orchestrator: 'Orchestrator', system: 'ForgeOS',
}

function mergeTurns(old: ChatTurn[], incoming: ChatTurn[]) {
  const rows = new Map(old.map(turn => [turn.id, turn]))
  for (const turn of incoming) {
    const previous = rows.get(turn.id)
    // A slow history request can finish after the stream has delivered the final turn.
    if ((previous?.status === 'completed' || previous?.status === 'failed') &&
        (turn.status === 'running' || turn.status === 'queued')) continue
    rows.set(turn.id, turn)
  }
  return [...rows.values()].sort((a, b) => a.id - b.id)
}

function SourceLink({ projectId, source }: { projectId: number; source: ChatSource }) {
  return (source.path === '' || linkedProjectPages.some(page => page.test(source.path)))
    ? <a href={projectHref(projectId, source.path)}>{source.label}</a> : <span>{source.label}</span>
}

function ChatActionCard({ action, enabled, projectId, onConfirm }: {
  action: ChatAction; enabled: boolean; projectId: number; onConfirm: (id: number, workType: string, draft?: IssueDraft) => Promise<void>
}) {
  const creating = action.kind === 'create_issue'
  const [choice, setChoice] = useState(creating ? action.work_type || 'development' : '')
  const [title, setTitle] = useState(action.label)
  const [body, setBody] = useState(action.body || '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const heading = useRef<HTMLDivElement>(null)
  const taskLink = useRef<HTMLAnchorElement>(null)
  const restoreFocus = useRef(false)
  const knownType = workTypeName(action.work_type)
  const chooseType = action.kind === 'dispatch_issue' && !knownType
  const pending = action.status === 'pending'
  const editable = creating && pending && !action.creation_started_at
  const label = creating ? (action.issue_number ? '继续派发' : action.creation_started_at ? '核对创建结果并派发' : '确认创建并派发') :
    action.kind === 'dispatch_issue' ? '确认派发' : '确认检查'
  const canConfirm = enabled && pending && !busy && (!chooseType || !!choice) && (!editable || !!(title.trim() && body.trim() && choice))

  useLayoutEffect(() => {
    if (!pending && restoreFocus.current) {
      restoreFocus.current = false
      const focusTarget = taskLink.current ?? heading.current
      focusTarget?.focus()
    }
  }, [pending])

  async function confirm() {
    if (!canConfirm) return
    setBusy(true)
    setError('')
    try { await onConfirm(action.id, editable || chooseType ? choice : '', editable ? {title: title.trim(), body: body.trim()} : undefined) }
    catch (e) { setError(e instanceof Error ? e.message : '无法确认操作。') }
    finally { setBusy(false) }
  }

  return <section className="chat-action" aria-label={action.label}>
    <div className="chat-action-heading" ref={heading} tabIndex={-1}>
      <Icon name={action.kind === 'dispatch_issue' ? 'ClipboardCheck' : 'GitBranch'} size={17}/>
      <strong>{editable ? 'Issue 草稿' : action.label}</strong>
      <span className={'status ' + (pending ? 'status-waiting_input' : action.status === 'confirmed' ? 'status-done' : '')}>
        <i/>{pending ? '待确认' : action.status === 'confirmed' ? '已确认' : '已失效'}
      </span>
    </div>
    {(action.repository || action.issue_number || knownType) && <p className="chat-action-target">
      {action.repository && <span>{action.repository}</span>}
      {!!action.issue_number && (creating ? <a href={'https://github.com/' + action.repository + '/issues/' + action.issue_number} target="_blank" rel="noreferrer">Issue #{action.issue_number}</a> : <span>Issue #{action.issue_number}</span>)}
      {knownType && !editable && <span>{knownType}</span>}
    </p>}
    {editable && <fieldset className="chat-issue-draft" disabled={busy || !enabled}>
      <label>Issue 标题<input value={title} maxLength={256} onChange={e => setTitle(e.target.value)}/></label>
      <label>Issue 正文<textarea rows={7} value={body} onChange={e => setBody(e.target.value)}/></label>
      <ThemedSelect label="工作类型" value={choice} options={issueOptions.slice(1)} disabled={busy || !enabled} onChange={setChoice}/>
    </fieldset>}
    {creating && !editable && <details><summary>查看 Issue 正文</summary><ChatMarkdown content={action.body || ''}/></details>}
    {creating && action.status === 'stale' && <p>Issue 内容或状态已改变，本次派发已停止。</p>}
    {creating && pending && action.creation_started_at && <p>{action.issue_number ? 'Issue 已创建，等待完成派发。' : '正在核对 GitHub 创建结果。'}</p>}
    {pending && <div className="chat-action-controls">
      {chooseType && <ThemedSelect label="工作类型" value={choice} options={issueOptions}
        disabled={!enabled || busy} onChange={setChoice}/>}
      <button className="button primary" type="button" disabled={!canConfirm} onClick={e => {
        restoreFocus.current = e.currentTarget === document.activeElement
        void confirm()
      }}>
        {busy ? (creating ? '处理中…' : '确认中…') : label}
      </button>
    </div>}
    {error && pending && <p className="detail-problem" role="alert">{error}</p>}
    {!!action.task_id && <a className="chat-task-link" ref={taskLink} href={projectHref(projectId, `/tasks/${action.task_id}`)}>
      查看任务 #{action.task_id}<Icon name="ArrowRight" size={14}/>
    </a>}
  </section>
}

function ChatExchange({ turn, projectId, enabled, onConfirm, onRetry, noteEditor, onOperation }: {
  turn: ChatTurn; projectId: number; enabled: boolean; onConfirm: (id: number, workType: string, draft?: IssueDraft) => Promise<void>
  onOperation?: (id:number, action:"cancel"|"recover")=>Promise<void>
  noteEditor?: React.ReactNode
  onRetry?: (text: string) => void
}) {
  const [working,setWorking]=useState(false)
  const [problem,setProblem]=useState('')
  async function operate(action:'cancel'|'recover'){
    if (!onOperation || working)return
    setWorking(true);setProblem('')
    try{await onOperation(turn.id,action)}catch(e){setProblem(e instanceof Error?e.message:'操作失败，请重试。')}
    finally{setWorking(false)}
  }
  return <li id={'chat-turn-' + turn.id} className="chat-exchange">
    <div className="chat-message chat-owner">
      <div className="chat-message-meta"><strong>你</strong><time dateTime={turn.created_at}>{formatTime(turn.created_at)}</time></div>
      {!!turn.attachments?.length && <div className="chat-owner-attachments">
        {turn.attachments.map(att => <a key={att.id} className="chat-owner-image" href={attachmentURL(projectId, att.id)} target="_blank" rel="noreferrer" title={att.name}>
          <img src={attachmentURL(projectId, att.id)} alt={att.name} loading="lazy"/>
        </a>)}
      </div>}
      {turn.text && <p className="chat-message-text">{turn.text}</p>}
    </div>
    <div className="chat-message chat-agent">
      <div className="chat-message-meta"><span className="chat-agent-icon"><Icon name="MessageSquare" size={15}/></span><strong>{turn.native_backend === "claude" ? "Claude Code" : turn.native_backend === "codex" ? "Codex" : "Management Agent"}</strong>{turn.requested_model && <span title={turn.requested_model}>所选模型 · {turn.requested_model}</span>}</div>
      <div className="chat-message-body">
        {turn.recovery_of && <small>从话题记录恢复 · 原回合 #{turn.recovery_of}</small>}
        {onOperation && (turn.status==='queued' || turn.status==='running') && <button className="button" type="button" disabled={working || turn.cancel_requested} onClick={()=>void operate('cancel')}>{turn.cancel_requested?'等待取消回执':'取消本轮'}</button>}
        {problem && <p role="alert">{problem}</p>}
        {turn.status === 'queued' && <span className="status status-pending"><i/>等待 Agent 领取</span>}
        {turn.status === 'running' && !turn.cancel_requested && <span className="status status-running"><i/>{turn.answer ? '正在回复' : '正在处理'}</span>}
        {turn.status === 'failed' && <div className={turn.cancel_requested?'chat-stopped':'chat-failed'}>
          <p>{turn.error || 'Agent 未能完成这次回复。'}</p>
          {turn.session_missing && onOperation && <button className="button" disabled={working} onClick={()=>void operate('recover')}>从话题记录恢复并重试</button>}
          {onRetry && !turn.session_missing && <button className="button" type="button" onClick={() => onRetry(turn.text)}>重新提问</button>}
        </div>}
        {turn.answer && <ChatMarkdown content={turn.answer} streaming={turn.status === 'running'}/>}
        {!!turn.sources?.length && <section className="chat-evidence" aria-label="回答依据">
          <h2>回答依据</h2>
          <ul>{turn.sources.map(source => <li key={source.key}><SourceLink projectId={projectId} source={source}/></li>)}</ul>
        </section>}
        {turn.action && <ChatActionCard key={turn.action.id} action={turn.action} enabled={enabled} projectId={projectId} onConfirm={onConfirm}/>}
        {!!turn.activities?.length && <section className="chat-evidence" aria-label="任务进展">
          <h2>任务进展</h2>
          <ol className="chat-activities">{turn.activities.map(item => {
            const role = item.role.toLowerCase()
            return <li key={item.key}>
              <div><strong>{Object.hasOwn(roleLabels, role) ? roleLabels[role] : item.role}</strong>
                <time dateTime={item.at}>{formatTime(item.at)}</time></div>
              <SourceLink projectId={projectId} source={item}/>
            </li>
          })}</ol>
        </section>}
      </div>
    </div>
    {noteEditor}
  </li>
}

export function ProjectChat({ projectId, request, feature, showProposals=false, delivery=false, compose=false, pickedModel, onModelChange }: {
  delivery?:boolean; showProposals?:boolean; compose?:boolean; projectId: number; request: ApiRequest; feature?:{id:number;title:string;profile_id:string}
  pickedModel?: {id: string; model?: string; seq: number}; onModelChange?: (id: string, model?: string) => void
}) {
  const chatPath = `/projects/${projectId}/chat`
  const path = feature ? `/projects/${projectId}/features/${feature.id}/messages` : chatPath
  const notesPath = feature ? `/projects/${projectId}/features/${feature.id}/notes` : ''
  const [noteVersion, setNoteVersion] = useState(0)
  const field = useId()
  const input = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const transcript = useRef<HTMLDivElement>(null)
  const followLatest = useRef(true)
  const pinnedTop = useRef(0)
  const [atBottom, setAtBottom] = useState(true)
  const lifetime = useRef<AbortController | null>(null)
  const scrollAnchor = useRef<{id: string; top: number} | null>(null)
  const cursorInitialized = useRef(false)
  const lastFrame = useRef<{id: number} | null>(null)
  const [messages, setMessages] = useState<ChatTurn[]>([])
  const [enabled, setEnabled] = useState<boolean>()
  const [before, setBefore] = useState<number>()
  const [olderBusy, setOlderBusy] = useState(false)
  const [olderError, setOlderError] = useState('')
  const [readError, setReadError] = useState('')
  const [reload, setReload] = useState(0)
  const draftKey = feature ? 'forgeos.feature.draft.' + projectId + '.' + feature.id : ''
  const [draft, setDraft] = useState<ChatDraft>(() => {
    if (!draftKey) return blankDraft
    try {
      const saved = JSON.parse(sessionStorage.getItem(draftKey) || 'null')
      if (typeof saved?.text === 'string' && typeof saved.clientID === 'string') {
        const attachments = Array.isArray(saved.attachments) ? saved.attachments.filter((a: unknown): a is ChatAttachment =>
          !!a && typeof a === 'object' && typeof (a as ChatAttachment).id === 'number' && typeof (a as ChatAttachment).name === 'string') : []
        return { text: saved.text, clientID: saved.clientID, attachments }
      }
    } catch { /* The composer also works without browser storage. */ }
    return blankDraft
  })
  const [uploadBusy, setUploadBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const {models, error: providerError, refresh: refreshModels} = useDiscussionModels(request,feature ? projectId : undefined)
  const selectionKey = 'forgeos.chat.model.' + projectId
  const [selection, setSelection] = useState(() => {
    if (feature) {
      try {return sessionStorage.getItem(draftKey + '.model') || feature.profile_id} catch {return feature.profile_id}
    }
    try {return localStorage.getItem(selectionKey) || ''} catch {return ''}
  })
  const [modelOverride, setModelOverride] = useState(() => {
    if (!feature) return ''
    try {return sessionStorage.getItem(draftKey + '.variant') || ''} catch {return ''}
  })
  useEffect(() => {
    if (!draftKey) return
    try {
      if (draft.text || draft.attachments.length > 0) {
        sessionStorage.setItem(draftKey, JSON.stringify(draft))
      } else {
        sessionStorage.removeItem(draftKey)
      }
      if (selection) sessionStorage.setItem(draftKey + '.model', selection)
      else sessionStorage.removeItem(draftKey + '.model')
      if (modelOverride) sessionStorage.setItem(draftKey + '.variant', modelOverride)
      else sessionStorage.removeItem(draftKey + '.variant')
    } catch { /* An unavailable browser store does not prevent sending. */ }
  }, [draftKey, draft, selection, modelOverride])
  const selected = models?.find(m => m.id === selection)
  const nativeChoices = selected?.output_format === 'native' ? (selected.available_models || []) : []
  const effectiveModel = selected?.output_format === 'native'
    ? (modelOverride && nativeChoices.some(o => o.id === modelOverride) ? modelOverride : selected.model)
    : ''
  const providerReady = !!selected
  const lastPickSeq = useRef(pickedModel?.seq || 0)
  useEffect(() => {
    if (!selection && models?.length) setSelection(models[0].id)
  }, [models, selection])
  useEffect(() => {
    if (pickedModel && pickedModel.seq !== lastPickSeq.current && models?.some(m => m.id === pickedModel.id)) {
      lastPickSeq.current = pickedModel.seq
      setSelection(pickedModel.id)
      setModelOverride(pickedModel.model || '')
      setDraft(old => ({...old, clientID: ''}))
      setSendError('')
    }
  }, [pickedModel, models])
  const onModelChangeRef = useRef(onModelChange)
  onModelChangeRef.current = onModelChange
  useEffect(() => {
    if (selected?.id) onModelChangeRef.current?.(selected.id, effectiveModel || selected.model)
  }, [selected?.id, selected?.model, effectiveModel])
  function chooseModel(id: string) {
    setSelection(id)
    const next = models?.find(m => m.id === id)
    const nextModel = next?.output_format === 'native' ? next.model : ''
    setModelOverride(nextModel)
    onModelChange?.(id, nextModel || next?.model)
    try {if (!feature) localStorage.setItem(selectionKey, id)} catch { /* Selection still applies in this tab. */ }
    setDraft(old => ({...old, clientID: ''}))
    setSendError('')
  }
  function chooseVariant(model: string) {
    setModelOverride(model)
    if (selected?.id) onModelChange?.(selected.id, model)
    setDraft(old => ({...old, clientID: ''}))
    setSendError('')
  }
  const [sendBusy, setSendBusy] = useState(false)
  const [sendError, setSendError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [path, request])

  useEffect(() => {
    const controller = new AbortController()
    let timer: number
    async function read() {
      const frameAtStart = lastFrame.current
      try {
        const page = await request<ChatPage>(path, {signal: controller.signal})
        if (controller.signal.aborted) return
        setEnabled(page.enabled)
        if (!cursorInitialized.current && page.messages?.length) {
          cursorInitialized.current = true
          setBefore(page.next_before)
        }
        // Ignore an older in-flight poll for the turn that just received a stream frame.
        const changed = lastFrame.current !== frameAtStart ? lastFrame.current?.id : undefined
        const incoming = (page.messages ?? []).filter(t => t.id !== changed || t.status === 'completed' || t.status === 'failed')
        setMessages(old => mergeTurns(old, incoming))
        setReadError('')
      } catch (e) {
        if (!controller.signal.aborted) setReadError(e instanceof Error ? e.message : '无法读取项目聊天。')
      } finally {
        if (!controller.signal.aborted) timer = window.setTimeout(read, document.hidden ? 20000 : 4000)
      }
    }
    void read()
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [path, request, reload])

  const activeId = (messages.find(turn => turn.status === 'running') ?? messages.find(turn => turn.status === 'queued'))?.id
  useEffect(() => {
    if (!activeId || typeof EventSource === 'undefined') return
    const events = new EventSource(chatPath + '/turns/' + activeId + '/stream')
    events.onmessage = event => {
      const turn: ChatTurn = JSON.parse(event.data)
      if (turn.id !== activeId) return
      lastFrame.current = {id: turn.id}
      setMessages(old => mergeTurns(old, [turn]))
      if (turn.status === 'completed' || turn.status === 'failed') {
        events.close()
        setReload(value => value + 1)
      }
    }
    return () => events.close()
  }, [chatPath, activeId])

  useEffect(() => {
    if (!sendBusy && draft.clientID && messages.some(turn => turn.client_id === draft.clientID)) {
      setDraft(blankDraft)
      setSendError('')
    }
  }, [messages, draft.clientID, sendBusy])

  const lastTurn = messages.at(-1)
  const scrollSignal = `${messages.length}:${lastTurn?.id ?? 0}:${lastTurn?.answer?.length ?? 0}:${lastTurn?.status ?? ''}:${messages[0]?.id ?? 0}`
  useLayoutEffect(() => {
    const view = transcript.current
    if (!view) return
    const anchor = scrollAnchor.current
    if (anchor) {
      scrollAnchor.current = null
      const turn = document.getElementById(anchor.id)
      if (turn) view.scrollTop += turn.getBoundingClientRect().top - anchor.top
    } else if (followLatest.current && view.scrollHeight - view.clientHeight - view.scrollTop > 1) {
      // Input can move scrollTop before the browser dispatches its scroll event.
      if (view.scrollTop < pinnedTop.current - 2) {
        followLatest.current = false
        setAtBottom(false)
      } else {
        view.scrollTop = view.scrollHeight
        pinnedTop.current = view.scrollTop
      }
    }
  }, [scrollSignal])

  useEffect(() => {
    const refreshVisible = () => { if (!document.hidden) setReload(value => value + 1) }
    document.addEventListener('visibilitychange', refreshVisible)
    return () => document.removeEventListener('visibilitychange', refreshVisible)
  }, [])

  const refresh = () => setReload(value => value + 1)

  async function loadEarlier() {
    const signal = lifetime.current?.signal
    if (!signal || !before || olderBusy) return
    setOlderBusy(true)
    setOlderError('')
    try {
      const page = await request<ChatPage>(`${path}?before=${before}`, {signal})
      if (signal.aborted) return
      const first = document.getElementById('chat-turn-' + messages[0]?.id)
      if (first) scrollAnchor.current = {id: first.id, top: first.getBoundingClientRect().top}
      setMessages(old => mergeTurns(old, page.messages ?? []))
      setBefore(page.next_before)
    } catch (e) {
      if (!signal.aborted) setOlderError(e instanceof Error ? e.message : '无法读取更早消息。')
    } finally {
      if (!signal.aborted) setOlderBusy(false)
    }
  }

  function editDraft(text: string) {
    setDraft(old => ({...old, text, clientID: text.trim() === old.text.trim() ? old.clientID : ''}))
    setSendError('')
  }

  function focusComposer(text: string) {
    editDraft(text)
    input.current?.focus({preventScroll: true})
  }

  async function uploadImages(files: File[]) {
    const signal = lifetime.current?.signal
    if (!signal || !files.length || sendBusy) return
    const images = files.filter(f => f.type.startsWith('image/'))
    if (!images.length) {
      setSendError('仅支持上传或粘贴 PNG、JPEG、WebP 或 GIF 图片。')
      return
    }
    const available = maxAttachments - draft.attachments.length
    if (available <= 0) {
      setSendError(`单条消息最多附带 ${maxAttachments} 张图片。`)
      return
    }
    const batch = images.slice(0, available)
    setUploadBusy(true)
    setSendError(images.length > available ? `单条消息最多附带 ${maxAttachments} 张图片，已保留前 ${available} 张。` : '')
    try {
      const uploaded: ChatAttachment[] = []
      for (const file of batch) {
        const form = new FormData()
        form.append('file', file, file.name || 'screenshot.png')
        const item = await request<ChatAttachment>(`${chatPath}/attachments`, {method: 'POST', body: form, signal})
        if (signal.aborted) return
        uploaded.push(item)
      }
      if (uploaded.length) {
        setDraft(old => ({...old, clientID: '', attachments: [...old.attachments, ...uploaded]}))
      }
    } catch (e) {
      if (!signal.aborted) setSendError(e instanceof Error ? e.message : '上传图片失败，请重试。')
    } finally {
      if (!signal.aborted) setUploadBusy(false)
    }
  }

  async function removeAttachment(id: number) {
    if (sendBusy) return
    setDraft(old => ({...old, clientID: '', attachments: old.attachments.filter(a => a.id !== id)}))
    setSendError('')
    try {
      await request(`${chatPath}/attachments/${id}`, {method: 'DELETE'})
    } catch { /* Best-effort cleanup of unattached draft image. */ }
  }

  async function send() {
    const signal = lifetime.current?.signal
    const text = draft.text.trim()
    const attachmentIDs = draft.attachments.map(a => a.id)
    if (!signal || !enabled || !providerReady || (!text && attachmentIDs.length === 0) || sendBusy || uploadBusy) return
    if (input.current?.form?.contains(document.activeElement)) input.current.focus({preventScroll: true})
    const clientID = draft.clientID || crypto.randomUUID()
    setDraft(old => ({...old, clientID}))
    setSendBusy(true)
    setSendError('')
    try {
      const turn = await request<ChatTurn>(path, {
        method: 'POST',
        body: JSON.stringify({
          client_id: clientID,
          text,
          profile_id: selection,
          model: effectiveModel || undefined,
          attachment_ids: attachmentIDs.length ? attachmentIDs : undefined,
        }),
        signal,
      })
      if (signal.aborted) return
      setMessages(old => mergeTurns(old, [turn]))
      setDraft(blankDraft)
    } catch (e) {
      if (!signal.aborted) {setSendError(e instanceof Error ? e.message : '无法发送消息。'); refreshModels()}
    } finally {
      if (!signal.aborted) { setSendBusy(false); refresh() }
    }
  }

  async function operateTurn(id:number,action:'cancel'|'recover') {
    const result=await request<ChatTurn | undefined>(path+'/'+id+'/'+action,{method:'POST',signal:lifetime.current?.signal})
    if (result) setMessages(old=>mergeTurns(old,[result]))
    refresh()
  }
  async function confirm(actionId: number, workType: string, draft?: IssueDraft) {
    const signal = lifetime.current?.signal
    try {
      const turn = await request<ChatTurn>(`${path}/actions/${actionId}/confirm`, {
        method: 'POST', body: JSON.stringify({...draft, work_type: workType || undefined}), signal,
      })
      if (!signal?.aborted) setMessages(old => mergeTurns(old, [turn]))
    } finally {
      if (!signal?.aborted) refresh()
    }
  }

  const ready = enabled !== undefined
  const completedCount = messages.filter(t => t.status === 'completed').length
  const briefVersion = noteVersion * 10000 + completedCount
  return <div className="project-chat">
    <header className="chat-studio-header">
      <div>
        <h1>{feature?.title || '项目聊天'}</h1>
        <p>{feature ? '功能话题 · 跨模型持续讨论并保留上下文' : '向 Management Agent 查询当前项目，或发起需要确认的操作'}</p>
      </div>
      <a className="button" href={projectHref(projectId, '/agents')}><Icon name="Bot" size={14}/>Agent 配置</a>
    </header>
    {feature && <nav className="proposal-tabs" aria-label="话题视图">
      <button className="button" aria-pressed={!showProposals && !delivery} onClick={() => {window.location.hash=projectHref(projectId,'/features/'+feature.id)}}>讨论</button>
      <button className="button" aria-pressed={showProposals} onClick={() => {window.location.hash=projectHref(projectId,'/features/'+feature.id+'?view=proposal')}}>方案与拍板</button>
      <button className="button" aria-pressed={delivery} onClick={()=>{window.location.hash=projectHref(projectId,'/features/'+feature.id+'?view=delivery')}}>设计与交付</button>
    </nav>}
    {feature && (showProposals || delivery) ? <FeatureProposals key={delivery?"delivery":"proposals"} delivery={delivery} compose={compose} project={projectId} feature={feature.id} profile={selected?.id || feature.profile_id} preferredModel={effectiveModel || selected?.model} request={request} models={models || []}/> : <>
    {feature && <FeatureNotes path={notesPath} request={request} version={noteVersion} profileId={selected?.id} model={effectiveModel || selected?.model} enabled={!!enabled && providerReady} hasTurns={completedCount > 0} onChanged={() => setNoteVersion(n => n + 1)}/>}
    {feature && <FeatureBrief projectId={projectId} featureId={feature.id} request={request} version={briefVersion} profileId={selected?.id} model={effectiveModel || selected?.model} enabled={!!enabled && providerReady} hasTurns={completedCount > 0}/>}
    <div className="panel project-chat-panel">
      <div ref={transcript} className="chat-scroll" role="region" aria-label="对话历史" tabIndex={0}
        onWheel={event => { if (event.deltaY < 0) followLatest.current = false }}
        onScroll={event => {
          const view = event.currentTarget
          const bottom = view.scrollHeight - view.clientHeight - view.scrollTop <= 8
          if (bottom) pinnedTop.current = view.scrollTop
          followLatest.current = bottom
          setAtBottom(bottom)
        }}>
      <div className="project-chat-content">
        {!ready && !readError && <p className="muted" role="status">正在读取聊天…</p>}
        {readError && <div className="chat-read-problem" role="alert">
          <p>{readError}</p><button className="button" type="button" onClick={refresh}>重新读取</button>
        </div>}
        {ready && !enabled && <p className="chat-agent-unavailable" role="status"><Icon name="AlertCircle" size={17}/>此项目暂未启用聊天 Agent。</p>}
        {ready && !messages.length && <EmptyState icon="Sparkles" title={feature ? '开始讨论这个功能' : '有什么我可以协助你的？'}
          description={enabled ? '从当前项目的任务、架构、验收或资料问起。' : undefined}>
          {!feature && enabled && providerReady && <div className="chat-prompts">{quickPrompts.map(prompt => <button className="button" key={prompt} type="button"
            disabled={sendBusy || !!draft.text.trim()} onClick={() => focusComposer(prompt)}>{prompt}</button>)}</div>}
        </EmptyState>}
        {before && <div className="chat-earlier">
          <button className="button" type="button" disabled={olderBusy} onClick={() => void loadEarlier()}>
            <Icon name="History" size={15}/>{olderBusy ? '读取中…' : '加载更早消息'}
          </button>
          {olderError && <p className="detail-problem" role="alert">{olderError}</p>}
        </div>}
        {!!messages.length && <div role="log" aria-label="项目聊天记录" aria-relevant="additions text">
          <ol className="chat-transcript">{messages.map(turn => <ChatExchange key={turn.id} turn={turn} projectId={projectId}
            onOperation={feature ? operateTurn : undefined}
            noteEditor={feature && turn.status === 'completed' && <RecordFeatureNote path={notesPath} request={request} turn={turn.id} profileId={selected?.id} model={effectiveModel || selected?.model} enabled={!!enabled && providerReady} onSaved={()=>setNoteVersion(n=>n+1)}/>}
            enabled={!!enabled} onConfirm={confirm} onRetry={enabled && providerReady && !sendBusy && !draft.text.trim() ? focusComposer : undefined}/>)}</ol>
        </div>}
      </div>
      </div>
      <section className="chat-latest" aria-label="最新消息及输入">
        <div className="chat-footer-bar">
          {enabled && providerError && <span role="alert">{providerError} <button className="button" onClick={refreshModels}>重新读取</button></span>}
          {enabled && models?.length === 0 && <span>尚未添加聊天模型。<a href={credentialsHref(projectId)}>前往密钥与凭据</a></span>}
        </div>
        {messages.length > 0 && !atBottom && <button className="button chat-jump" onClick={() => {
            const view = transcript.current
            if (view) { followLatest.current = true; view.scrollTop = view.scrollHeight; view.focus({preventScroll: true}) }
          }}><Icon name="ChevronDown" size={15}/>最新消息</button>}
        {enabled && !!models?.length && <form className={'chat-composer' + (dragging ? ' chat-composer-drag' : '')}
          onSubmit={e => { e.preventDefault(); void send() }}
          onDragOver={e => { if (e.dataTransfer.types.includes('Files')) { e.preventDefault(); setDragging(true) } }}
          onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false) }}
          onDrop={e => {
            e.preventDefault()
            setDragging(false)
            const dropped = Array.from(e.dataTransfer.files || [])
            if (dropped.length) void uploadImages(dropped)
          }}>
          {!!draft.attachments.length && <div className="chat-draft-attachments" aria-label="已添加图片">
            {draft.attachments.map(att => <figure key={att.id} className="chat-draft-thumb">
              <img src={attachmentURL(projectId, att.id)} alt={att.name}/>
              <button type="button" className="chat-draft-remove" aria-label={`移除图片 ${att.name}`} disabled={sendBusy} onClick={() => void removeAttachment(att.id)}>
                <Icon name="X" size={12}/>
              </button>
            </figure>)}
          </div>}
          <label className="sr-only" htmlFor={field}>{feature ? '讨论内容' : '发给 Management Agent'}</label>
          <textarea id={field} ref={input} rows={2} maxLength={4000} aria-describedby={sendError ? `${field}-error` : undefined}
            placeholder={feature ? '输入你的想法、追问或方案要求，支持粘贴或拖入截图…（Ctrl+Enter 发送）' : '向 Management Agent 提问或下达指令，支持粘贴截图…（Ctrl+Enter 发送）'}
            value={draft.text} readOnly={sendBusy} onChange={e => editDraft(e.currentTarget.value)}
            onPaste={e => {
              const files = Array.from(e.clipboardData?.items || [])
                .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
                .map(item => item.getAsFile())
                .filter((f): f is File => !!f)
              if (files.length) {
                if (!e.clipboardData?.getData('text')) e.preventDefault()
                void uploadImages(files)
              }
            }}
            onKeyDown={e => {
              if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.nativeEvent.isComposing) { e.preventDefault(); void send() }
            }}/>
          <input ref={fileInput} type="file" accept="image/png,image/jpeg,image/webp,image/gif" multiple hidden
            onChange={e => {
              const picked = Array.from(e.currentTarget.files || [])
              e.currentTarget.value = ''
              if (picked.length) void uploadImages(picked)
            }}/>
          <div className="chat-composer-bottom">
            <div className="chat-composer-tools">
              <ThemedSelect label="讨论客户端" hideLabel value={selected?.id || ''} disabled={sendBusy}
                options={[...(!selected ? [{value: '', label: '选择模型'}] : []), ...models.map(m => ({
                  value: m.id,
                  label: m.output_format==='native' ? (m.available_models && m.available_models.length > 1 ? (m.backend==='claude' ? 'Claude Code' : 'Codex') : m.name) : m.model,
                  group: models.some(other => other.id !== m.id && other.model === m.model) ? m.base_url : undefined,
                }))]} onChange={chooseModel}/>
              {nativeChoices.length > 1 && <ThemedSelect label="模型版本" hideLabel value={effectiveModel} disabled={sendBusy}
                options={nativeChoices.map(o => ({value: o.id, label: o.label || o.id}))} onChange={chooseVariant}/>}
              <button type="button" className="button chat-attach-btn" disabled={sendBusy || uploadBusy || draft.attachments.length >= maxAttachments}
                onClick={() => fileInput.current?.click()} title="上传图片（支持 Ctrl+V 粘贴截图）">
                <Icon name="ImagePlus" size={14}/>{uploadBusy ? '上传中…' : '图片'}
              </button>
            </div>
            <button className="button primary chat-send-btn" type="submit" disabled={!providerReady || (!draft.text.trim() && draft.attachments.length === 0) || sendBusy || uploadBusy} aria-keyshortcuts="Control+Enter Meta+Enter">
              {sendBusy ? '发送中…' : sendError ? '重试' : '发送'}<Icon name="ArrowRight" size={14}/>
            </button>
          </div>
          {sendError && <p className="detail-problem" id={`${field}-error`} role="alert">{sendError}</p>}
        </form>}
      </section>
    </div></>}
  </div>
}
