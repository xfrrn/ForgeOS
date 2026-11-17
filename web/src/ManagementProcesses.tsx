import { Fact, formatTime } from './shared'

export type ReviewProcess = { attempt: number; node: string; pid: number; version: string; session_id?: string; started_at: string; exited_at?: string; exit_code: number | null; observed_at: string; output?: { sequence: number; text: string } }

export function ManagementProcesses({processes}: {processes?: ReviewProcess[]}) {
  return processes?.length ? <section><h3>Management 审核进程</h3>
    {processes.map(p => <details key={p.attempt}><summary>{p.node} · PID {p.pid} · {p.exited_at ? '已退出' : '最近观察，尚无退出回执'}</summary>
      <dl><Fact label="原生版本" code>{p.version}</Fact><Fact label="原生会话" code>{p.session_id || '尚未确认'}</Fact>
        <Fact label="开始时间">{formatTime(p.started_at)}</Fact><Fact label="最近上报">{formatTime(p.observed_at)}</Fact>
        {p.exited_at && <Fact label="退出">{formatTime(p.exited_at)} · {p.exit_code}</Fact>}</dl>
      {p.output && <><h4>模型公开消息</h4><p>最近 8 KiB，经脱敏后保存。</p><pre className="material-content">{p.output.text}</pre></>}
    </details>)}
  </section> : null
}
