import type { ReactNode } from 'react'
import {
  Activity, AlertCircle, ArrowRight, Bell, Check, CheckCircle2, ChevronDown, ChevronRight,
  Circle, ClipboardCheck, ExternalLink, FileCode2, GitBranch, GitPullRequest,
  History, Inbox, KeyRound, Layers3, LayoutList, ListChecks, LogOut, Menu,
  MessageSquare, RefreshCw, Search, SlidersHorizontal, Terminal, X,
  type LucideProps,
} from 'lucide-react'

const icons = { Activity, AlertCircle, ArrowRight, Bell, Check, CheckCircle2, ChevronDown, ChevronRight,
  Circle, ClipboardCheck, ExternalLink, FileCode2, GitBranch, GitPullRequest, History, Inbox,
  KeyRound, Layers3, LayoutList, ListChecks, LogOut, Menu, MessageSquare,
  RefreshCw, Search, SlidersHorizontal, Terminal, X }

export function Icon({ name, size = 17, ...props }: LucideProps & { name: keyof typeof icons }) {
  const Glyph = icons[name]
  return <Glyph size={size} strokeWidth={1.7} aria-hidden="true" {...props} />
}

export const statusLabels: Record<string, string> = {
  WAITING_INPUT: '等待回答',
  PAUSED: '已暂停', CANCELLED: '已取消', SUPERSEDED: '已从计划移除', PLANNING: '规划中', ACTIVE: '目标执行中', BLOCKED: '等待依赖', READY: '待执行', RUNNING: '执行中', WAITING_ACCEPTANCE: '待验收',
  ACCEPTED: '已验收', DONE: '已完成', FAILED: '失败', COMPLETED: '执行成功',
  PENDING: '待交付', WAITING: '等待合并', MERGED: '已合并',
  REVIEW_PENDING: '等待审查', REVIEW_FAILED: '审查失败', REVIEW_NEEDS_CHECK: '待复核', REVIEW_CLEAR: '审查通过',
}

export function Status({ value }: { value: string }) {
  return <span className={'status status-' + value.toLowerCase()}><i />{statusLabels[value] || value}</span>
}

export function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false })
}

export function safeGitHubURL(value?: string): string | undefined {
  if (!value) return
  try {
    const url = new URL(value)
    if (url.origin === 'https://github.com' && !url.username && !url.password) return url.href
  } catch { /* Ignore malformed evidence links. */ }
}

export function projectHref(projectId: number, suffix = '') {
  return `#/projects/${projectId}${suffix}`
}

export type ApiRequest = <T>(path: string, init?: RequestInit, format?: 'blob') => Promise<T>
export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message) }
}

export function requestWith(onUnauthorized: () => void): ApiRequest {
  return async <T,>(path: string, init: RequestInit = {}, format?: 'blob') => {
    const globalRulesPath = /^\/global-rules(?:\?revision=[1-9]\d*)?$/.test(path)
    const materialPath = /^\/projects\/[1-9]\d*\/(?:experience(?:\?(?:q=[^&#]+|before=[1-9]\d*))?|materials(?:\?[^#]*|\/[^/?#\\]+(?:\?revision=[1-9]\d*|\/revisions\/[1-9]\d*\/curation)?)?)$/.test(path)
    const documentPath = /^\/projects\/[1-9]\d*\/document-changes(?:\?before=[1-9]\d*|\/[1-9]\d*\/curation(?:\?part=[1-9]\d*)?)?$/.test(path)
    const taskEventPath = /^\/projects\/[1-9]\d*\/tasks\/[1-9]\d*\/events\/[1-9]\d*(?:\/curation)?$/.test(path)
    const decisionPath = /^\/projects\/[1-9]\d*\/decisions(?:\?[^#]*|\/search\?status=[A-Z]+&q=[^&#]+|\/[1-9]\d*(?:\/(?:status|curation))?)?$/.test(path)
    const sharePath = /^\/projects\/[1-9]\d*\/(?:shares(?:\/[1-9]\d*(?:\/publication)?)?|shared-experience)(?:\?before=[1-9]\d*)?$/.test(path)
    const roleArchivePath = /^\/role-processes\/[1-9]\d*\/instances\/[1-9]\d*\/archive$/.test(path)
    const shareSearchPath = /^\/projects\/[1-9]\d*\/shared-experience\/search\?q=[^&#]+$/.test(path)
    const nativeApprovalPath = /^\/native-approvals\/[1-9]\d*$/.test(path)
    const codeReviewPath = /^\/projects\/[1-9]\d*\/code-reviews(?:\?before=[1-9]\d*)?$/.test(path)
    const projectWorkPath = /^\/projects\/[1-9]\d*\/(?:overview-context|execution-queue|test-checks(?:\?before=[1-9]\d*)?|chat(?:\?before=[1-9]\d*|\/actions\/[1-9]\d*\/confirm)?)$/.test(path)
    const workerPath = /^\/workers(?:\/(?:control-plane|model-budgets))?$/.test(path)
    const clonePath = /^\/projects\/(?:clone-settings|clone(?:\/jobs\/[1-9]\d*)?)$/.test(path)
    const projectPath = /^\/projects(?:\?[^#]*|\/[1-9]\d*(?:\/(?:check|inbox(?:\?before=[1-9]\d*)?|issues\/[1-9]\d*|classifications\/[1-9]\d*\/processes))?)?$/.test(path)
    if (!nativeApprovalPath && !codeReviewPath && !projectWorkPath && !clonePath && !shareSearchPath && !roleArchivePath && !globalRulesPath && !documentPath && !taskEventPath && !sharePath && !decisionPath && !materialPath && !projectPath && !workerPath && !/^\/executions\/[1-9]\d*\/(?:abandon|retry|cancel|pause-ci|resume-ci|pause-model|resume-model|instructions|turn-inputs)$/.test(path) && !/^\/tasks(?:[/?]|$)/.test(path)) throw new Error('无效的工作台接口')
    const headers = new Headers(init.headers)
    if (typeof init.body === 'string') headers.set('Content-Type', 'application/json')
    let response: Response
    try { response = await fetch(path, { ...init, headers, redirect: 'error', credentials: 'same-origin', cache: 'no-store' }) }
    catch (err) {
      if (init.signal?.aborted) throw err
      throw new Error('无法连接控制面，请检查服务后重试。')
    }
    if (!response.ok) {
      const info = await response.json().catch(() => null)
      if (response.status === 401) onUnauthorized()
      throw new ApiError(response.status, response.status === 401 ? '登录已失效，请重新登录。' :
        response.status === 409 && info?.error === 'select an Issue work type before dispatch' ? '请先为这条 Issue 选择工作类型。' :
        response.status === 409 && info?.error === 'work type differs from the current GitHub Issue' ? '所选工作类型与 GitHub Issue 声明的类型不一致。' :
        response.status === 409 && !clonePath ? '任务状态或候选版本已变化，请刷新记录。' :
        response.status === 404 ? '找不到这条记录。' : (info?.error || '请求失败，请稍后重试。'))
    }
    if (format === 'blob') return await response.blob() as T
    return response.status === 204 ? undefined as T : await response.json() as T
  }
}

export function Fact({ label, children, code = false }: { label: string; children: ReactNode; code?: boolean }) {
  return <div className="detail-fact"><dt>{label}</dt><dd className={code ? 'mono' : undefined}>{children}</dd></div>
}
