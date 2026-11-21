import { useEffect, useRef, useState, type ReactNode } from 'react'
import { type ApiRequest, Icon } from './shared'
import './knowledge-library.css'

export function useKnowledgeList<T extends {id:number}>(request: ApiRequest, path: string, revision: number, paginated = true) {
  const [cursor, setCursor] = useState({path, before:0, revision})
  const before = cursor.path === path && cursor.revision === revision ? cursor.before : 0
  const [result, setResult] = useState<{path:string; revision:number; rows:T[]; more:boolean; loading:boolean; error:string}>()
  useEffect(() => {
    const c = new AbortController()
    setResult(old => ({path,revision,rows:old?.path === path ? old.rows : [],more:false,loading:true,error:''}))
    const suffix = before ? (path.includes('?') ? '&' : '?') + 'before=' + before : ''
    request<T[]>(path + suffix, {signal:c.signal}).then(items => {
      if (c.signal.aborted) return
      setResult(old => ({path,revision,loading:false,error:'',more:paginated && items.length === 30,
        rows:before && old?.path === path ? [...old.rows, ...items.filter(i=>!old.rows.some(o=>o.id===i.id))] : items}))
    }).catch(err => { if (!c.signal.aborted) setResult(old => ({
      path,revision,rows:before && old?.path === path ? old.rows : [],more:false,loading:false,error:err.message,
    })) })
    return () => c.abort()
  }, [request,path,revision,before,paginated])
  const data = result?.path === path ? result : {rows:[] as T[],more:false,loading:true,error:''}
  return {...data, next:() => {const last = data.rows.at(-1); if (last) setCursor({path,revision,before:last.id})}}
}

export function useKnowledgeDetail<T>(request:ApiRequest, path:string | undefined, revision:number) {
  const [result, setResult] = useState<{path:string; data?:T; error?:string; revision:number}>()
  useEffect(() => {
    if (!path) return
    const c = new AbortController()
    request<T>(path,{signal:c.signal}).then(data => {if (!c.signal.aborted) setResult({path,revision,data})})
      .catch(err => {if (!c.signal.aborted) setResult({path,revision,error:err.message})})
    return () => c.abort()
  },[request,path,revision])
  return path && result?.path === path ? {data:result.data,error:result.error,loading:result.revision !== revision} : {loading:Boolean(path)}
}

export function KnowledgeSearch({query,setQuery,search,refresh,filter,placeholder}:{
  query:string; setQuery:(q:string)=>void; search:(q:string)=>void; refresh:()=>void; filter?:ReactNode; placeholder:string
}) {
  return <div className="knowledge-tools">{filter}
    <form role="search" onSubmit={e=>{e.preventDefault(); search(query.trim())}}>
      <input aria-label={placeholder} type="search" value={query} placeholder={placeholder} maxLength={512} onChange={e=>setQuery(e.target.value)}/>
      <button className="button" type="submit" aria-label="检索"><Icon name="Search" size={16}/></button>
    </form>
    <button className="button" type="button" onClick={refresh}><Icon name="RefreshCw" size={15}/>刷新</button>
  </div>
}

export function KnowledgeBrowser({detailActive,toolbar,catalog,children,back}:{
  detailActive:boolean; toolbar:ReactNode; catalog:ReactNode; children:ReactNode; back:()=>void
}) {
  const container=useRef<HTMLDivElement>(null)
  const previous=useRef(detailActive)
  useEffect(()=>{
    if (previous.current!==detailActive && matchMedia('(max-width: 700px)').matches) {
      const target=detailActive?'.knowledge-back':'.knowledge-row[aria-current="true"], .knowledge-tools input'
      container.current?.querySelector<HTMLElement>(target)?.focus({preventScroll:true})
    }
    previous.current=detailActive
  },[detailActive])
  return <div ref={container} className={'panel knowledge-browser'+(detailActive?' knowledge-reading':'')}>
    {toolbar}<div className="knowledge-columns"><aside aria-label="条目列表" className="knowledge-catalog">{catalog}</aside>
      <section aria-label="条目详情" className="knowledge-detail">
        <button className="button knowledge-back" type="button" onClick={back}>返回列表</button>
        {children}
      </section>
    </div>
  </div>
}

export function KnowledgeComparison({summary,previous,proposed,from,to}:{
  summary:string; previous:string; proposed:string; from:string; to:string
}) {
  const [open,setOpen]=useState(false)
  return <details onToggle={event=>setOpen(event.currentTarget.open)}><summary>{summary}</summary>
    {open && <LineChanges previous={previous} proposed={proposed} from={from} to={to}/>}
  </details>
}

function LineChanges({previous,proposed,from,to}:{previous:string;proposed:string;from:string;to:string}) {
  const oldLines=previous.split(/\r?\n/), newLines=proposed.split(/\r?\n/)
  let first=0, oldEnd=oldLines.length, newEnd=newLines.length
  while (first<oldEnd && first<newEnd && oldLines[first]===newLines[first]) first++
  while (oldEnd>first && newEnd>first && oldLines[oldEnd-1]===newLines[newEnd-1]) {oldEnd--;newEnd--}
  if (oldEnd===first && newEnd===first) return <p>两个版本的内容相同。</p>
  return <div className="knowledge-comparison">
    <section><h4>{from}</h4>{first<oldEnd?<pre><del>{oldLines.slice(first,oldEnd).join('\n')}</del></pre>:<p>此处为空</p>}</section>
    <section><h4>{to}</h4>{first<newEnd?<pre><ins>{newLines.slice(first,newEnd).join('\n')}</ins></pre>:<p>此处为空</p>}</section>
  </div>
}

export function LibraryNotice({loading,error,empty,more,next,retry}:{
  loading:boolean; error?:string; empty:string; more?:boolean; next?:()=>void; retry:()=>void
}) {
  if (error) return <p role="alert">{error} <button className="button" onClick={retry}>重试</button></p>
  if (loading) return <p role="status">正在读取…</p>
  if (empty) return <p className="muted">{empty}</p>
  if (more) return <button className="button" onClick={next}>加载更多</button>
  return null
}
