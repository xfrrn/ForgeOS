import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon, Status } from './shared'
import { EmptyState } from './ui'

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
  return ''
}

function stageProgress(m: Milestone, tasks: PlanningTask[]) {
  if (m.deliveries.length && m.deliveries.every(d => d.accepted)) return { label: '已完成', tone: 'done' as const }
  if (m.deliveries.some(d => d.accepted) || tasks.some(t => !['READY', 'PENDING'].includes(t.status))) {
    return { label: '进行中', tone: 'active' as const }
  }
  return { label: '待开始', tone: 'pending' as const }
}

function ItemStatus({ item }: { item: Delivery }) {
  if (item.accepted) return <Status value="ACCEPTED"/>
  if (item.tasks.length === 1) return <Status value={item.tasks[0].status}/>
  if (item.tasks.length > 1) return <span className="roadmap-chip">{item.tasks.length} 项任务</span>
  return <span className="roadmap-chip muted">待派发</span>
}

export function PlanningTimeline({
  milestones,
  selected,
  selectedItem,
  href,
  details,
  itemDetails,
  onAddItem,
  renderAddForm,
}: {
  milestones: Milestone[]
  selected?: number
  selectedItem?: number
  href: (id: number, item?: number) => string
  details: (milestone: Milestone, index: number) => ReactNode
  itemDetails: (item: Delivery) => ReactNode
  onAddItem: (milestone: Milestone) => void
  renderAddForm?: (milestone: Milestone) => ReactNode
}) {
  const [expanded, setExpanded] = useState<number>()
  const root = useRef<HTMLDivElement>(null)
  const active = milestones.find(m => m.id === selected)?.id
  const months = new Set(
    milestones
      .flatMap(m => [m, ...m.deliveries])
      .flatMap(i => [i.start_date, i.end_date])
      .flatMap(d => (d ? [d.slice(0, 7)] : [])),
  )
  const sharedMonth = months.size === 1

  useEffect(() => {
    if (!selected || selected !== active) return
    setExpanded(selectedItem ? undefined : selected)
    const target = selectedItem ? '#plan-item-' + selectedItem : '#plan-stage-' + selected
    root.current?.querySelector<HTMLElement>(target)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }, [selected, selectedItem, active])

  if (!milestones.length) {
    return <section className="panel roadmap-empty">
      <EmptyState icon="Layers3" title="还没有规划里程碑" description="新建第一个里程碑并填写排期，即可在时间线上拆解小任务与验收标准。"/>
    </section>
  }

  return <div className="roadmap" ref={root}>
    <ol className="roadmap-list">
      {milestones.map((m, index) => {
        const tasks = [...new Map(m.deliveries.flatMap(d => d.tasks).map(t => [t.id, t])).values()]
        const progress = stageProgress(m, tasks)
        const open = expanded === m.id
        const acceptedCount = m.deliveries.filter(d => d.accepted).length
        const range = dateRange(m, sharedMonth)
        const addForm = renderAddForm?.(m)

        return <li key={m.id} className={'roadmap-milestone tone-' + progress.tone} id={'plan-stage-' + m.id}>
          <span className={'roadmap-node ' + progress.tone} aria-hidden="true">
            {progress.tone === 'done' && <Icon name="Check" size={10}/>}
          </span>

          <div className="roadmap-stage-body">
            <header className="roadmap-stage-head">
              <div className="roadmap-stage-title-group">
                <button type="button" className="roadmap-stage-toggle" aria-expanded={open}
                  aria-controls={'plan-stage-details-' + m.id}
                  onClick={() => {
                    setExpanded(open ? undefined : m.id)
                    if (!open && (selected !== m.id || selectedItem)) location.hash = href(m.id)
                  }}>
                  <Icon name="ChevronRight" size={15} className={'roadmap-chevron' + (open ? ' open' : '')}/>
                  <h2>{m.title}</h2>
                </button>
                {range && <span className="roadmap-date">{range}</span>}
                {m.deliveries.length > 0 && <span className="roadmap-ratio">{acceptedCount}/{m.deliveries.length}</span>}
              </div>

              <div className="roadmap-stage-meta">
                <span className={'stage-status ' + progress.tone}>{progress.label}</span>
                <button type="button" className="roadmap-ghost-btn" onClick={() => onAddItem(m)} title="在此里程碑添加小任务">
                  <Icon name="Plus" size={14}/><span>小任务</span>
                </button>
              </div>
            </header>

            {open && <div id={'plan-stage-details-' + m.id} className="roadmap-stage-details">
              {details(m, index)}
            </div>}

            {m.deliveries.length > 0 && <ul className="roadmap-deliveries">
              {m.deliveries.map(item => {
                const showing = selected === m.id && selectedItem === item.id
                const itemDate = dateRange(item, sharedMonth)
                return <li key={item.id} className={'roadmap-item' + (showing ? ' open' : '')} id={'plan-item-' + item.id}>
                  <a className="roadmap-item-row" href={href(m.id, showing ? undefined : item.id)}
                    aria-expanded={showing} aria-controls={'item-actions-' + item.id}>
                    <span className={'roadmap-item-dot' + (item.accepted ? ' done' : item.tasks.some(t => t.status === 'RUNNING') ? ' active' : '')} aria-hidden="true">
                      {item.accepted && <Icon name="Check" size={10}/>}
                    </span>
                    <span className="roadmap-item-title">{item.title}</span>
                    <span className="roadmap-item-right">
                      {itemDate && <span className="roadmap-item-date">{itemDate}</span>}
                      <ItemStatus item={item}/>
                      <Icon name="ChevronRight" size={14} className={'roadmap-chevron' + (showing ? ' open' : '')}/>
                    </span>
                  </a>
                  {showing && <div id={'item-actions-' + item.id} className="roadmap-item-drawer">
                    {itemDetails(item)}
                  </div>}
                </li>
              })}
            </ul>}

            {addForm && <div className="roadmap-inline-editor">{addForm}</div>}
          </div>
        </li>
      })}
    </ol>
  </div>
}
