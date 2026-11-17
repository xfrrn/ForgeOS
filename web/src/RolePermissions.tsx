export type StructuredProfile = { version: number; sandbox: string; permission: string; disabled_features: string[] }

export function RolePermissions({role, profile}: {role: string; profile: StructuredProfile}) {
  return <details><summary>{role}权限配置</summary>
    <p>Codex · 配置 v{profile.version} · 沙箱 <code>{profile.sandbox}</code> · 审批 <code>{profile.permission}</code></p>
    <p>禁用功能 · {profile.disabled_features.join('、')}</p>
  </details>
}
