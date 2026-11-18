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
