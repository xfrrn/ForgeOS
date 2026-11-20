import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { formatTime, Icon, projectHref, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'
import { ModelSelect } from './ModelSelect'
import { EmptyState, PageHeading } from './ui'
import { workTypeName, workTypes } from './workTypes'
import './project-chat.css'

type ChatSource = { key: string; label: string; path: string }
type ChatActivity = ChatSource & { role: string; at: string }
type ChatAction = {
  id: number; kind: 'dispatch_issue' | 'check_head'; label: string; repository?: string
  issue_number?: number; work_type?: string; task_id?: number; status: 'pending' | 'confirmed' | 'stale'
}
type ChatTurn = {
  id: number; client_id: string; text: string; created_at: string; requested_model?: string; requested_harness?: string
  status: 'queued' | 'running' | 'completed' | 'failed'; answer?: string; error?: string
  sources?: ChatSource[]; action?: ChatAction; activities?: ChatActivity[]
}
type ChatPage = { harness?: 'codex' | 'claude'; enabled: boolean; messages: ChatTurn[]; next_before?: number }
type ChatDraft = { text: string; clientID: string }

const blankDraft: ChatDraft = {text: '', clientID: ''}
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
  for (const turn of incoming) rows.set(turn.id, turn)
  return [...rows.values()].sort((a, b) => a.id - b.id)
}

function SourceLink({ projectId, source }: { projectId: number; source: ChatSource }) {
  return (source.path === '' || linkedProjectPages.some(page => page.test(source.path)))
    ? <a href={projectHref(projectId, source.path)}>{source.label}</a> : <span>{source.label}</span>
}

function ChatActionCard({ action, enabled, projectId, onConfirm }: {
  action: ChatAction; enabled: boolean; projectId: number; onConfirm: (id: number, workType: string) => Promise<void>
}) {
  const [choice, setChoice] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const heading = useRef<HTMLDivElement>(null)
  const taskLink = useRef<HTMLAnchorElement>(null)
  const restoreFocus = useRef(false)
  const knownType = workTypeName(action.work_type)
  const chooseType = action.kind === 'dispatch_issue' && !knownType
  const pending = action.status === 'pending'
  const label = action.kind === 'dispatch_issue' ? '确认派发' : '确认检查'

  useLayoutEffect(() => {
    if (!pending && restoreFocus.current) {
      restoreFocus.current = false
      const focusTarget = taskLink.current ?? heading.current
      focusTarget?.focus()
    }
  }, [pending])

  async function confirm() {
    if (!enabled || !pending || busy || (chooseType && !choice)) return
    setBusy(true)
    setError('')
    try { await onConfirm(action.id, chooseType ? choice : '') }
    catch (e) { setError(e instanceof Error ? e.message : '无法确认操作。') }
    finally { setBusy(false) }
  }

  return <section className="chat-action" aria-label={action.label}>
    <div className="chat-action-heading" ref={heading} tabIndex={-1}>
      <Icon name={action.kind === 'dispatch_issue' ? 'ClipboardCheck' : 'GitBranch'} size={17}/>
      <strong>{action.label}</strong>
      <span className={'status ' + (pending ? 'status-waiting_input' : action.status === 'confirmed' ? 'status-done' : '')}>
        <i/>{pending ? '待确认' : action.status === 'confirmed' ? '已确认' : '已失效'}
      </span>
    </div>
    {(action.repository || action.issue_number || knownType) && <p className="chat-action-target">
      {action.repository && <span>{action.repository}</span>}
      {!!action.issue_number && <span>Issue #{action.issue_number}</span>}
      {knownType && <span>{knownType}</span>}
    </p>}
    {pending && <div className="chat-action-controls">
      {chooseType && <ThemedSelect label="工作类型" value={choice} options={issueOptions}
        disabled={!enabled || busy} onChange={setChoice}/>}
      <button className="button primary" type="button" disabled={!enabled || busy || (chooseType && !choice)} onClick={e => {
        restoreFocus.current = e.currentTarget === document.activeElement
        void confirm()
      }}>
        {busy ? '确认中…' : label}
      </button>
    </div>}
    {error && pending && <p className="detail-problem" role="alert">{error}</p>}
    {!!action.task_id && <a className="chat-task-link" ref={taskLink} href={projectHref(projectId, `/tasks/${action.task_id}`)}>
      查看任务 #{action.task_id}<Icon name="ArrowRight" size={14}/>
    </a>}
  </section>
}

function ChatExchange({ turn, projectId, enabled, onConfirm, onRetry }: {
  turn: ChatTurn; projectId: number; enabled: boolean; onConfirm: (id: number, workType: string) => Promise<void>
  onRetry?: (text: string, model?: string, harness?:string) => void
}) {
  return <li id={'chat-turn-' + turn.id} className="chat-exchange">
    <div className="chat-message chat-owner">
      <div className="chat-message-meta"><strong>你</strong><time dateTime={turn.created_at}>{formatTime(turn.created_at)}</time></div>
      <p className="chat-message-text">{turn.text}</p>
    </div>
    <div className="chat-message chat-agent">
      <div className="chat-message-meta"><span className="chat-agent-icon"><Icon name="MessageSquare" size={15}/></span><strong>Management Agent</strong>{turn.requested_model && <span>所选模型 · {turn.requested_model}</span>}</div>
      <div className="chat-message-body">
        {turn.status === 'queued' && <span className="status status-pending"><i/>等待 Agent 领取</span>}
        {turn.status === 'running' && <span className="status status-running"><i/>正在处理</span>}
        {turn.status === 'failed' && <div className="chat-failed">
          <p>{turn.error || 'Agent 未能完成这次回复。'}</p>
          {onRetry && <button className="button" type="button" onClick={() => onRetry(turn.text,turn.requested_model ?? '',turn.requested_harness)}>重新提问</button>}
        </div>}
        {turn.answer && <p className="chat-message-text">{turn.answer}</p>}
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
  </li>
}

export function ProjectChat({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const path = `/projects/${projectId}/chat`
  const field = useId()
  const input = useRef<HTMLTextAreaElement>(null)
  const navigation = useRef<HTMLElement>(null)
  const lifetime = useRef<AbortController | null>(null)
  const scrollAnchor = useRef<{id: string; top: number} | null>(null)
  const cursorInitialized = useRef(false)
  const [messages, setMessages] = useState<ChatTurn[]>([])
  const [enabled, setEnabled] = useState<boolean>()
  const [before, setBefore] = useState<number>()
  const [olderBusy, setOlderBusy] = useState(false)
  const [olderError, setOlderError] = useState('')
  const [readError, setReadError] = useState('')
  const [reload, setReload] = useState(0)
  const [draft, setDraft] = useState(blankDraft)
  const [harness, setHarness] = useState<'codex' | 'claude'>('codex')
  const [model, setModel] = useState('')
  useEffect(() => { setModel('') }, [harness])
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
      try {
        const page = await request<ChatPage>(path, {signal: controller.signal})
        if (controller.signal.aborted) return
        setEnabled(page.enabled)
        setHarness(page.harness ?? 'codex')
        if (!cursorInitialized.current && page.messages?.length) {
          cursorInitialized.current = true
          setBefore(page.next_before)
        }
        setMessages(old => mergeTurns(old, page.messages ?? []))
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

  useEffect(() => {
    if (!sendBusy && draft.clientID && messages.some(turn => turn.client_id === draft.clientID)) {
      setDraft(blankDraft)
      setModel('')
      setSendError('')
    }
  }, [messages, draft.clientID, sendBusy])

  useLayoutEffect(() => {
    const anchor = scrollAnchor.current
    if (!anchor) return
    scrollAnchor.current = null
    const turn = document.getElementById(anchor.id)
    if (turn) window.scrollBy(0, turn.getBoundingClientRect().top - anchor.top)
  }, [messages])

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
    setDraft(old => ({text, clientID: text.trim() === old.text.trim() ? old.clientID : ''}))
    setSendError('')
  }

  function focusComposer(text: string, previousModel?: string, previousHarness?:string) {
    if(previousModel!==undefined)setModel(previousHarness && previousHarness!==harness ? '' : previousModel)
    editDraft(text)
    input.current?.focus()
  }

  async function send() {
    const signal = lifetime.current?.signal
    const text = draft.text.trim()
    if (!signal || !enabled || !text || sendBusy) return
    if(input.current?.form && !input.current.form.reportValidity())return
    const clientID = draft.clientID || crypto.randomUUID()
    setDraft({text: draft.text, clientID})
    setSendBusy(true)
    setSendError('')
    try {
      const turn = await request<ChatTurn>(path, {method: 'POST', body: JSON.stringify({client_id: clientID, text, model, harness:model ? harness : undefined}), signal})
      if (signal.aborted) return
      setMessages(old => mergeTurns(old, [turn]))
      setModel('')
      setDraft(blankDraft)
      window.requestAnimationFrame(() => {
        input.current?.focus({preventScroll: true})
        navigation.current?.scrollIntoView({block: 'end'})
      })
    } catch (e) {
      if (!signal.aborted) setSendError(e instanceof Error ? e.message : '无法发送消息。')
    } finally {
      if (!signal.aborted) { setSendBusy(false); refresh() }
    }
  }

  async function confirm(actionId: number, workType: string) {
    const signal = lifetime.current?.signal
    try {
      const turn = await request<ChatTurn>(`${path}/actions/${actionId}/confirm`, {
        method: 'POST', body: JSON.stringify(workType ? {work_type: workType} : {}), signal,
      })
      if (!signal?.aborted) setMessages(old => mergeTurns(old, [turn]))
    } finally {
      if (!signal?.aborted) refresh()
    }
  }

  const ready = enabled !== undefined
  return <div className="project-chat">
    <PageHeading title="聊天" description="向 Management Agent 查询当前项目，或发起需要确认的操作。"
      actions={<><a className="button" href={projectHref(projectId, '/agents')}>查看 Agent 配置</a>{messages.length > 0 && <button className="button" type="button" onClick={() => {
        const focusTarget = input.current ?? navigation.current
        focusTarget?.focus({preventScroll: true})
        navigation.current?.scrollIntoView({block: 'end'})
      }}>
        <Icon name="ChevronDown" size={15}/>最新消息
      </button>}</>}/>
    <div className="panel project-chat-panel">
      <div className="project-chat-content">
        {!ready && !readError && <p role="status">正在读取聊天…</p>}
        {readError && <div className="chat-read-problem" role="alert">
          <p>{readError}</p><button className="button" type="button" onClick={refresh}>重新读取</button>
        </div>}
        {ready && !enabled && <p className="chat-agent-unavailable" role="status"><Icon name="AlertCircle" size={17}/>此项目暂未启用聊天 Agent。</p>}
        {ready && !messages.length && <EmptyState icon="MessageSquare" title="暂无项目对话"
          description={enabled ? '从当前项目的任务、验收或资料问起。' : undefined}>
          {enabled && <div className="chat-prompts">{quickPrompts.map(prompt => <button className="button" key={prompt} type="button"
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
            enabled={!!enabled} onConfirm={confirm} onRetry={enabled && !sendBusy && !draft.text.trim() ? focusComposer : undefined}/>)}</ol>
        </div>}
      </div>
      <section ref={navigation} className="chat-latest" tabIndex={-1} aria-label="最新消息及输入">
        {enabled && <form className="chat-composer" onSubmit={e => { e.preventDefault(); void send() }}>
          <ModelSelect key={harness} request={request} harness={harness} label="本条消息模型" defaultLabel="跟随项目聊天配置"
            value={model} disabled={sendBusy} allowCustom={false} onChange={id=>{
              setModel(id);setDraft(old=>({...old,clientID:''}));setSendError('')
            }}/>
          <label htmlFor={field}>发给 Management Agent</label>
          <div className="chat-composer-fields">
            <textarea id={field} ref={input} rows={2} maxLength={4000} aria-describedby={sendError ? `${field}-error` : undefined}
              value={draft.text} readOnly={sendBusy} onChange={e => editDraft(e.currentTarget.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.nativeEvent.isComposing) { e.preventDefault(); void send() }
              }}/>
            <button className="button primary" type="submit" disabled={!draft.text.trim() || sendBusy} aria-keyshortcuts="Control+Enter Meta+Enter">
              {sendBusy ? '发送中…' : sendError ? '重试发送' : '发送消息'}<Icon name="ArrowRight" size={15}/>
            </button>
          </div>
          {sendError && <p className="detail-problem" id={`${field}-error`} role="alert">{sendError}</p>}
        </form>}
      </section>
    </div>
  </div>
}
