import { ThemedSelect } from './ThemedSelect'
import { useEffect, useState } from 'react'
import type { ApiRequest } from './shared'

type Harness = 'codex' | 'claude'
type Catalog = Record<Harness, { id: string; label: string }[]>
const harnesses: Harness[] = ['codex', 'claude']
const names = {codex: 'Codex CLI', claude: 'Claude Code'}

export function ModelSelect({request,harness,value,onChange,label='模型',defaultLabel='跟随 Harness 默认',disabled=false,allowCustom=true}: {
  request: ApiRequest; harness: string; value: string; onChange: (model: string, harness?: Harness)=>void
  label?: string; defaultLabel?: string; disabled?: boolean; allowCustom?: boolean
}) {
  const [catalog,setCatalog]=useState<Catalog>()
  const [custom,setCustom]=useState(false)
  const [error,setError]=useState(false)
  const [reload,setReload]=useState(0)
  useEffect(()=>{
    const controller=new AbortController()
    setError(false)
    request<Catalog>('/projects/agent-models',{signal:controller.signal}).then(data=>{
      if(!controller.signal.aborted) setCatalog(data)
    }).catch(()=>{if(!controller.signal.aborted)setError(true)})
    return ()=>controller.abort()
  },[request,reload])
  const groups=harnesses.filter(h=>harness==='task'||h===harness)
  const found=groups.find(h=>catalog?.[h].some(m=>m.id===value))
  const selected=custom?'custom':value?(found?found+'|'+value:'saved'):''
  return <div className="model-select">
    <ThemedSelect label={label} value={selected} disabled={disabled} options={[
      {value:'',label:defaultLabel},
      ...(value && !found && !custom ? [{value:'saved',label:value}] : []),
      ...groups.flatMap(h=>(catalog?.[h] ?? []).map(m=>({value:h+'|'+m.id,label:m.label,group:names[h]}))),
      ...(allowCustom ? [{value:'custom',label:'输入自定义模型 ID…'}] : [])
    ]} onChange={choice=>{
      setCustom(choice==='custom')
      if(choice==='custom'){onChange(value);return}
      if(choice==='saved')return
      if(!choice){onChange('');return}
      const [h,id]=choice.split('|')
      onChange(id,h as Harness)
    }}/>
    {custom && <label>自定义模型 ID<input value={value} disabled={disabled} required maxLength={128}
      autoComplete="off" spellCheck={false} onChange={e=>onChange(e.target.value)}/></label>}
    {error && <span role="alert">模型列表读取失败。 <button type="button" className="button" onClick={()=>setReload(n=>n+1)}>重试</button></span>}
    {catalog && harness==='codex' && !catalog.codex.length && <small>本机尚无 Codex 模型目录。</small>}
  </div>
}
