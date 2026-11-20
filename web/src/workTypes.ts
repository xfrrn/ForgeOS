export const workTypes = {
  development: '开发',
  'code-review': '代码审查',
  testing: '测试',
  investigation: '调查',
  operations: '运维',
}

export function workTypeName(type?: string) {
  return type && Object.hasOwn(workTypes, type) ? workTypes[type as keyof typeof workTypes] : undefined
}

export const systemTaskTypes = {
  'git-head': 'Git HEAD 检查',
  'environment-check': '环境启动检查',
}

const taskCategories = { ...workTypes, ...systemTaskTypes }
export type TaskCategory = keyof typeof taskCategories

export function taskCategory(value: string | null): TaskCategory {
  return value && Object.hasOwn(taskCategories, value) ? value as TaskCategory : 'development'
}
