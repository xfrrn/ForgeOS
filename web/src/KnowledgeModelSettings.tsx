import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useChatModels, type ChatModel } from './ProjectChatProvider'
import { ApiError, Icon, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'
import { PageHeading, SkeletonCard } from './ui'
import { credentialsHref } from './workbenchRoutes'
import './knowledge-library.css'

type Role = 'curator' | 'management'
type Binding = { role: Role; revision: number; model_id: string | null; model_revision: string | null; last_failure?: string }
type ModelSettings = Record<Role, Binding>

const roles = {
  curator: { title: '知识提取（Curator）', icon: 'Sparkles' as const, help: '从本项目的资料、复盘和更正请求中提取经验陈述与原文引句。' },
  management: { title: '知识审核（Management）', icon: 'ShieldCheck' as const, help: '审核本项目经验以及准备发给其他项目的脱敏副本；审核通过后仍由 Owner 发布。' },
}
const workerDefault = 'worker-default'

function selection(binding: Binding) {
  return binding.revision === 0 ? workerDefault : binding.model_id || ''
}

function connectionName(value: string, models: ChatModel[]) {
  if (value === workerDefault) return '沿用 Worker 配置'
  if (value === '') return '已停止（不调用模型）'
  const model = models.find(item => item.id === value)
  return model ? model.model + ' · ' + model.base_url : '模型连接 ' + value
}

export function KnowledgeModelSettings({ projectId, request }: { projectId: number; request: ApiRequest }) {
  const path = '/projects/' + projectId + '/knowledge/model-settings'
  const catalog = useChatModels(request)
  const [settings, setSettings] = useState<ModelSettings>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const refresh = useCallback(() => setRevision(value => value + 1), [])
  const accept = useCallback((next: ModelSettings) => setSettings(old => old ? {
    curator: old.curator.revision > next.curator.revision ? old.curator : next.curator,
    management: old.management.revision > next.management.revision ? old.management : next.management,
  } : next), [])

  useEffect(() => {
    const controller = new AbortController()
    request<ModelSettings>(path, { signal: controller.signal }).then(next => {
      if (!controller.signal.aborted) { accept(next); setError('') }
    }).catch(cause => {
      if (!controller.signal.aborted) setError(cause.message || '无法读取项目知识库模型设置。')
    })
    return () => controller.abort()
  }, [path, request, revision, accept])

  useEffect(() => {
    const visible = () => { if (!document.hidden) refresh() }
    document.addEventListener('visibilitychange', visible)
    return () => document.removeEventListener('visibilitychange', visible)
  }, [refresh])

  function reload() { refresh(); catalog.refresh() }

  return <div className="knowledge-page">
    <PageHeading title="知识库模型设置" description="分别为本项目的知识提取与知识审核指定模型连接，保存后立即接管待处理任务。"
      actions={<>
        <button type="button" className="button" onClick={reload}><Icon name="RefreshCw" size={14}/>刷新</button>
        <a className="button" href={credentialsHref(projectId, true)}><Icon name="KeyRound" size={14}/>管理模型连接</a>
      </>}/>
    {(error || catalog.error) && <p role="alert" className="detail-problem">{error || catalog.error}{' '}
      <button className="button" onClick={reload}>重新读取</button>
    </p>}
    {(!settings || !catalog.models) && !error && !catalog.error && <SkeletonCard label="正在读取模型设置…" lines={4}/>}
    {catalog.models?.length === 0 && <p className="detail-notice">模型目录为空。请先前往「管理模型连接」配置 Base URL、模型 ID 和 API Key。</p>}
    {settings && <div className="kb-model-grid">
      {(['curator', 'management'] as const).map(role =>
        <RoleBinding key={role} binding={settings[role]} models={catalog.models} request={request} path={path} accept={accept} reload={reload}/>)}
    </div>}
  </div>
}

function RoleBinding({ binding, models, path, request, accept, reload }: {
  binding: Binding; models?: ChatModel[]; path: string; request: ApiRequest
  accept: (next: ModelSettings) => void; reload: () => void
}) {
  const [draft, setDraft] = useState<{ value: string; revision: number }>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [savedRevision, setSavedRevision] = useState<number>()
  const lifetime = useRef<AbortController | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [])

  const role = roles[binding.role]
  const active = selection(binding)
  const chosen = draft?.value ?? active
  const changed = chosen !== active
  const conflict = draft !== undefined && draft.revision !== binding.revision && changed
  const missing = Boolean(chosen && chosen !== workerDefault && models && !models.some(item => item.id === chosen))
  const options = [
    ...(binding.revision === 0 || chosen === workerDefault ? [{ value: workerDefault, label: '沿用 Worker 配置（默认）' }] : []),
    { value: '', label: '停止处理（不调用模型）' },
    ...(models || []).map(item => ({ value: item.id, label: item.model, group: item.base_url })),
    ...(missing ? [{ value: chosen, label: '已移除的模型连接 · ' + chosen }] : []),
  ]
  const statusTone = active === '' ? 'neutral' : active === workerDefault ? 'info' : 'positive'
  const statusLabel = active === '' ? '已停止' : active === workerDefault ? 'Worker 默认' : '直连模型'

  async function save(event: FormEvent) {
    event.preventDefault()
    if (busy || !draft || !changed || missing || chosen === workerDefault) return
    const signal = lifetime.current?.signal
    setBusy(true); setError('')
    try {
      const next = await request<ModelSettings>(path, {
        method: 'PUT', signal,
        body: JSON.stringify({ role: binding.role, expected_revision: binding.revision, model_id: chosen || null }),
      })
      if (!signal?.aborted) {
        accept(next); setDraft(undefined); setSavedRevision(next[binding.role].revision)
      }
    } catch (cause) {
      if (signal?.aborted) return
      if (cause instanceof ApiError && cause.status === 409) {
        setError('项目设置或模型连接已发生变化。已保留你的选择，请核对当前设置。')
        reload()
      } else setError(cause instanceof Error ? cause.message : '保存失败。')
    } finally { if (!signal?.aborted) setBusy(false) }
  }

  return <section className="panel knowledge-role-settings" aria-label={role.title}>
    <div className="kb-role-head">
      <span className="kb-role-icon"><Icon name={role.icon} size={18}/></span>
      <div>
        <h2>{role.title}</h2>
        <p>{role.help}</p>
      </div>
    </div>

    <div className="kb-role-active">
      <div>
        <span>当前生效</span>
        <div><strong>{connectionName(active, models || [])}</strong></div>
      </div>
      <span className={'kb-badge ' + statusTone}>{statusLabel}</span>
    </div>

    {binding.last_failure && <p className="detail-problem" role="status">最近失败 · {binding.last_failure}</p>}
    {binding.revision === 0 && <p className="muted">模型 API 默认关闭，已配置 CLI 的 Worker 仍可继续处理该角色任务。</p>}

    <form onSubmit={save}>
      <ThemedSelect label={binding.role === 'curator' ? '提取模型' : '审核模型'} value={chosen} options={options} disabled={busy}
        onChange={value => { setDraft({ value, revision: binding.revision }); setError(''); setSavedRevision(undefined) }}/>
      {chosen === '' && changed && <p className="muted">保存后，该角色将停止领取本项目的新知识任务。</p>}
      {conflict && <p className="detail-problem" role="alert">其他窗口已更新此角色。可以保留当前设置，或按你选择的模型保存。</p>}
      {missing && <p className="detail-problem" role="alert">此连接已从模型目录移除，请重新选择。</p>}
      <div className="knowledge-actions">
        <button className="button primary" disabled={busy || !changed || missing || chosen === workerDefault}>
          {busy ? '正在保存…' : conflict ? '按我的选择保存' : binding.role === 'curator' ? '保存提取设置' : '保存审核设置'}
        </button>
        {draft && <button type="button" className="button" disabled={busy} onClick={() => { setDraft(undefined); setError('') }}>
          {conflict ? '保留当前设置' : '撤销修改'}
        </button>}
      </div>
      {error && <p className="detail-problem" role="alert">{error} <button type="button" className="button" disabled={busy} onClick={reload}>刷新</button></p>}
      {savedRevision === binding.revision && <p className="detail-notice" role="status">{role.title}设置已保存。</p>}
    </form>
  </section>
}
