import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon, Status } from './shared'

export type PlanningTask = { id: number; title: string; status: string; goal: boolean; removable?: boolean }
export type Delivery = { id: number; title: string; body: string; tasks: PlanningTask[]; accepted: boolean; improvement?: number }
export type Milestone = { id: number; title: string; body: string; deliveries: Delivery[]; start_date?: string | null; end_date?: string | null }

function dateRange(m: Milestone, sharedMonth: boolean) {
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

export function PlanningTimeline({milestones, selected, href, base, details}: {
  milestones: Milestone[]; selected?: number; href: (id: number) => string; base: string
  details: (milestone: Milestone, index: number) => ReactNode
}) {
  const [expanded, setExpanded] = useState<number>()
  const root = useRef<HTMLElement>(null)
  const active = milestones.find(m => m.id === selected)?.id ?? milestones[0]?.id
  const months = new Set(milestones.flatMap(m => [m.start_date, m.end_date].flatMap(d => d ? [d.slice(0,7)] : [])))
  const month = months.size === 1 ? [...months][0] : ''
  useEffect(() => {
    if (!selected || selected !== active) return
    setExpanded(selected)
    root.current?.querySelector<HTMLElement>('#plan-stage-' + selected)?.scrollIntoView({block:'nearest'})
  }, [selected, active])
  return <div className="planning-columns">
    <nav className="panel planning-index milestone-index" aria-label="里程碑列表">
      <header><h2>里程碑</h2>{month && <small>{month.slice(0,4)} 年 {Number(month.slice(5))} 月</small>}</header>
      {milestones.map(m => <a key={m.id} href={href(m.id)} aria-current={active === m.id ? 'location' : undefined}
        onClick={() => { if (selected === m.id) { setExpanded(m.id); document.getElementById('plan-stage-'+m.id)?.scrollIntoView({block:'nearest'}) } }}>
        <strong>{m.title}</strong><small>{dateRange(m, Boolean(month))}</small>
      </a>)}
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
                if (!open && selected !== m.id) location.hash = href(m.id)
              }} title="展开或收起阶段详情">
              {m.title}<Icon name={open?'ChevronDown':'ChevronRight'} size={15}/></button></h3>
            <span className={'stage-status '+progress.tone}>{progress.label}</span>
          </div>
          <ul className="timeline-tasks">{tasks.map(t => <li key={t.id} className="timeline-row task-row">
            <a href={base+'/tasks/'+t.id}>{t.title}</a><Status value={t.status}/>
          </li>)}</ul>
          {!tasks.length && <p className="timeline-empty">暂无关联任务</p>}
          <div hidden={!open} id={'plan-stage-details-'+m.id} className="stage-details">{open && details(m,index)}</div>
        </li>
      })}</ol>
    </article>
  </div>
}
