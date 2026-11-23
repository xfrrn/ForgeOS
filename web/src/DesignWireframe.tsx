import { useState } from 'react'
import { ThemedSelect } from './ThemedSelect'
import type { ChatModel } from './ProjectChatProvider'
import type { Post } from './FeatureProposals'

export type WireElement = {id:string;kind:'input'|'button'|'text'|'list'|'status'|'tabs';label:string;value?:string}
export type Wireframe = {title:string;regions:{layout?:'row'|'column';position:'header'|'sidebar'|'main'|'footer';label:string;elements:WireElement[]}[]}

function contents(e:WireElement){
  const lines=e.value?.split('\n').filter(Boolean)||[]
  return <><span className="wire-label">{e.label}</span>
    {e.kind==='list'||e.kind==='tabs'? <span className="wire-items">{lines.map((line,i)=><span key={i}>{line}</span>)}</span>:
      e.value && <span className="wire-value">{e.value}</span>}
  </>
}

export function DesignWireframe({frame,selected,onSelect}:{frame:Wireframe;selected?:string;onSelect?:(e:WireElement)=>void}){
  return <section className="design-wire" aria-label={'可视稿 · '+frame.title}>
    <header><strong>{frame.title}</strong><small>界面示意</small></header>
    <div className={'wire-canvas'+(frame.regions.some(r=>r.position==='sidebar')?' has-sidebar':'')}>
      {frame.regions.map((r,i)=><section key={i} className={'wire-region wire-'+r.position+(r.layout==='row'?' wire-row':'')} aria-label={r.label}>
        <h4>{r.label}</h4>
        {r.elements.map(e=>onSelect?
          <button type="button" key={e.id} className={'wire-element wire-'+e.kind} aria-label={'点评'+r.label+'里的'+e.label} aria-pressed={selected===e.id} onClick={()=>onSelect(e)}>{contents(e)}</button>:
          <div key={e.id} className={'wire-element wire-'+e.kind}>{contents(e)}</div>)}
      </section>)}
    </div>
    {onSelect && <small>点选控件，可针对它提出修订意见。</small>}
  </section>
}

export function GenerateWireframe({turn,revision,visual,busy,post,models}:{models:ChatModel[];turn?:number;revision:number|null;visual?:{status:string;error:string};busy:boolean;post:Post}){
  const [profile,setProfile]=useState('')
  const pending=visual?.status==='running'||visual?.status==='queued'
  return <div className="detail-actions">
    {visual && !pending && <ThemedSelect label="补图模型" value={profile} options={[{value:'',label:'原模型'},...models.map(m=>({value:m.id,label:m.name||m.model}))]} onChange={setProfile}/>}
    <button type="button" className="button" disabled={busy||pending} onClick={()=>void post({operation:turn?'generate_visual':'preview_proposal',turn_id:turn,revision,profile_id:profile||undefined},true)}>{pending?'正在生成可视稿…':visual?'重新生成可视稿':'为此版生成可视稿'}</button>
    {visual && !pending && <small role="status">{visual.error||'此回合未得到可展示的控件布局。'}</small>}
  </div>
}
