import { taskCategory, type TaskCategory } from './workTypes'

export type ProjectTab = 'overview' | 'features' | 'chat' | 'agents' | 'tasks' | 'reviews' | 'materials' | 'knowledge' | 'planning'

type TaskRoute = {
  id: number
  preview?: string
  planRevision?: number
  questionId?: number
  executionId?: number
  category?: TaskCategory
}

type ProjectPage =
  | {view:'features'; feature?:number; proposals?:boolean; delivery?:boolean}
  | { view: 'overview' | 'chat' | 'agents' | 'inbox' | 'questions' | 'notifications' | 'model-settings' }
  | { view: 'planning' | 'retrospectives'; selected?: number; item?: number }
  | { view: 'materials'; key?: string; revision?: number; path?: string }
  | { view: 'tasks'; status?: string; queue: boolean; category: TaskCategory }
  | { view: 'reviews'; tab: 'approvals' | 'acceptance' | 'code-reviews' | 'checks' }
  | { view: 'decisions' | 'experience'; record?: number }
  | { view: 'shares'; record?: number; received: boolean }
  | { view: 'task'; task: TaskRoute }

export type ProjectRoute = ProjectPage & {
  kind: 'project'
  projectId: number
  projectTab: ProjectTab
  captureQuery?: string
}

export type WorkbenchRoute =
  | { kind: 'projects' | 'skills' | 'agents' | 'system' | 'notifications' | 'missing' }
  | { kind: 'credentials'; projectId?: number; fromKnowledge?: boolean }
  | { kind: 'legacy-task'; task: TaskRoute; path: string; captureQuery?: string }
  | ProjectRoute

export function credentialsHref(projectId?: number, fromKnowledge = false) {
  return '#/settings/credentials' + (projectId ? '?project=' + projectId + (fromKnowledge ? '&from=knowledge' : '') : '')
}

function positiveId(value: string | undefined): number | undefined {
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? id : undefined
}

function readTaskRoute(path: string): TaskRoute | undefined {
  const match = /^\/tasks\/([1-9]\d*)(?:\/(?:previews\/([a-f0-9]{48})\/feedback|plans\/([1-9]\d{0,9})|questions\/([1-9]\d*)))?$/.exec(path)
  if (!match) return
  const id = positiveId(match[1])
  const planRevision = positiveId(match[3])
  const questionId = positiveId(match[4])
  if (!id || (match[3] && !planRevision) || (match[4] && !questionId)) return
  return { id, preview: match[2], planRevision, questionId }
}

export function workbenchRoute(location: string): WorkbenchRoute {
  const separator = location.indexOf('?')
  const path = separator === -1 ? location : location.slice(0, separator)
  const query = separator === -1 ? undefined : location.slice(separator + 1)
  const params = new URLSearchParams(query)
  if (['/', '/projects', '/overview', '/tasks', '/review', '/approvals', '/questions'].includes(path)) {
    return { kind: 'projects' }
  }
  if (path === '/skills') return { kind: 'skills' }
  if (path === '/agents') return { kind: 'agents' }
  if (path === '/settings/credentials') return { kind: 'credentials', projectId: positiveId(params.get('project') ?? undefined), fromKnowledge: params.get('from') === 'knowledge' }
  if (path === '/workers') return { kind: 'system' }
  if (path === '/notifications') return { kind: 'notifications' }
  const legacy = readTaskRoute(path)
  if (legacy) return { kind: 'legacy-task', task: legacy, path: location, captureQuery: query }

  const scope = /^\/projects\/([1-9]\d*)(\/.*)?$/.exec(path)
  const projectId = positiveId(scope?.[1])
  if (!projectId || !scope) return { kind: 'missing' }
  const root = { kind: 'project' as const, projectId }
  const subpath = scope[2] ?? '/'
  const feature = /^\/features(?:\/([1-9]\d*))?$/.exec(subpath)
  if (feature && (!feature[1] || positiveId(feature[1]))) return {...root,view:'features',projectTab:'features',feature:positiveId(feature[1]),proposals:params.get('view')==='proposal',delivery:params.get('view')==='delivery'}
  switch (subpath) {
    case '/': return { ...root, view: 'overview', projectTab: 'overview' }
    case '/planning': return { ...root, view: 'planning', projectTab: 'planning', selected: positiveId(params.get('milestone') ?? undefined), item: positiveId(params.get('item') ?? undefined) }
    case '/planning/retrospectives': return { ...root, view: 'retrospectives', projectTab: 'planning', selected: positiveId(params.get('retro') ?? undefined) }
    case '/agents': return { ...root, view: 'agents', projectTab: 'agents' }
    case '/chat': return { ...root, view: 'chat', projectTab: 'chat' }
    case '/knowledge': return { ...root, view: 'decisions', projectTab: 'knowledge' }
    case '/knowledge/model-settings': return { ...root, view: 'model-settings', projectTab: 'knowledge' }
    case '/tasks': return { ...root, view: 'tasks', projectTab: 'tasks', status: params.get('status') ?? undefined,
      queue: params.get('tab') === 'queue', category: taskCategory(params.get('category')) }
    case '/inbox': return { ...root, view: 'inbox', projectTab: 'tasks' }
    case '/reviews': {
      const tab = params.get('tab')
      return { ...root, view: 'reviews', projectTab: 'reviews', tab: tab === 'acceptance' || tab === 'code-reviews' || tab === 'checks' ? tab : 'approvals' }
    }
    case '/questions': return { ...root, view: 'questions', projectTab: 'reviews' }
    case '/notifications': return { ...root, view: 'notifications', projectTab: 'overview' }
    case '/materials': {
      const key = params.get('key') ?? ''
      return { ...root, view: 'materials', projectTab: 'materials',
        key: /^[\p{L}\p{N}][\p{L}\p{N}._-]{0,95}$/u.test(key) ? key : undefined,
        path: params.get('path') || undefined,
        revision: positiveId(params.get('revision') ?? undefined) }
    }
  }
  const task = readTaskRoute(subpath)
  if (task) return { ...root, view: 'task', task: { ...task, executionId: positiveId(params.get('execution') ?? undefined),
    category: params.has('category') ? taskCategory(params.get('category')) : undefined },
    projectTab: 'tasks', captureQuery: query }
  const knowledge = /^(?:\/knowledge)?\/(decisions|shares|experience)(?:\/(sent|received))?(?:\/([1-9]\d*))?$/.exec(subpath)
  if (knowledge) {
    const [, view, direction, number] = knowledge
    const record = positiveId(number)
    if (number && !record || direction && view !== 'shares') return { kind:'missing' }
    if (view === 'shares') return { ...root, view, record, received: direction === 'received' || (!direction && !record), projectTab:'knowledge' }
    if (view === 'decisions' || view === 'experience') return { ...root, view, record, projectTab:'knowledge' }
  }
  return { kind: 'missing' }
}

export function workbenchTitle(route: WorkbenchRoute): string {
  if (route.kind === 'projects') return '项目'
  if (route.kind === 'skills') return 'Skill 库'
  if (route.kind === 'agents') return 'Agent 管理'
  if (route.kind === 'credentials') return '密钥与凭据'
  if (route.kind === 'system') return '运行环境'
  if (route.kind === 'notifications') return '通知'
  if (route.kind === 'legacy-task') return route.task.preview ? '预览反馈' : '任务详情'
  if (route.kind !== 'project') return '页面不存在'
  if (route.view === 'task') return route.task.preview ? '预览反馈' : '任务详情'
  if (route.view === 'reviews') return { approvals: '待审批', acceptance: '待验收', 'code-reviews': '代码审查', checks: '测试与检查' }[route.tab]
  if (route.view === 'tasks' && route.queue) return '执行队列'
  return {
    features:'功能', planning: '项目规划', retrospectives: '项目复盘', overview: '项目概览', agents: 'Agent 管理', chat: '项目聊天', tasks: '项目任务', inbox: '讨论收件箱', materials: '项目资料',
    decisions: '项目决策', experience: '项目经验', shares: '共享经验', 'model-settings': '知识库模型设置', questions: '待答问题', notifications: '项目通知',
  }[route.view]
}
