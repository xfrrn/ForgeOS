import { useEffect, useState } from 'react'
import { Fact, formatTime, Icon, type ApiRequest } from './shared'
import './runtime-status.css'

type Space = { available: number; total: number }
type Host = { sampled_at: string; cpu_percent?: number; memory?: Space; repositories?: Space; managed?: Space; workspace?: Space }
type CLI = { name: string; status: keyof typeof cliStates; version?: string }
type Tools = { checked_at: string; cli: CLI[]; docker: keyof typeof dockerStates }
export type Diagnostics = { host?: Host; tools?: Tools }
export type WorkerSummary = {
  worker: string; os: string; arch: string; cpus: number; slots: number; reserved: number
  model_slots?: number; model_reserved: number; ci_slots?: number; ci_reserved: number
  last_seen: string; online: boolean
  capabilities?: { diagnostics?: Diagnostics }
}
type ControlPlane = { os: string; arch: string; cpus: number; postgresql: 'reachable' | 'unreachable'; diagnostics: Diagnostics }

const cliNames: Record<string, string> = { codex: 'Codex', claude: 'Claude Code', git: 'Git', gh: 'GitHub CLI', docker: 'Docker',
  node: 'Node.js', npm: 'npm', python: 'Python', go: 'Go' }
const cliStates = { supported: '通过能力检测', responds: '可执行', found: '已找到', not_found: '当前环境未找到', probe_failed: '版本探测失败' }
const dockerStates = { reachable: '可连接', unreachable: '探测未通过', not_found: 'CLI 未找到' }
const giB = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 })

export function useRuntimeStatus<Node extends WorkerSummary>(request: ApiRequest) {
  const [nodes, setNodes] = useState<Node[]>()
  const [control, setControl] = useState<ControlPlane>()
  const [nodeError, setNodeError] = useState(false)
  const [controlError, setControlError] = useState(false)
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      const options = { signal: stop.signal }
      await Promise.allSettled([
        request<Node[]>('/workers', options).then(rows => {
          if (!stop.signal.aborted) { setNodes(rows); setNodeError(false) }
        }, () => { if (!stop.signal.aborted) setNodeError(true) }),
        request<ControlPlane>('/workers/control-plane', options).then(snapshot => {
          if (!stop.signal.aborted) { setControl(snapshot); setControlError(false) }
        }, () => { if (!stop.signal.aborted) setControlError(true) }),
      ])
      if (!stop.signal.aborted) timer = setTimeout(refresh, 15000)
    }
    void refresh()
    return () => { stop.abort(); clearTimeout(timer) }
  }, [request])
  return { nodes, control, nodeError, controlError }
}

function freeSlots(nodes: WorkerSummary[], kind: 'model' | 'ci') {
  let total = 0
  let free = 0
  for (const node of nodes) {
    const slots = kind === 'model' ? node.model_slots : node.ci_slots
    if (!slots) continue
    const reserved = kind === 'model' ? node.model_reserved : node.ci_reserved
    total += Math.min(slots, node.slots)
    free += Math.max(0, Math.min(slots - reserved, node.slots - node.reserved))
  }
  return total ? free + ' / ' + total : '—'
}

export function RuntimeSummary({ request }: { request: ApiRequest }) {
  const { nodes, control, nodeError, controlError } = useRuntimeStatus(request)
  const online = nodes?.filter(node => node.online) ?? []
  const offline = nodes ? nodes.length - online.length : 0
  const full = online.filter(node => node.reserved >= node.slots).length
  const reportedCapacity = online.some(node => node.model_slots != null || node.ci_slots != null)
  const limitedCapacity = reportedCapacity && online.some(node => node.model_slots == null || node.ci_slots == null)
  const lowDisk = (host?: Host) => [host?.repositories, host?.managed, host?.workspace].some(space => space && space.available / space.total < 0.1)
  const controlLowDisk = !controlError && lowDisk(control?.diagnostics.host)
  const workersLowDisk = nodeError ? 0 : online.filter(node => lowDisk(node.capabilities?.diagnostics?.host)).length
  const postgres = controlError ? '读取失败' : control?.postgresql === 'reachable' ? '连通' : control ? '不可用' : '读取中'

  return <section className="panel runtime-summary" aria-labelledby="runtime-heading">
    <div className="runtime-summary-header">
      <h2 id="runtime-heading"><Icon name="Activity" size={17}/>运行状态</h2>
      <a className="overview-link" href="#/workers">节点与 CLI<Icon name="ChevronRight" size={14}/></a>
    </div>
    <dl className="runtime-summary-facts">
      <Fact label="Worker 在线">{nodes ? online.length + ' / ' + nodes.length : '—'}</Fact>
      <Fact label="节点模型空闲">{freeSlots(online, 'model')}</Fact>
      <Fact label="节点 CI 空闲">{freeSlots(online, 'ci')}</Fact>
      <Fact label="控制面 · PostgreSQL"><span className={postgres === '不可用' || controlError ? 'runtime-warning' : ''}>{postgres}</span></Fact>
    </dl>
    {(nodeError || !nodes || !nodes.length || offline > 0 || full > 0 || controlLowDisk || workersLowDisk > 0 || limitedCapacity) && <p className="runtime-summary-notes">
      {nodeError ? <span className="runtime-warning">{nodes ? '节点刷新失败，显示上次结果' : '节点读取失败'}</span>
        : !nodes ? <span role="status">正在读取节点…</span>
        : !nodes.length && <span>尚未收到 Worker 心跳</span>}
      {offline > 0 && <span className="runtime-warning">{offline} 个节点心跳超时</span>}
      {full > 0 && <span className="runtime-warning">{full} 个节点执行名额已满</span>}
      {controlLowDisk && <span className="runtime-warning">控制面仓库盘剩余空间低于 10%</span>}
      {workersLowDisk > 0 && <span className="runtime-warning">{workersLowDisk} 个在线节点的磁盘可用空间低于 10%</span>}
      {limitedCapacity && <span>名额来自已上报容量的在线节点</span>}
    </p>}
  </section>
}

function SpaceFact({ label, space, warn = false }: { label: string; space: Space; warn?: boolean }) {
  return <Fact label={label}>
    <span className={warn && space.available / space.total < 0.1 ? 'runtime-warning' : ''}>
      {giB.format(space.available / 2 ** 30)} / {giB.format(space.total / 2 ** 30)} GiB
    </span>
  </Fact>
}

export function HostDiagnostics({ diagnostics, online = true }: { diagnostics?: Diagnostics; online?: boolean }) {
  const host = diagnostics?.host
  const tools = diagnostics?.tools
  return <>
    <section className="runtime-diagnostics">
      <h3>主机资源</h3>
      {host ? <>
        <p className="muted">{online ? '采样于' : '离线前采样于'} <time dateTime={host.sampled_at}>{formatTime(host.sampled_at)}</time></p>
        <dl className="runtime-resource-facts">
          {host.cpu_percent !== undefined && <Fact label="CPU 使用率">{host.cpu_percent}%</Fact>}
          {host.memory && <SpaceFact label="可用内存" space={host.memory}/>}
          {host.repositories && <SpaceFact label="仓库盘可用" space={host.repositories} warn/>}
          {host.managed && <SpaceFact label="托管仓库盘可用" space={host.managed} warn/>}
          {host.workspace && <SpaceFact label="工作区盘可用" space={host.workspace} warn/>}
        </dl>
        {host.cpu_percent === undefined && !host.memory && !host.repositories && !host.managed && !host.workspace &&
          <p className="muted">当前系统未提供资源指标</p>}
      </> : <p className="muted">暂无资源样本</p>}
    </section>
    <section className="runtime-diagnostics">
      <h3>CLI 与 Docker</h3>
      {tools ? <>
        <p className="muted">{online ? '探测于' : '离线前探测于'} <time dateTime={tools.checked_at}>{formatTime(tools.checked_at)}</time> · 登录状态未检查</p>
        <p className="runtime-docker"><strong>Docker 引擎</strong> <span className={tools.docker === 'unreachable' ? 'runtime-warning' : ''}>{dockerStates[tools.docker]}</span></p>
        <table className="runtime-cli">
          <caption className="sr-only">CLI 探测结果</caption>
          <thead><tr><th scope="col">工具</th><th scope="col">状态</th><th scope="col">版本</th></tr></thead>
          <tbody>{tools.cli.map(cli => <tr key={cli.name}>
            <th scope="row">{cliNames[cli.name] ?? cli.name}</th>
            <td className={cli.status === 'probe_failed' ? 'runtime-warning' : ''}>{cliStates[cli.status] ?? cli.status}</td>
            <td><code>{cli.version ?? '—'}</code></td>
          </tr>)}</tbody>
        </table>
      </> : <p className="muted">暂无 CLI 探测结果</p>}
    </section>
  </>
}
