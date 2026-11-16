import type { ReactNode } from 'react'
import {
  Activity, AlertCircle, ArrowRight, Check, CheckCircle2, ChevronDown, ChevronRight,
  Circle, ClipboardCheck, ExternalLink, FileCode2, GitBranch, GitPullRequest,
  History, Inbox, KeyRound, Layers3, LayoutList, ListChecks, LogOut, Menu,
  MessageSquare, PanelLeft, RefreshCw, Search, SlidersHorizontal, Terminal,
  type LucideProps,
} from 'lucide-react'

const icons = { Activity, AlertCircle, ArrowRight, Check, CheckCircle2, ChevronDown, ChevronRight,
  Circle, ClipboardCheck, ExternalLink, FileCode2, GitBranch, GitPullRequest, History, Inbox,
  KeyRound, Layers3, LayoutList, ListChecks, LogOut, Menu, MessageSquare, PanelLeft,
  RefreshCw, Search, SlidersHorizontal, Terminal }

export function Icon({ name, size = 17, ...props }: LucideProps & { name: keyof typeof icons }) {
  const Glyph = icons[name]
  return <Glyph size={size} strokeWidth={1.7} aria-hidden="true" {...props} />
}

export const statusLabels: Record<string, string> = {
  WAITING_INPUT: '等待回答',
  PAUSED: '已暂停', CANCELLED: '已取消', SUPERSEDED: '已从计划移除', PLANNING: '规划中', ACTIVE: '目标执行中', BLOCKED: '等待依赖', READY: '待执行', RUNNING: '执行中', WAITING_ACCEPTANCE: '待验收',
  ACCEPTED: '已验收', DONE: '已完成', FAILED: '失败', COMPLETED: '执行成功',
  PENDING: '待交付', WAITING: '等待合并', MERGED: '已合并',
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

export type ApiRequest = <T>(path: string, init?: RequestInit, format?: 'blob') => Promise<T>
export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message) }
}

export function requestWith(token: string, onUnauthorized: () => void): ApiRequest {
  return async <T,>(path: string, init: RequestInit = {}, format?: 'blob') => {
    const materialPath = /^\/projects\/[1-9]\d*\/materials(?:\?[^#]*|\/[^/?#\\]+(?:\?revision=[1-9]\d*)?)?$/.test(path)
    const decisionPath = /^\/projects\/[1-9]\d*\/decisions(?:\?[^#]*|\/[1-9]\d*\/status)?$/.test(path)
    if (!decisionPath && !materialPath && path !== '/workers' && !/^\/executions\/[1-9]\d*\/(?:cancel|pause-ci|resume-ci)$/.test(path) && !/^\/tasks(?:[/?]|$)/.test(path) && !/^\/projects(?:\?before=[1-9]\d*|\/[1-9]\d*\/(?:check|issues\/[1-9]\d*))?$/.test(path)) throw new Error('无效的工作台接口')
    const headers = new Headers(init.headers)
    headers.set('Authorization', 'Bearer ' + token)
    if (typeof init.body === 'string') headers.set('Content-Type', 'application/json')
    let response: Response
    try { response = await fetch(path, { ...init, headers, redirect: 'error', credentials: 'omit', cache: 'no-store' }) }
    catch (err) {
      if (init.signal?.aborted) throw err
      throw new Error('无法连接控制面，请检查服务后重试。')
    }
    if (!response.ok) {
      const info = await response.json().catch(() => null)
      if (response.status === 401) onUnauthorized()
      throw new ApiError(response.status, response.status === 401 ? 'Owner 令牌无效，请重新连接。' :
        response.status === 409 ? '任务状态或候选版本已变化，请刷新记录。' :
        response.status === 404 ? '找不到这条记录。' : (info?.error || '请求失败，请稍后重试。'))
    }
    if (format === 'blob') return await response.blob() as T
    return response.status === 204 ? undefined as T : await response.json() as T
  }
}

export function Fact({ label, children, code = false }: { label: string; children: ReactNode; code?: boolean }) {
  return <div className="detail-fact"><dt>{label}</dt><dd className={code ? 'mono' : undefined}>{children}</dd></div>
}
