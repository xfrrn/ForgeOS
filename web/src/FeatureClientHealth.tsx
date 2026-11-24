import { useEffect, useRef, useState } from 'react'
import { formatTime, Icon, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'

type ModelOption = {id:string; label?:string}
type ClientHealth = {id:string; backend:string; model:string; node:string; instance:string; status:string; checked_at?:string; pending:boolean; available_models?:ModelOption[]}
const labels: Record<string,string> = {
  ready:'已连接', auth:'AionCore 登录凭据失效', unreachable:'无法连接本机 AionCore',
  timeout:'节点探测超时', disabled:'客户端已禁用', not_installed:'节点未发现此客户端',
  stale:'节点报告已过期', removed:'节点配置中已移除', configuration:'节点配置无法读取',
}

export function FeatureClientHealth({projectId,request,selectedModel,selectedVariant,onSelectModel}:{
  projectId:number; request:ApiRequest; selectedModel?:string; selectedVariant?:string; onSelectModel?:(id:string, model?:string)=>void
}) {
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
        const ready=rows.filter(c=>c.status==='ready').map(c=>c.id+':'+(c.available_models?.map(m=>m.id).join('|')||c.model)).sort().join(',')
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
  const readyCount=clients?.filter(c=>c.status==='ready').length
  return <details className="feature-clients">
    <summary>
      <span className="feature-clients-summary-label">
        <span className={'feature-client-dot' + (readyCount === 0 ? ' down' : readyCount === undefined ? ' warn' : '')} aria-hidden="true"/>
        <span>本机客户端 · {readyCount ?? '…'} 可用</span>
      </span>
      <Icon name="ChevronDown" size={14} className="feature-clients-chevron"/>
    </summary>
    <div className="feature-client-health">
      {visible?.map(c=>{
        const isReady=c.status==='ready'
        const isSelected=selectedModel===c.id
        const backendLabel=c.backend==='claude'?'Claude Code':'Codex'
        const choices=c.available_models?.length ? c.available_models : [{id:c.model,label:c.model}]
        const activeChoice=isSelected && selectedVariant && choices.some(m=>m.id===selectedVariant) ? selectedVariant : c.model
        return <div key={c.id} className={'feature-client-row' + (isSelected ? ' selected' : '')}>
          <div className="feature-client-row-main">
            <button type="button" className="feature-client-pick" disabled={!isReady || !onSelectModel}
              title={isReady ? `切换到 ${backendLabel}（${c.node} · ${c.instance}）` : `${labels[c.status] || '状态未知'}（${c.node}）`}
              onClick={()=>{ if (isReady && onSelectModel) onSelectModel(c.id, activeChoice) }}>
              <span className="feature-client-title">
                <span className={'feature-client-dot' + (isReady ? '' : c.status==='stale' ? ' warn' : ' down')} aria-hidden="true"/>
                <span>{choices.length > 1 ? backendLabel : `${backendLabel} · ${c.model}`}</span>
                {isSelected && <span className="feature-client-active">当前</span>}
              </span>
              <span className={'feature-client-meta' + (!isReady ? ' is-error' : '')}>
                {isReady ? (c.checked_at ? `已连接 · ${formatTime(c.checked_at)}` : '已连接') : (labels[c.status] || '状态未知')}
              </span>
            </button>
            <button type="button" className="icon-button feature-client-recheck" disabled={!!busy || c.pending}
              title={c.pending ? '等待节点重新检测' : '重新检测客户端及可用模型'} aria-label={c.pending ? '等待节点重新检测' : `重新检测 ${backendLabel}`}
              onClick={()=>void recheck(c)}>
              <Icon name="RefreshCw" size={13}/>
            </button>
          </div>
          {isReady && choices.length > 1 && onSelectModel && <div className="feature-client-model-select">
            <ThemedSelect label={`${backendLabel} 模型`} hideLabel value={activeChoice}
              options={choices.map(m => ({value: m.id, label: m.label || m.id}))}
              onChange={nextModel => onSelectModel(c.id, nextModel)}/>
          </div>}
        </div>
      })}
      {clients?.some(c=>c.status==='removed') && <button type="button" className="button feature-clients-toggle" onClick={()=>setShowRemoved(!showRemoved)}>{showRemoved?'隐藏已移除':'查看已移除'}</button>}
      {visible?.length===0 && <p className="feature-empty-hint">尚无节点客户端报告。</p>}
      {error && <p role="alert">{error} <button className="button" onClick={()=>setRevision(n=>n+1)}>重新读取</button></p>}
      {actionError && <p role="alert">{actionError}</p>}
    </div>
  </details>
}
