import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon, Status } from './shared'

export type PlanningTask = { id: number; title: string; status: string; goal: boolean; removable?: boolean }
type Schedule = { start_date?: string | null; end_date?: string | null }
export type Delivery = Schedule & { id: number; title: string; body: string; tasks: PlanningTask[]; accepted: boolean; improvement?: number }
export type Milestone = Schedule & { id: number; title: string; body: string; deliveries: Delivery[] }

export function dateRange(m: Schedule, sharedMonth: boolean) {
  const format = (date: string) => (sharedMonth ? date.slice(5) : date).replaceAll('-', '.')
  const start = m.start_date && format(m.start_date)
  const end = m.end_date && format(m.end_date)
  if (start && end) return start === end ? start : start + ' – ' + end
  if (start) return start + ' 起'
  if (end) return '截至 ' + end
  return '日期待定'
}

function stageProgress(m: Milestone, tasks: PlanningTask[]) {
  if (m.deliveries.length && m.deliveries.every(d => d.accepted)) return {label:'已完成', tone:'done'}
  if (m.deliveries.some(d => d.accepted) || tasks.some(t => !['READY','PENDING'].includes(t.status))) {
    return {label:'进行中', tone:'active'}
  }
  return {label:'待开始', tone:'pending'}
}

function ItemStatus({item}:{item:Delivery}) {
  if (item.accepted) return <Status value="ACCEPTED"/>
  if (item.tasks.length === 1) return <Status value={item.tasks[0].status}/>
  return <span className="stage-status">{item.tasks.length ? '已关联 '+item.tasks.length+' 项' : '待派发'}</span>
}

export function PlanningTimeline({milestones, selected, selectedItem, href, details, itemDetails, addItem}: {
  milestones: Milestone[]; selected?: number; selectedItem?: number; href: (id: number, item?:number) => string
  details: (milestone: Milestone, index: number) => ReactNode
  itemDetails: (item: Delivery) => ReactNode
  addItem: (milestone: Milestone) => ReactNode
}) {
  const [expanded, setExpanded] = useState<number>()
  const root = useRef<HTMLElement>(null)
  const active = milestones.find(m => m.id === selected)?.id ?? milestones[0]?.id
  const months = new Set(milestones.flatMap(m => [m,...m.deliveries]).flatMap(i => [i.start_date,i.end_date]).flatMap(d => d ? [d.slice(0,7)] : []))
  const month = months.size === 1 ? [...months][0] : ''
  useEffect(() => {
    if (!selected || selected !== active) return
    setExpanded(selectedItem ? undefined : selected)
    const target = selectedItem ? '#plan-item-'+selectedItem : '#plan-stage-'+selected
    root.current?.querySelector<HTMLElement>(target)?.scrollIntoView({block:'nearest'})
  }, [selected, selectedItem, active])
  return <div className="planning-columns">
    <nav className="panel planning-index milestone-index" aria-label="里程碑列表">
      <header><h2>里程碑</h2>{month && <small>{month.slice(0,4)} 年 {Number(month.slice(5))} 月</small>}</header>
      {milestones.map(m => <div key={m.id} className="milestone-group"><a href={href(m.id)} aria-current={active === m.id ? 'location' : undefined}
        onClick={() => { if (selected === m.id) { setExpanded(m.id); document.getElementById('plan-stage-'+m.id)?.scrollIntoView({block:'nearest'}) } }}>
        <strong>{m.title}</strong><small>{dateRange(m, Boolean(month))}</small>
      </a>
        {m.deliveries.map(item=><a className="planned-child" key={item.id} href={href(m.id,item.id)} aria-current={selected===m.id && selectedItem===item.id?'location':undefined}>
          <span>{item.title}</span><small>{dateRange(item,Boolean(month))}</small></a>)}
      </div>)}
      {!milestones.length && <small>尚无里程碑</small>}
    </nav>
    <article className="panel planning-timeline" ref={root}>
      <h2>规划时间线</h2>
      {!milestones.length && <p>新建第一个里程碑，填写日期后即可安排任务。</p>}
      <ol className="timeline-stages">{milestones.map((m,index) => {
        const tasks = [...new Map(m.deliveries.flatMap(d => d.tasks).map(t => [t.id,t])).values()]
        const progress = stageProgress(m,tasks)
        const open = expanded === m.id
        return <li key={m.id} className="timeline-stage" id={'plan-stage-'+m.id}>
          <div className="timeline-row stage-row">
            <h3><button type="button" className="stage-toggle" aria-expanded={open} aria-controls={'plan-stage-details-'+m.id}
              onClick={() => {
                setExpanded(open ? undefined : m.id)
                if (!open && (selected !== m.id || selectedItem)) location.hash = href(m.id)
              }} title="展开或收起阶段详情">
              {m.title}<Icon name={open?'ChevronDown':'ChevronRight'} size={15}/></button></h3>
            <span className={'stage-status '+progress.tone}>{progress.label}</span>
          </div>
          <ul className="timeline-tasks">{m.deliveries.map(item => {
            const showing = selected === m.id && selectedItem === item.id
            return <li key={item.id} className="planned-item" id={'plan-item-'+item.id}>
              <div className="timeline-row task-row">
                <a href={href(m.id,showing ? undefined : item.id)} aria-expanded={showing} aria-controls={'item-actions-'+item.id}>
                  {item.title}<Icon name={showing?'ChevronDown':'ChevronRight'} size={14}/></a>
                <ItemStatus item={item}/>
              </div>
              <div id={'item-actions-'+item.id} hidden={!showing}>{showing && itemDetails(item)}</div>
            </li>
          })}</ul>
          {addItem(m)}
          <div hidden={!open} id={'plan-stage-details-'+m.id} className="stage-details">{open && details(m,index)}</div>
        </li>
      })}</ol>
    </article>
  </div>
}
