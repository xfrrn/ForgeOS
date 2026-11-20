export type ProjectTab = 'overview' | 'chat' | 'tasks' | 'reviews' | 'materials'

type TaskRoute = {
  id: number
  preview?: string
  planRevision?: number
  questionId?: number
  executionId?: number
}

type ProjectPage =
  | { view: 'overview' | 'chat' | 'inbox' | 'questions' | 'notifications' }
  | { view: 'materials'; key?: string; revision?: number }
  | { view: 'tasks'; status?: string; queue: boolean }
  | { view: 'reviews'; tab: 'approvals' | 'acceptance' | 'code-reviews' | 'checks' }
  | { view: 'decisions' | 'shares'; record?: number }
  | { view: 'task'; task: TaskRoute }

export type ProjectRoute = ProjectPage & {
  kind: 'project'
  projectId: number
  projectTab: ProjectTab
  captureQuery?: string
}

export type WorkbenchRoute =
  | { kind: 'projects' | 'system' | 'notifications' | 'missing' }
  | { kind: 'legacy-task'; task: TaskRoute; path: string; captureQuery?: string }
  | ProjectRoute

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
  if (path === '/workers') return { kind: 'system' }
  if (path === '/notifications') return { kind: 'notifications' }
  const legacy = readTaskRoute(path)
  if (legacy) return { kind: 'legacy-task', task: legacy, path: location, captureQuery: query }

  const scope = /^\/projects\/([1-9]\d*)(\/.*)?$/.exec(path)
  const projectId = positiveId(scope?.[1])
  if (!projectId || !scope) return { kind: 'missing' }
  const root = { kind: 'project' as const, projectId }
  const subpath = scope[2] ?? '/'
  switch (subpath) {
    case '/': return { ...root, view: 'overview', projectTab: 'overview' }
    case '/chat': return { ...root, view: 'chat', projectTab: 'chat' }
    case '/tasks': return { ...root, view: 'tasks', projectTab: 'tasks', status: params.get('status') ?? undefined, queue: params.get('tab') === 'queue' }
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
        revision: positiveId(params.get('revision') ?? undefined) }
    }
  }
  const task = readTaskRoute(subpath)
  if (task) return { ...root, view: 'task', task: { ...task, executionId: positiveId(params.get('execution') ?? undefined) },
    projectTab: 'tasks', captureQuery: query }
  const material = /^\/(decisions|shares)(?:\/([1-9]\d*))?$/.exec(subpath)
  if (material) {
    const record = positiveId(material[2])
    if (!material[2] || record) return { ...root, view: material[1] === 'decisions' ? 'decisions' : 'shares', record, projectTab: 'materials' }
  }
  return { kind: 'missing' }
}

export function workbenchTitle(route: WorkbenchRoute): string {
  if (route.kind === 'projects') return '项目'
  if (route.kind === 'system') return '执行节点'
  if (route.kind === 'notifications') return '通知'
  if (route.kind === 'legacy-task') return route.task.preview ? '预览反馈' : '任务详情'
  if (route.kind !== 'project') return '页面不存在'
  if (route.view === 'task') return route.task.preview ? '预览反馈' : '任务详情'
  if (route.view === 'reviews') return { approvals: '待审批', acceptance: '待验收', 'code-reviews': '代码审查', checks: '测试与检查' }[route.tab]
  if (route.view === 'tasks' && route.queue) return '执行队列'
  return {
    overview: '项目概览', chat: '项目聊天', tasks: '项目任务', inbox: '讨论收件箱', materials: '项目资料',
    decisions: '项目决策', shares: '共享经验', questions: '待答问题', notifications: '项目通知',
  }[route.view]
}
