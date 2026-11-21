import { useEffect, useState } from 'react'
import { ManagementProcesses, type ReviewProcess } from './ManagementProcesses'
import { ApiError, formatTime, projectHref, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'
import { PageHeading } from './ui'
import { MaterialReader } from './MaterialReader'
import { KnowledgeBrowser, KnowledgeSearch, LibraryNotice, useKnowledgeDetail, useKnowledgeList } from './KnowledgeBrowser'
import { ShareEditor, type SharedCopy, type ShareRevision } from './ShareDraft'

type Receipt = ShareRevision & {
  review_processes?:ReviewProcess[]
  review?:{verdict:'approve'|'revise'|'needs_owner'; rationale:string; evidence:string; effective_level:string; session_id:string; profile?:StructuredProfile}
}
const verdicts={approve:'已通过审核，待发布',revise:'需要修改副本',needs_owner:'需要 Owner 决定'}
function status(row:Receipt) {
  if (row.revoked_at) return '已撤销'
  if (row.superseded_at) return '已替代'
  if (row.published_at) return '已发布'
  if (row.stale) return '发布状态已变化'
  return row.review?verdicts[row.review.verdict]:'待审核'
}

export function ProjectShares({project,receipt,received,request}:{project:number;receipt?:number;received:boolean;request:ApiRequest}) {
  const base='/projects/'+project
  const section=projectHref(project,'/knowledge/shares/')
  const href=section+(received?'received':'sent')
  const [creating,setCreating]=useState(false)
  const [query,setQuery]=useState(''),[text,setText]=useState('')
  const [revision,setRevision]=useState(0)
  const refresh=()=>setRevision(r=>r+1)
  useEffect(()=>{
    if (!received) return
    const update=()=>{if (document.visibilityState==='visible') setRevision(r=>r+1)}
    window.addEventListener('focus',update)
    document.addEventListener('visibilitychange',update)
    return ()=>{window.removeEventListener('focus',update);document.removeEventListener('visibilitychange',update)}
  },[received])
  const path=received ? base+'/shared-experience'+(query?'/search?versions=1&q='+encodeURIComponent(query):'?versions=1') :
    base+'/shares?latest=1'+(query?'&q='+encodeURIComponent(query):'')
  const list=useKnowledgeList<Receipt|SharedCopy>(request,path,revision,!received||!query)
  const first=list.rows[0]
  const selected=receipt??(received?first?.series_id:first?.id)
  const detailPath=selected ? base+(received?'/shared-experience/':'/shares/')+selected : undefined
  const detail=useKnowledgeDetail<Receipt|SharedCopy>(request,detailPath,revision)
  function back(){setCreating(false);location.hash=href}
  function saved(row:SharedCopy){setCreating(false);location.hash=section+'sent/'+row.id;refresh()}
  return <div className="project-setup">
    <PageHeading title="共享经验" actions={!received && <button className="button primary" onClick={()=>setCreating(true)}>起草脱敏副本</button>}/>
    <nav className="knowledge-direction" aria-label="共享方向">
      <a className="button" aria-current={!received?'page':undefined} href={section+'sent'}>我发出的</a>
      <a className="button" aria-current={received?'page':undefined} href={section+'received'}>我收到的</a>
    </nav>
    <KnowledgeBrowser detailActive={Boolean(receipt||creating)} back={back}
      toolbar={<KnowledgeSearch query={text} setQuery={setText} search={q=>{setQuery(q);back()}} refresh={refresh}
        placeholder={received?'搜索收到的共享经验':'搜索发出的副本'}/>}
      catalog={<>{list.rows.map(row=>{
        const id=received?row.series_id:row.id
        return <a href={href+'/'+id} className="knowledge-row" key={row.id} aria-current={selected===id && !creating?'true':undefined}
          onClick={()=>setCreating(false)}><strong>{row.title}</strong>
          <small className="knowledge-row-meta">v{row.share_version} · {'project_id' in row?status(row)+' · 项目 #'+row.recipient:'已发布'}</small>
          <small className="knowledge-meta">{formatTime(('project_id' in row?row.superseded_at||row.revoked_at||row.published_at||row.proposed_at:row.published_at)??'')}</small></a>
      })}<LibraryNotice {...list} retry={refresh} empty={list.rows.length?'':query?'没有匹配的共享经验。':received?'尚无发布给本项目的共享经验。':'尚无发出的副本。'}/></>}>
      {creating ? <ShareEditor project={project} request={request} onCancel={()=>setCreating(false)} onSaved={saved}/> :
        detail.data ? 'project_id' in detail.data ? <SentDetails key={detail.data.id} row={detail.data} project={project} request={request} revision={revision} refresh={refresh} saved={saved}/> :
          <ReceivedDetails key={detail.data.id} project={project} row={detail.data}/> :
          <LibraryNotice loading={detail.loading} error={detail.error} empty="从列表选择共享经验，查看副本。" retry={refresh}/>}
    </KnowledgeBrowser>
  </div>
}

function ReceivedDetails({project,row}:{project:number;row:SharedCopy}) {
  const [copy,setCopy]=useState('')
  const href=projectHref(project,'/knowledge/shares/received/'+row.series_id)
  // This deep link resolves the current released copy through the recipient's own project.
  const link=new URL(href,window.location.href).href
  const ref=row.id+':'+row.digest
  async function copyValue(value:string,label:string) {
    try{await navigator.clipboard.writeText(value);setCopy(label+'已复制。')}catch{setCopy(value)}
  }
  return <article><header><p className="knowledge-meta">收到的共享经验 · v{row.share_version} · 副本 #{row.id}</p><h2>{row.title}</h2></header>
    <MaterialReader markdown content={row.content}/>
    <p>发布时间 · {row.published_at && formatTime(row.published_at)}</p>
    <div className="knowledge-actions"><button className="button" onClick={()=>void copyValue(ref,'版本引用')}>复制本版引用</button>
      <button className="button" onClick={()=>void copyValue(link,'条目链接')}>复制条目链接</button>{copy&&<span role="status">{copy}</span>}</div>
    <details><summary>CLI 引用与副本摘要</summary><p>来源类型 · <code>shared_experience</code></p><p>版本引用 · <code>{ref}</code></p><p>SHA-256 · <code>{row.digest}</code></p></details>
  </article>
}

function SentDetails({project,row,request,revision,refresh,saved}:{
  project:number;row:Receipt;request:ApiRequest;revision:number;refresh:()=>void;saved:(row:SharedCopy)=>void
}) {
  const [editing,setEditing]=useState(false),[revoking,setRevoking]=useState(false)
  const [busy,setBusy]=useState(false),[message,setMessage]=useState(''),[error,setError]=useState('')
  const [showHistory,setShowHistory]=useState(false)
  const base='/projects/'+project+'/shares/'
  const current=useKnowledgeDetail<Receipt>(request,editing && row.current_share_id && row.current_share_id!==row.id?base+row.current_share_id:undefined,revision)
  async function publish(action:'publish'|'revoke') {
    setBusy(true);setError('');setMessage('')
    try {
      await request<Receipt>(base+row.id+'/publication',{method:'POST',body:JSON.stringify({action,digest:row.digest})})
      setMessage(action==='publish'?'已向接收项目发布该版本。':'已撤销副本。');setRevoking(false);refresh()
    } catch(err) {
      setError(err instanceof ApiError && err.status===409?'发布状态或审核结果已变化，请核对当前版本。':err instanceof Error?err.message:'操作失败。')
      refresh()
    } finally {setBusy(false)}
  }
  return <article><header><p className="knowledge-meta">发给项目 #{row.recipient} · v{row.share_version} · {status(row)}</p><h2>{row.title}</h2></header>
    <MaterialReader markdown content={row.content}/>
    <p>来源 · {row.experience_id?<a href={projectHref(project,'/knowledge/experience/'+row.experience_id)}>项目经验 #{row.experience_id} · 修订 {row.experience_revision}</a>:
      <a href={projectHref(project,'/materials?key='+encodeURIComponent(row.material_key??'')+'&revision='+row.revision)}>{row.material_key} · 修订 {row.revision}</a>}</p>
    <p>提交时间 · {formatTime(row.proposed_at)}{row.published_at && '　发布时间 · '+formatTime(row.published_at)}</p>
    {row.current_share_id && row.current_share_id!==row.id && <p><a href={projectHref(project,'/knowledge/shares/sent/'+row.current_share_id)}>查看当前发布副本 #{row.current_share_id}</a></p>}
    {row.review && <details><summary>审核依据 · {row.review.effective_level}</summary><p>{row.review.rationale}</p>
      <p>证据 · {row.review.evidence}</p><p>{row.review.session_id.startsWith('api-')?'模型 API 调用':'原生会话'} · <code>{row.review.session_id}</code></p>
      {row.review.profile && <RolePermissions role="Management" profile={row.review.profile}/>}</details>}
    <ManagementProcesses request={request} processes={row.review_processes}/>
    <div className="knowledge-actions">{!editing&&<button className="button" onClick={()=>setEditing(true)}>起草新版</button>}
      {row.review?.verdict==='approve' && !row.published_at && !row.revoked_at && !row.stale &&
        <button className="button primary" disabled={busy} onClick={()=>void publish('publish')}>发布给项目 #{row.recipient}</button>}
      {!row.revoked_at && !row.superseded_at && <button className="button" disabled={busy} onClick={()=>setRevoking(!revoking)}>撤销副本</button>}
    </div>
    {revoking && <div className="knowledge-section"><p>{row.published_at?'撤销后，接收项目无法再读取或采用这个版本。':'撤销后，这份草稿将无法发布。'}</p>
      <div className="knowledge-actions"><button className="button danger" disabled={busy} onClick={()=>void publish('revoke')}>确认撤销</button>
        <button className="button" onClick={()=>setRevoking(false)}>取消</button></div></div>}
    {error&&<p role="alert">{error}</p>}{message&&<p role="status">{message}</p>}
    {editing && current.error && <p role="alert">当前发布版读取失败。 {current.error} <button className="button" onClick={refresh}>重试</button></p>}
    {editing && <ShareEditor project={project} row={row} current={row.current_share_id===row.id?row:current.data} request={request}
      onCancel={()=>setEditing(false)} onConflict={refresh} onSaved={item=>{setEditing(false);saved(item)}}/>}
    <details onToggle={e=>setShowHistory(e.currentTarget.open)}><summary>版本与发布记录</summary>
      <p>本版摘要 · <code>{row.digest}</code></p>
      {showHistory && <ShareHistory project={project} row={row} request={request} revision={revision}/>}
    </details>
  </article>
}

function ShareHistory({project,row,request,revision}:{project:number;row:Receipt;request:ApiRequest;revision:number}) {
  const [tick,setTick]=useState(0)
  const list=useKnowledgeList<Receipt>(request,'/projects/'+project+'/shares/'+row.id+'/versions',revision+tick)
  return <div>{list.rows.map(item=><a className="knowledge-row" href={projectHref(project,'/knowledge/shares/sent/'+item.id)} key={item.id}>
    <strong>v{item.share_version} · {item.title}</strong><small>{status(item)} · {formatTime(item.proposed_at)}</small>
  </a>)}<LibraryNotice {...list} empty="" retry={()=>setTick(n=>n+1)}/></div>
}
