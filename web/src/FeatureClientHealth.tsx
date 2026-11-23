import { useEffect, useRef, useState } from 'react'
import { formatTime, type ApiRequest } from './shared'

type ClientHealth = {id:string; backend:string; model:string; node:string; instance:string; status:string; checked_at?:string; pending:boolean}
const labels: Record<string,string> = {
  ready:'已连接', auth:'AionCore 登录凭据失效', unreachable:'无法连接本机 AionCore',
  timeout:'节点探测超时', disabled:'客户端已禁用', not_installed:'节点未发现此客户端',
  stale:'节点报告已过期', removed:'节点配置中已移除', configuration:'节点配置无法读取',
}

export function FeatureClientHealth({projectId,request}:{projectId:number;request:ApiRequest}) {
  const base='/projects/'+projectId+'/features/clients'
  const availability=useRef('')
  const [clients,setClients]=useState<ClientHealth[]>()
  const [revision,setRevision]=useState(0)
  const [busy,setBusy]=useState('')
  const [error,setError]=useState('')
  const [showRemoved,setShowRemoved]=useState(false)
  const [actionError,setActionError]=useState('')
  useEffect(()=>{
    const controller=new AbortController()
    let timer:number
    async function read(){
      let delay=15000
      try{
        const rows=await request<ClientHealth[]>(base+'/status',{signal:controller.signal})
        if(controller.signal.aborted)return
        const ready=rows.filter(c=>c.status==='ready').map(c=>c.id).sort().join(',')
        if(ready!==availability.current){availability.current=ready;window.dispatchEvent(new Event('forgeos-discussion-clients'))}
        setClients(rows);setError('')
        if(rows.some(c=>c.pending))delay=2000
      }catch(e){if(!controller.signal.aborted)setError(e instanceof Error?e.message:'无法读取节点状态')}
      finally{if(!controller.signal.aborted)timer=window.setTimeout(read,delay)}
    }
    void read()
    return ()=>{controller.abort();window.clearTimeout(timer)}
  },[base,request,revision])
  async function recheck(client:ClientHealth){
    setBusy(client.id);setActionError('')
    try{
      await request(base+'/'+client.id+'/recheck',{method:'POST'})
      setClients(old=>old?.map(c=>c.node===client.node?{...c,pending:true}:c))
      setRevision(n=>n+1)
    }catch(e){setActionError(e instanceof Error?e.message:'重新检测失败')}
    finally{setBusy('')}
  }
  const visible=clients?.filter(c=>showRemoved || c.status!=='removed')
  return <details className="feature-clients" open>
    <summary>本机讨论客户端 · {clients?.filter(c=>c.status==='ready').length ?? '…'} 个可用</summary>
    <div className="feature-client-health">
      {visible?.map(c=><div key={c.id} className="feature-client-row">
        <strong>{c.backend==='claude'?'Claude Code':'Codex'} · {c.model}</strong>
        <small>{c.node} · {c.instance}</small><span>{labels[c.status] || '状态未知'}</span>
        {c.checked_at && <small>上次检测 {formatTime(c.checked_at)}</small>}
        <button type="button" className="button" disabled={!!busy || c.pending} onClick={()=>void recheck(c)}>{c.pending?'等待节点重新检测':'重新检测'}</button>
      </div>)}
      {clients?.some(c=>c.status==='removed') && <button type="button" className="button" onClick={()=>setShowRemoved(!showRemoved)}>{showRemoved?'隐藏已移除':'查看已移除'}</button>}
      {visible?.length===0 && <p className="feature-empty-hint">尚无节点客户端报告。</p>}
      {error && <p role="alert">{error} <button className="button" onClick={()=>setRevision(n=>n+1)}>重新读取</button></p>}
      {actionError && <p role="alert">{actionError}</p>}
    </div>
  </details>
}
