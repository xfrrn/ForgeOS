import { Fact, formatTime, type ApiRequest } from './shared'
import { ArchiveFacts, ArchiveDownload, type ProcessArchive } from './ProcessArchive'

export type ReviewProcess = { archive?: ProcessArchive; attempt: number; instance: number; resumed_from?: number; missing_session?: boolean; handoff_from?: string; node: string; pid: number; version: string; session_id?: string; started_at: string; exited_at?: string; exit_code: number | null; observed_at: string; output?: { sequence: number; text: string } }

export function ManagementProcesses({processes, request, title = "Management 审核进程"}: {processes?: ReviewProcess[]; request: ApiRequest; title?: string}) {
  return processes?.length ? <section><h3>{title}</h3>
    {processes.map(p => <details key={p.attempt}><summary>{p.node} · PID {p.pid} · {p.exited_at ? '已退出' : '最近观察，尚无退出回执'}</summary>
      <dl><Fact label="节点进程实例">{p.instance}</Fact>{p.resumed_from && <Fact label="接续自实例">{p.resumed_from}</Fact>}
        {p.handoff_from && <Fact label="会话交接来源" code>{p.handoff_from}</Fact>}
        {p.missing_session && <Fact label="原生恢复">会话不存在，保留此次失败记录</Fact>}
        <Fact label="原生版本" code>{p.version}</Fact><Fact label="原生会话" code>{p.session_id || '尚未确认'}</Fact>
        <Fact label="开始时间">{formatTime(p.started_at)}</Fact><Fact label="最近上报">{formatTime(p.observed_at)}</Fact>
        {p.exited_at && <Fact label="退出">{formatTime(p.exited_at)} · {p.exit_code === -1 ? "未取得正常退出码" : p.exit_code}</Fact>}
        {p.archive && <ArchiveFacts instance={p.instance} archive={p.archive}/>}</dl>
      {p.archive?.central && <ArchiveDownload request={request} path={`/role-processes/${p.attempt}/instances/${p.instance}/archive`} file={`role-process-${p.attempt}-process-${p.instance}.jsonl`}/>}
      {p.output && <><h4>模型公开消息</h4><p>最近 8 KiB，经脱敏后保存。</p><pre className="material-content">{p.output.text}</pre></>}
    </details>)}
  </section> : null
}
