import { useEffect, useState } from 'react'
import type { ApiRequest } from './shared'

export function PlanningTaskLinks({project,task,request}:{project:number;task:number;request:ApiRequest}){
  const [rows,setRows]=useState<{id:number;milestone:string;delivery_id:number;delivery:string}[]>([])
  const [error,setError]=useState(''),[attempt,setAttempt]=useState(0)
  useEffect(()=>{
    const controller=new AbortController()
    request<typeof rows>('/projects/'+project+'/planning/tasks/'+task,{signal:controller.signal})
      .then(value=>{if(!controller.signal.aborted){setRows(value);setError('')}})
      .catch(e=>{if(!controller.signal.aborted)setError(e.message)})
    return ()=>controller.abort()
  },[project,task,request,attempt])
  if(error)return <p role="alert">读取关联规划失败。 <button className="button" onClick={()=>setAttempt(n=>n+1)}>重试</button></p>
  return rows.length?<nav className="project-secondary-links" aria-label="关联项目规划">{rows.map(r=>
    <a key={r.delivery_id} href={'#/projects/'+project+'/planning?milestone='+r.id+'&item='+r.delivery_id}>{r.milestone} · {r.delivery}</a>)}</nav>:null
}
