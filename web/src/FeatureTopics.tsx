import { useEffect, useRef, useState } from 'react'
import { projectHref, formatTime, type ApiRequest } from './shared'
import { ProjectChat } from './ProjectChat'
import { useDiscussionModels } from './ProjectChatProvider'
import { ThemedSelect } from './ThemedSelect'
import './feature-topics.css'

type Topic = {id:number; title:string; profile_id:string; updated_at:string}

export function FeatureTopics({projectId,selected,request}:{projectId:number;selected?:number;request:ApiRequest}) {
  const path = '/projects/'+projectId+'/features'
  const href = projectHref(projectId,'/features')
  const [topics,setTopics] = useState<Topic[]>()
  const [creating,setCreating] = useState(false)
  const [title,setTitle] = useState('')
  const [model,setModel] = useState('')
  const [error,setError] = useState('')
  const [busy,setBusy] = useState(false)
  const [refresh,setRefresh] = useState(0)
  const intent = useRef({key:'',id:''})
  const {models,error:modelError} = useDiscussionModels(request,projectId)
  const profile = model || models?.[0]?.id || ''
  const topic = topics?.find(item=>item.id===selected)
  useEffect(()=>{
    const controller = new AbortController()
    setError('')
    request<Topic[]>(path,{signal:controller.signal}).then(items=>{
      if (!controller.signal.aborted) setTopics(items)
    }).catch(e=>{if (!controller.signal.aborted) setError(e.message)})
    return ()=>controller.abort()
  },[path,selected,refresh,request])
  async function create() {
    if (!title.trim() || !profile || busy) return
    const key = JSON.stringify([title.trim(),profile])
    if (intent.current.key!==key) intent.current={key,id:crypto.randomUUID()}
    setBusy(true);setError('')
    try {
      const item = await request<Topic>(path,{method:'POST',body:JSON.stringify({title:title.trim(),profile_id:profile,client_id:intent.current.id})})
      setTopics(old=>[item,...(old||[]).filter(t=>t.id!==item.id)])
      setCreating(false);setTitle('');intent.current={key:'',id:''}
      window.location.hash=href+'/'+item.id
    } catch(e) {setError(e instanceof Error?e.message:'创建话题失败。')}
    finally {setBusy(false)}
  }
  return <div className={'feature-room'+(selected||creating?' feature-open':'')}>
    <aside className="feature-list panel" aria-label="功能话题">
      <div className="feature-list-head"><h2>功能话题</h2><button className="button" onClick={()=>setCreating(true)}>新建话题</button></div>
      {topics?.map(item=><a key={item.id} className="feature-link" aria-current={item.id===selected?'page':undefined} href={href+'/'+item.id} onClick={()=>setCreating(false)}>{item.title}<small>{formatTime(item.updated_at)}</small></a>)}
      {topics?.length===0 && <p>还没有功能话题。</p>}
      <details className="feature-clients"><summary>本机讨论客户端</summary>
        {(models || []).filter(m=>m.output_format==='native').map(client=><div key={client.id}><strong>{client.name}</strong><small>{client.base_url} · 节点已连接</small></div>)}
        {!models?.some(m=>m.output_format==='native') && <p>尚未收到可用客户端的节点报告。</p>}
      </details>
    </aside>
    <section className="feature-detail">
      {error && <p role="alert">{error} <button className="button" onClick={()=>setRefresh(r=>r+1)}>重新读取</button></p>}
      {creating ? <form className="panel feature-form" onSubmit={e=>{e.preventDefault();void create()}}>
        <h2>新建功能话题</h2>
        <label>功能名称<input required maxLength={120} value={title} onChange={e=>setTitle(e.target.value)}/></label>
        <ThemedSelect label="讨论模型" value={profile} options={(models||[]).map(m=>({value:m.id,label:m.name || m.model}))} onChange={setModel}/>
        {modelError && <p role="alert">{modelError}</p>}
        {models?.length===0 && <p>请先在账号设置中添加模型连接。</p>}
        <div><button className="button primary" disabled={busy||!profile||!title.trim()}>创建并开始讨论</button> <button className="button" type="button" disabled={busy} onClick={()=>setCreating(false)}>取消</button></div>
      </form> : topic ? <><a className="feature-back" href={href}>返回功能话题</a><ProjectChat key={topic.id} projectId={projectId} request={request} feature={topic}/></> :
        <p role="status">{!topics?'正在读取功能话题…':selected?'此项目中找不到这个话题。':'选择一个话题继续讨论，或新建功能话题。'}</p>}
    </section>
  </div>
}
