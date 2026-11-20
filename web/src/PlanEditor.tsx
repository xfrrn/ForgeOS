import { ThemedSelect } from './ThemedSelect'
import { useState, type FormEvent } from 'react'
import { ApiError, type ApiRequest } from './shared'
import type { Plan, PlanItem } from './GoalPlan'

type Document = Plan['plan']
type Draft = { base: number; level: string; document: Document }
type Props = { task: number; current?: Plan; issueTitle: string; docsOnly: boolean; canEdit: boolean; disabled: boolean; request: ApiRequest;
  onSaved: (revision: number) => void; onConflict: () => void }

function emptyItem(): PlanItem {
  return { key: '', objective: '', scope: '', acceptance_criteria: '', deliverables: '' }
}

function lines(text?: string[]) {
  return text?.filter(line => line !== '')
}

export function PlanEditor({task, current, issueTitle, docsOnly, canEdit, disabled, request, onSaved, onConflict}: Props) {
  const [draft, setDraft] = useState<Draft>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  function start() {
    setError('')
    setDraft({base: current?.revision ?? 0, level: current?.minimum_level || '',
      document: current ? structuredClone(current.plan) : {objective: issueTitle, milestones: [{name: '', items: [emptyItem()]}]}})
  }
  if (!draft) return canEdit ? <button className="button" disabled={disabled} onClick={start}>{current ? `以修订 ${current.revision} 编辑新草案` : '起草 Goal 计划'}</button> : null

  const {document, base} = draft
  function change(next: Document) { setDraft(state => state && {...state, document: next}) }
  function milestone(index: number, patch: Partial<Document['milestones'][number]>) {
    change({...document, milestones: document.milestones.map((m,i) => i === index ? {...m,...patch} : m)})
  }
  function item(m: number, index: number, patch: Partial<PlanItem>) {
    milestone(m, {items: document.milestones[m].items!.map((it,i) => i === index ? {...it,...patch} : it)})
  }
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!draft || busy || disabled) return
    setBusy(true); setError('')
    const plan = {...document, milestones: document.milestones.map(m => ({...m, items: m.items?.map(it => ({
      ...it, dependencies: lines(it.dependencies), context_refs: lines(it.context_refs),
    })) ?? null}))}
    try {
      const saved = await request<Plan>(`/tasks/${task}/plans`, {method: 'POST', body: JSON.stringify({
        previous_revision: base, minimum_level: draft.level, plan,
      })})
      setDraft(undefined)
      onSaved(saved.revision)
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setError('此草案无法发布。请核对最新修订和已开始的子项；本地编辑已保留。')
        onConflict()
      } else { setError(e instanceof ApiError ? e.message : '未确认发布结果。已保留草案，可用相同内容重试。') }
    } finally { setBusy(false) }
  }
  return <form className="panel detail-card material-form plan-editor" onSubmit={submit}>
    <h2>编辑计划草案 · 修订 {base + 1}</h2>
    {current && current.revision !== base && <p role="status">此编辑基于修订 {base}，最新修订为 {current.revision}。</p>}
    <fieldset disabled={busy}>
      <label>Goal 目标<textarea required value={document.objective} onChange={e => change({...document, objective: e.target.value})}/></label>
      <ThemedSelect label="最低决策等级" value={draft.level} onChange={level=>setDraft({...draft,level})}
        options={[{value:'',label:'默认（D2）'},...(docsOnly ? ['D1','D2','D3'] : ['D2','D3']).map(value=>({value,label:value}))]}/>
      {document.milestones.map((m, mi) => <fieldset className="plan-stage" key={mi}>
        <legend>里程碑 {mi + 1}</legend>
        <label>里程碑名称<input required value={m.name} onChange={e => milestone(mi, {name: e.target.value})}/></label>
        {m.items?.map((it, ii) => <fieldset className="plan-item" key={ii}><legend>工作项 {ii + 1}</legend>
          <label>工作项标识<input required maxLength={64} value={it.key} onChange={e => item(mi, ii, {key: e.target.value})}/></label>
          <label>工作项目标<input required value={it.objective} onChange={e => item(mi, ii, {objective: e.target.value})}/></label>
          <label>范围<textarea required value={it.scope} onChange={e => item(mi, ii, {scope: e.target.value})}/></label>
          <label>验收条件<textarea required value={it.acceptance_criteria} onChange={e => item(mi, ii, {acceptance_criteria: e.target.value})}/></label>
          <label>交付物<textarea required value={it.deliverables} onChange={e => item(mi, ii, {deliverables: e.target.value})}/></label>
          <label>依赖工作项标识（每行一项）<textarea value={it.dependencies?.join('\n') ?? ''} onChange={e => item(mi, ii, {dependencies: e.target.value.split('\n')})}/></label>
          <label>上下文引用（每行一项）<textarea placeholder="仓库路径或项目决策页中的计划引用" value={it.context_refs?.join('\n') ?? ''} onChange={e => item(mi, ii, {context_refs: e.target.value.split('\n')})}/></label>
          <button className="button" type="button" onClick={() => milestone(mi, {items: m.items!.filter((_,i) => i !== ii)})}>移除工作项</button>
        </fieldset>)}
        <div className="detail-actions">
          <button className="button" type="button" onClick={() => milestone(mi, {items: [...(m.items ?? []), emptyItem()]})}>添加工作项</button>
          <button className="button" type="button" onClick={() => change({...document, milestones: document.milestones.filter((_,i) => i !== mi)})}>移除里程碑</button>
        </div>
      </fieldset>)}
      <button className="button" type="button" onClick={() => change({...document, milestones: [...document.milestones, {name: '', items: []}]})}>添加里程碑</button>
    </fieldset>
    {error && <p role="alert">{error}</p>}
    <div className="detail-actions">
      <button className="button primary" disabled={busy || disabled} type="submit">发布草案修订 {base + 1}</button>
      <button className="button" disabled={busy} type="button" onClick={() => setDraft(undefined)}>放弃本地编辑</button>
    </div>
  </form>
}
