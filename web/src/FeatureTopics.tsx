import { useEffect, useRef, useState } from 'react'
import { Icon, projectHref, formatTime, type ApiRequest } from './shared'
import { FeatureClientHealth } from './FeatureClientHealth'
import { ProjectChat } from './ProjectChat'
import { useDiscussionModels } from './ProjectChatProvider'
import { ThemedSelect } from './ThemedSelect'
import { EmptyState } from './ui'
import './feature-topics.css'

const deliveryStages:Record<string,[string,string]> = {
  implementation_queued:['等待实现',''],implementing:['实现中','active'],
  implementation_failed:['实现失败 · 需要处理','attention'],step_cancelled:['子任务已取消','attention'],
  integration_queued:['等待合成',''],integrating:['合成验证中','active'],integration_failed:['合成失败 · 需要处理','attention'],
  rework_queued:['验收退回 · 待返工','attention'],reworking:['返工中','active'],
  waiting_input:['等你答复','attention'],paused:['已暂停',''],acceptance:['等你验收','attention'],
  accepted:['已验收','done'],delivery:['已验收 · 交付中','active'],merged:['已合并','done'],cancelled:['已取消',''],superseded:['已替代',''],
}
const phases:Record<string,string>={handoff:'待派工',execution:'已派工',design:'设计中',review:'待拍板',proposals:'方案中',discussion:'讨论中'}
type Topic = { delivery_stage?:string; total_steps?:number; finished_steps?:number; phase?:string; id: number; title: string; profile_id: string; updated_at: string }

export function FeatureTopics({ projectId, selected, proposals, delivery, request }: { projectId: number; selected?: number; proposals?:boolean; delivery?:boolean; request: ApiRequest }) {
  const path = '/projects/' + projectId + '/features'
  const href = projectHref(projectId, '/features')
  const [topics, setTopics] = useState<Topic[]>()
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const [model, setModel] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const intent = useRef({ key: '', id: '' })
  const { models, error: modelError } = useDiscussionModels(request, projectId)
  const profile = model || models?.[0]?.id || ''
  const topic = topics?.find(item => item.id === selected)

  useEffect(() => {
    const controller = new AbortController()
    setError('')
    request<Topic[]>(path, { signal: controller.signal }).then(items => {
      if (!controller.signal.aborted) setTopics(items)
    }).catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [path, selected, refresh, request])

  useEffect(() => {
    const changed = () => setRefresh(v => v + 1)
    window.addEventListener('forgeos-proposal-changed', changed)
    const timer=window.setInterval(()=>{if(!document.hidden)changed()},15000)
    return () => {window.removeEventListener('forgeos-proposal-changed', changed);window.clearInterval(timer)}
  }, [])

  async function create() {
    if (!title.trim() || !profile || busy) return
    const key = JSON.stringify([title.trim(), profile])
    if (intent.current.key !== key) intent.current = { key, id: crypto.randomUUID() }
    setBusy(true); setError('')
    try {
      const item = await request<Topic>(path, { method: 'POST', body: JSON.stringify({ title: title.trim(), profile_id: profile, client_id: intent.current.id }) })
      setTopics(old => [item, ...(old || []).filter(t => t.id !== item.id)])
      setCreating(false); setTitle(''); intent.current = { key: '', id: '' }
      window.location.hash = href + '/' + item.id
    } catch (e) { setError(e instanceof Error ? e.message : '创建话题失败。') }
    finally { setBusy(false) }
  }


  return <div className={'feature-room' + (selected || creating ? ' feature-open' : '')}>
    <aside className="feature-list" aria-label="功能话题">
      <div className="feature-list-head">
        <h2>功能话题</h2>
        <button className="button" type="button" onClick={() => setCreating(true)}>
          <Icon name="Plus" size={14}/>新建
        </button>
      </div>
      <div className="feature-items">
        {topics?.map(item => <a key={item.id} className="feature-link" aria-current={item.id === selected ? 'page' : undefined}
          href={href + '/' + item.id + (item.phase==='review'?'?view=proposal':['design','handoff','execution'].includes(item.phase||'')?'?view=delivery':'')} onClick={() => setCreating(false)}>
          <span>{item.title}</span>
          <small className="feature-link-meta"><strong data-tone={deliveryStages[item.delivery_stage||'']?.[1]}>{deliveryStages[item.delivery_stage||'']?.[0] || phases[item.phase||'discussion']}{!!item.total_steps && ' · '+(item.finished_steps||0)+'/'+item.total_steps}</strong><time dateTime={item.updated_at}>{formatTime(item.updated_at)}</time></small>
        </a>)}
        {topics?.length === 0 && <p className="feature-empty-hint">还没有功能话题，点击右上角新建。</p>}
      </div>
      <FeatureClientHealth projectId={projectId} request={request}/>
    </aside>

    <section className="feature-detail">
      {error && <p className="detail-problem feature-error" role="alert">{error}{' '}
        <button className="button" type="button" onClick={() => setRefresh(r => r + 1)}>重新读取</button>
      </p>}
      {creating ? <div className="feature-form-wrap">
        <form className="feature-form" onSubmit={e => { e.preventDefault(); void create() }}>
          <h2>新建功能话题</h2>
          <label>功能名称
            <input required maxLength={120} placeholder="例如：支持工作台多模型方案对比" value={title} onChange={e => setTitle(e.target.value)}/>
          </label>
          <ThemedSelect label="讨论模型" value={profile} options={(models || []).map(m => ({ value: m.id, label: m.name || m.model }))} onChange={setModel}/>
          {modelError && <p className="detail-problem" role="alert">{modelError}</p>}
          {models?.length === 0 && <p className="muted">请先在账号设置中添加模型连接。</p>}
          <div className="feature-form-actions">
            <button className="button primary" disabled={busy || !profile || !title.trim()}>创建并开始讨论</button>
            <button className="button" type="button" disabled={busy} onClick={() => setCreating(false)}>取消</button>
          </div>
        </form>
      </div> : topic ? <>
        <a className="feature-back" href={href}><Icon name="ArrowLeft" size={14}/>返回功能话题</a>
        <ProjectChat key={topic.id} projectId={projectId} request={request} feature={topic} showProposals={proposals} delivery={delivery}/>
      </> : <div className="feature-placeholder">
        <EmptyState
          icon="Sparkles"
          title={!topics ? '正在读取功能话题…' : selected ? '找不到这个功能话题' : '围绕功能与多个 AI 模型持续协作'}
          description={!topics ? undefined : selected ? '请从左侧选择已有话题，或新建一个功能话题。' : '选择左侧话题继续讨论，或开启新的功能构思。'}>
          {topics && <button className="button primary" type="button" onClick={() => setCreating(true)}>
            <Icon name="Plus" size={15}/>新建功能话题
          </button>}
        </EmptyState>
      </div>}
    </section>
  </div>
}
