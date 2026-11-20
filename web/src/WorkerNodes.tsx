import { useEffect, useState } from 'react'
import { Fact, formatTime, Icon, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import { HostDiagnostics, useRuntimeStatus, type Diagnostics, type WorkerSummary } from './RuntimeStatus'

type AgentProfile = { version: number; sandbox?: string; permission: string; tools?: string[] }
type Adapter = { version: string; operations: string[]; profiles?: Partial<Record<'implementation' | 'planner', AgentProfile>> }
type PoolLimits = { normal: number; maximum: number }
type FixedLimits = {workers:number; tasks:number; model:number; ci:number}
type Node = WorkerSummary & { fixed_limits?:FixedLimits; resident_online?:number|null; pool?: PoolLimits; pool_running?: number; capabilities: Record<string, unknown> & { adapters?: Record<string, Adapter>; diagnostics?: Diagnostics } }

const agentRoleLabels = { curator: '知识提取', curation_review: '知识审核', plan_review: 'Goal 计划审核', share_review: '共享资料审核', repository_analysis: '仓库分析', issue_classification: 'Issue 分类', feedback_classification: '预览反馈分类', question_advice: '业务问题建议' }

type ModelBudget = { name: string; harness: string; slots: number; reserved: number; workers: string[]; reviewer_projects?: number[]; test_plan_roles?: {project_id: number; role: 'test' | 'management'}[]; agent_roles?: {project_id: number; role: keyof typeof agentRoleLabels}[] }

const capabilities: Record<string, string> = { supports_startup: '本地环境启动', supports_codex: 'Codex', supports_claude: 'Claude Code', supports_planner: '规划', supports_global_rules: '全局默认规则',
  supports_checkpoints: '检查点', supports_rework: '退回修订', supports_validation: '候选验证', supports_declared_checks: '项目检查', supports_risk_policy: '按风险选级' }
const operations: Record<string, string> = { probe: '探测', start: '启动', resume: '恢复会话', events: '读取事件', interrupt: '中断回合' }

export function WorkerNodes({ request }: { request: ApiRequest }) {
  const { nodes, control, nodeError, controlError } = useRuntimeStatus<Node>(request)
  const [budgets, setBudgets] = useState<ModelBudget[]>([])
  const [budgetError, setBudgetError] = useState('')
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const accounts = await request<ModelBudget[]>('/workers/model-budgets', { signal: stop.signal })
        if (!stop.signal.aborted) { setBudgets(accounts); setBudgetError('') }
      } catch { if (!stop.signal.aborted) setBudgetError('读取共享模型名额失败。') }
      if (!stop.signal.aborted) timer = setTimeout(refresh, 15000)
    }
    void refresh()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request])
  return <>
    <PageHeading title="运行环境" description="按 ForgeOS 服务进程所在的机器查看资源与 CLI。"/>
    {nodeError && <p className="detail-problem" role="alert">{nodes ? '节点刷新失败，显示上次结果。' : '读取执行节点失败。'}</p>}
    {budgetError && <p className="detail-problem" role="alert">{budgetError}</p>}
    <div className="worker-stack">
      <section className="worker-grid" aria-label="控制面与执行节点">
        <article className="panel detail-card" aria-labelledby="control-plane-heading">
          <h2 id="control-plane-heading"><Icon name="Activity" size={18}/>控制面</h2>
          {controlError && <p className="detail-problem" role="alert">{control ? '控制面状态刷新失败，显示上次结果。' : '当前无法读取控制面状态。'}</p>}
          {!control && !controlError && <p className="muted" role="status">正在读取控制面状态…</p>}
          {control && <>
            <dl className="detail-facts">
              <Fact label="运行环境">{control.os} / {control.arch} · {control.cpus} 个逻辑 CPU</Fact>
              <Fact label="PostgreSQL">{control.postgresql === 'reachable' ? '连通' : <span className="runtime-warning">不可用</span>}</Fact>
            </dl>
            <HostDiagnostics diagnostics={control.diagnostics}/>
          </>}
        </article>
        {!nodeError && !nodes?.length && <div className="panel">{nodes ?
          <EmptyState icon="Terminal" title="尚未收到 Worker 心跳"/> :
          <div className="table-message" role="status">正在读取节点…</div>}
        </div>}
        {nodes?.map(n => <article key={n.worker} className="panel detail-card">
          <div className="worker-card-heading"><h2><Icon name="Terminal" size={18}/>{n.worker}</h2>
            <span className={'status worker-presence' + (n.online ? ' online' : '')}><i aria-hidden="true"/>{n.online ? '在线' : '心跳超时'}</span>
          </div>
          <dl className="detail-facts">
            <Fact label="运行环境">{n.os} / {n.arch} · {n.cpus} 个逻辑 CPU</Fact>
            <Fact label={n.resident_online != null ? "在途任务" : "已占名额"} code>{n.reserved} / {n.slots}</Fact>
            <Fact label="最近心跳"><time dateTime={n.last_seen}>{formatTime(n.last_seen)}</time></Fact>
            {!!n.model_slots && !!n.ci_slots && <>
              <Fact label={n.resident_online != null ? "并发模型回合" : "模型名额"} code>{n.model_reserved} / {n.model_slots}</Fact>
              <Fact label="CI 名额" code>{n.ci_reserved} / {n.ci_slots}</Fact>
            </>}
          </dl>
          {n.fixed_limits && n.resident_online != null && <FixedPoolSettings key={n.worker} node={n.worker}
            policy={n.fixed_limits} count={n.resident_online} online={n.online} request={request}/>}
          {n.pool && <PoolSettings key={n.worker} policy={n.pool} running={n.pool_running ?? 0} online={n.online} request={request}/>}
          <div className="worker-capabilities">
            <h3>执行能力</h3>
            <p>{['Git HEAD', ...Object.entries(capabilities).filter(([key]) => n.capabilities[key]).map(([, label]) => label)].join(' · ')}</p>
            {Object.entries(n.capabilities.adapters ?? {}).map(([name, probe]) =>
              <div key={name}><p>{capabilities[`supports_${name}`] ?? name} {probe.version} · {probe.operations.map(op => operations[op] ?? op).join('、')}</p>
                {Object.entries(probe.profiles ?? {}).map(([role, p]) => <p key={role}>
                  {role === 'planner' ? '规划' : '实现'}角色配置 v{p.version} · 权限 {p.permission}
                  {p.sandbox && ` · 沙箱 ${p.sandbox}`}{p.tools && ` · 工具 ${p.tools.join('、')}`}
                </p>)}
              </div>)}
          </div>
          <HostDiagnostics diagnostics={n.capabilities.diagnostics} online={n.online}/>
        </article>)}
      </section>
      {budgets.length > 0 && <section className="panel detail-card" aria-labelledby="shared-model-budgets">
        <h2 id="shared-model-budgets"><Icon name="Layers3" size={18}/>共享模型名额</h2>
        <ul className="worker-budgets">
          {budgets.map(b => {
            const consumers = [...b.workers, ...(b.reviewer_projects ?? []).map(id => `项目 ${id} 的 Reviewer`),
              ...(b.test_plan_roles ?? []).map(m => `项目 ${m.project_id} ${m.role === 'test' ? '的 Test Agent' : '的覆盖审核'}`),
              ...(b.agent_roles ?? []).map(m => `项目 ${m.project_id} 的${agentRoleLabels[m.role]}`)]
            return <li key={b.name} className="worker-budget"><div><strong>{b.name}</strong><p className="muted">
              {b.harness}{consumers.length > 0 && ' · ' + consumers.join('、')}
            </p></div><span className="mono">已占 {b.reserved} / {b.slots}</span></li>
          })}
        </ul>
      </section>}
    </div>
  </>
}


function PoolSettings({policy, running, online, request}: {policy:PoolLimits;running:number;online:boolean;request:ApiRequest}) {
  const [draft, setDraft] = useState<PoolLimits | null>(null)
  const [saved, setSaved] = useState<PoolLimits | null>(null)
  const [busy,setBusy] = useState(false)
  const [notice,setNotice] = useState('')
  const [error,setError] = useState('')
  useEffect(() => {setSaved(null)}, [policy.normal, policy.maximum])
  const current = saved ?? policy
  const value = draft ?? current
  const changed = value.normal !== current.normal || value.maximum !== current.maximum
  const valid = Number.isInteger(value.normal) && Number.isInteger(value.maximum) && value.normal >= 1 && value.maximum >= value.normal && value.maximum <= 2147483647
  function edit(key: keyof PoolLimits, n: number) {
    setDraft({...value, [key]: n}); setNotice(''); setError('')
  }
  async function save() {
    if (!valid || !changed || busy) return
    setBusy(true); setError('')
    try {
      await request('/workers/process-pool', {method:'PUT',body:JSON.stringify(value)})
      setSaved(value); setDraft(null); setNotice('已保存。池管理进程会在下一次心跳时读取设置。')
    } catch(e) { setError(e instanceof Error ? e.message : '保存失败。') }
    finally { setBusy(false) }
  }
  return <section className="worker-capabilities pool-settings">
    <h3>Worker 进程池</h3>
    <p>{online ? '最近心跳时' : '离线前最后一次心跳时'}有 <strong>{running}</strong> 个任务 Worker 进程。</p>
    <form onSubmit={e=>{e.preventDefault();void save()}}>
      <label>常规上限<input required type="number" min="1" max="2147483647" value={value.normal || ''} onChange={e=>edit('normal',e.target.valueAsNumber)}/></label>
      <label>硬上限<input required type="number" min={value.normal || 1} max="2147483647" value={value.maximum || ''} onChange={e=>edit('maximum',e.target.valueAsNumber)}/></label>
      <button className="button" disabled={busy || !changed || !valid}>{busy ? '正在保存…' : '保存进程上限'}</button>
    </form>
    <p>有可领取的任务时启动进程，任务结束后回收。常规名额连续占满 15 秒后可超开至硬上限。两项相同即关闭超开。</p>
    {changed && !valid && <p role="alert">请输入正整数，硬上限须大于或等于常规上限。</p>}
    {notice && <p role="status">{notice}</p>}{error && <p role="alert">{error}</p>}
  </section>
}

const fixedFields = [
  ['workers','常驻 Worker 数'], ['tasks','并发任务上限'], ['model','并发模型回合上限'], ['ci','并发 CI 上限']
] as const

function FixedPoolSettings({node,policy,count,online,request}: {node:string;policy:FixedLimits;count:number;online:boolean;request:ApiRequest}) {
  const [draft,setDraft] = useState<FixedLimits | null>(null)
  const [saved,setSaved] = useState<FixedLimits | null>(null)
  const [busy,setBusy] = useState(false)
  const [message,setMessage] = useState('')
  const [error,setError] = useState('')
  useEffect(()=>setSaved(null),[policy.workers,policy.tasks,policy.model,policy.ci])
  const current = saved ?? policy
  const value = draft ?? current
  const valid = fixedFields.every(([key])=>Number.isInteger(value[key]) && value[key]>=1 && value[key]<=2147483647)
  const changed = fixedFields.some(([key])=>value[key]!==current[key])
  async function save() {
    if (busy || !valid || !changed) return
    setBusy(true);setError('')
    try {
      await request('/workers/fixed-pool',{method:'PUT',body:JSON.stringify({node,...value})})
      setSaved(value);setDraft(null);setMessage('已保存。Worker 数量将在下一次心跳后调整。')
    } catch(e) {setError(e instanceof Error ? e.message : '保存失败。')}
    finally {setBusy(false)}
  }
  return <section className="worker-capabilities pool-settings">
    <h3>常驻 Worker</h3>
    <p>{online ? '最近心跳' : '离线前最后一次心跳'}上报 {count} 个就绪，目标 {current.workers} 个。任务、模型回合和 CI 名额由这些 Worker 共享。</p>
    <form onSubmit={e=>{e.preventDefault();void save()}}>
      {fixedFields.map(([key,label])=><label key={key}>{label}<input type="number" required min="1" max="2147483647"
        value={value[key] || ''} onChange={e=>{setDraft({...value,[key]:e.target.valueAsNumber});setError('');setMessage('')}}/></label>)}
      <button className="button" disabled={busy || !changed || !valid}>{busy?'正在保存…':'保存常驻配置'}</button>
    </form>
    <p>一个任务调用模型时占用一个模型回合名额，回合结束即释放。同一模型可以同时占用多个名额。</p>
    {changed && !valid && <p role="alert">各项上限须为正整数。</p>}
    {error && <p role="alert">{error}</p>}{message && <p role="status">{message}</p>}
  </section>
}
