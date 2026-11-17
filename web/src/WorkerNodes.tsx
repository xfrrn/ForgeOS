import { useEffect, useState } from 'react'
import { formatTime, type ApiRequest } from './shared'

type Node = { worker: string; os: string; arch: string; cpus: number; slots: number; reserved: number
  model_slots?: number; model_reserved: number; ci_slots?: number; ci_reserved: number
  last_seen: string; online: boolean; capabilities: Record<string, boolean> }

type ModelBudget = { name: string; harness: string; slots: number; reserved: number; workers: string[]; reviewer_projects?: number[]; test_plan_roles?: {project_id: number; role: 'test' | 'management'}[] }

const capabilities: Record<string, string> = { supports_startup: '本地环境启动', supports_codex: 'Codex', supports_claude: 'Claude Code', supports_planner: '规划', supports_global_rules: '全局默认规则',
  supports_checkpoints: '检查点', supports_rework: '退回修订', supports_validation: '候选验证', supports_declared_checks: '项目检查', supports_risk_policy: '按风险选级' }

export function WorkerNodes({ request }: { request: ApiRequest }) {
  const [nodes, setNodes] = useState<Node[]>([])
  const [budgets, setBudgets] = useState<ModelBudget[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const [rows, accounts] = await Promise.all([
          request<Node[]>('/workers', { signal: stop.signal }),
          request<ModelBudget[]>('/workers/model-budgets', { signal: stop.signal }),
        ])
        if (!stop.signal.aborted) { setNodes(rows); setBudgets(accounts); setError(''); setLoaded(true) }
      } catch (e) { if (!stop.signal.aborted) setError(e instanceof Error ? e.message : '节点读取失败。') }
      if (!stop.signal.aborted) timer = setTimeout(refresh, 5000)
    }
    void refresh()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request])
  return <section className="panel detail-card"><h1>执行节点</h1>
    {error && <p role="alert">{error}</p>}
    {budgets.length > 0 && <section><h2>共享模型名额</h2>{budgets.map(b =>
      <p key={b.name}>{b.name} · {b.harness} · 已占 {b.reserved} / {b.slots} · {[...b.workers, ...(b.reviewer_projects ?? []).map(id => `项目 ${id} 的 Reviewer`),
        ...(b.test_plan_roles ?? []).map(m => `项目 ${m.project_id} ${m.role === 'test' ? '的 Test Agent' : '的覆盖审核'}`)].join('、')}</p>)}</section>}
    {!error && !nodes.length && <p>{loaded ? '尚未收到 Worker 心跳。' : '正在读取节点…'}</p>}
    {nodes.map(n => <article key={n.worker}><h2>{n.worker}</h2><p>{n.online ? '在线' : '心跳超时'} · {n.os} / {n.arch} · {n.cpus} 个逻辑 CPU</p>
      <p>已占名额 {n.reserved} / {n.slots}</p><p>最近心跳　<time dateTime={n.last_seen}>{formatTime(n.last_seen)}</time></p>
      {n.model_slots && n.ci_slots && <p>模型名额 {n.model_reserved} / {n.model_slots} · CI 名额 {n.ci_reserved} / {n.ci_slots}</p>}
      <p>{['Git HEAD', ...Object.entries(capabilities).filter(([key]) => n.capabilities[key]).map(([, label]) => label)].join(' · ')}</p>
    </article>)}</section>
}
