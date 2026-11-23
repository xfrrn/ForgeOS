import { useEffect, useRef, useState, type FormEvent } from 'react'
import { MaterialReader } from './MaterialReader'
import { ThemedSelect, type SelectOption } from './ThemedSelect'
import { EmptyState, PageHeading, SkeletonCard } from './ui'
import { ApiError, formatTime, Icon, statusLabels, type ApiRequest } from './shared'
import { PlanningTimeline, dateRange, type PlanningTask as Task, type Delivery, type Milestone } from './PlanningTimeline'
import './project-planning.css'

type TextRecord = { id: number; title: string; body: string }
type Retro = TextRecord & { milestone?: number; snapshot: Milestone[]; created_at: string }
type Improvement = TextRecord & { retro: number; destination?: number }
type Plan = {
  revision: number; overview: string; milestones: Milestone[]; retros: Retro[]; improvements: Improvement[]
  tasks: Task[]; excerpts: { entry: number; retro: number; title: string }[]
  drafts: { id: number; delivery: number; title: string; body: string; status: string; goal: boolean; removable?: boolean }[]
}
type Edit = { action: string; id?: number; target?: number; title?: string; body?: string; flag?: boolean; start_date?: string | null; end_date?: string | null }
type Editor = Edit & { heading: string; revision: number; options?: SelectOption[]; original?: string }

const bodies: Record<string, string> = { overview: '规划正文', milestone: '阶段目标', delivery: '说明与验收标准', retro: '复盘正文', improvement: '改进措施与验收标准', draft: 'Issue 正文' }
const choices: Record<string, string> = { carry: '目标里程碑', link: '项目任务或 Goal', unlink: '移除关联', retro: '复盘范围' }
const scopeOption = { value: '0', label: '整个项目' }
const opt = (r: { id: number; title: string }) => ({ value: String(r.id), label: r.title })
const completed = (t: Task) => ['DONE', 'ACCEPTED', 'COMPLETED'].includes(t.status)

function TaskLinks({ tasks, base }: { tasks: Task[]; base: string }) {
  return <ul className="planning-task-list">{tasks.map(t => <li key={t.id}>
    <a href={base + '/tasks/' + t.id}>{t.goal ? 'Goal' : '任务'} #{t.id} · {t.title}</a>
    <span className="roadmap-chip">{statusLabels[t.status] || t.status}</span>
  </li>)}</ul>
}

function PlanningEditor({ initial, busy, error, submit, close }: { initial: Editor; busy: boolean; error: string; submit: (e: Edit) => void; close: () => void }) {
  const [title, setTitle] = useState(initial.title || ''), [body, setBody] = useState(initial.body || '')
  const [target, setTarget] = useState(String(initial.target || initial.options?.[0]?.value || 0))
  const [query, setQuery] = useState('')
  const [start, setStart] = useState(initial.start_date || ''), [end, setEnd] = useState(initial.end_date || '')
  const form = useRef<HTMLFormElement>(null)
  const scheduled = initial.action === 'milestone' || initial.action === 'delivery'
  const excerpt = initial.action === 'excerpt', hasTitle = !!bodies[initial.action] && initial.action !== 'overview' || excerpt
  useEffect(() => {
    form.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    form.current?.querySelector<HTMLElement>('input,textarea,button')?.focus({ preventScroll: true })
  }, [])
  const options = initial.options?.filter(o => o.label.toLowerCase().includes(query.toLowerCase()))
  const chosen = options?.some(o => o.value === target) ? target : options?.[0]?.value
  function save(event: FormEvent) {
    event.preventDefault()
    submit({ action: initial.action, id: initial.id, target: Number(chosen ?? initial.target ?? 0), title, body, flag: initial.flag, ...(scheduled ? { start_date: start, end_date: end } : {}) })
  }
  return <form className="panel planning-editor" ref={form} onSubmit={save}>
    <h2>{initial.heading}</h2>
    {initial.action === 'link' && <label>筛选任务<input value={query} onChange={e => setQuery(e.target.value)} placeholder="任务编号或标题"/></label>}
    {options && <ThemedSelect label={choices[initial.action]} options={options} value={chosen || ''} onChange={setTarget}/>}
    {hasTitle && <label>{initial.action === 'delivery' ? '小任务名称' : '标题'}<input required maxLength={256} value={title} onChange={e => setTitle(e.target.value)}/></label>}
    {scheduled && <div className="milestone-dates">
      <label>开始日期<input type="date" value={start} max={end || '9999-12-31'} onChange={e => setStart(e.target.value)}/></label>
      <label>结束日期<input type="date" value={end} min={start || '0001-01-01'} max="9999-12-31" onChange={e => setEnd(e.target.value)}/></label>
    </div>}
    {initial.action === 'delivery' && initial.id && <details open={Boolean(initial.body)}><summary>说明与验收标准（可选）</summary>
      <label>正文（Markdown）<textarea rows={3} value={body} onChange={e => setBody(e.target.value)}/></label></details>}
    {bodies[initial.action] && initial.action !== 'delivery' && <label>{bodies[initial.action]}（Markdown）<textarea rows={6} value={body} onChange={e => setBody(e.target.value)}/></label>}
    {excerpt && <><label>在复盘原文中选取片段<textarea readOnly rows={6} value={initial.original} onSelect={e => {
      const el = e.currentTarget
      if (el.selectionEnd > el.selectionStart) setBody(el.value.slice(el.selectionStart, el.selectionEnd))
    }}/></label><label>选存内容<textarea rows={3} value={body} onChange={e => setBody(e.target.value)} placeholder="也可以粘贴一段复盘原文"/></label></>}
    {initial.action === 'draft' && <p className="muted">保存草稿后，可以确认创建 GitHub Issue 并派发。</p>}
    {error && <p className="detail-problem" role="alert">{error}</p>}
    <div className="planning-actions">
      <button className="button primary" disabled={busy || (hasTitle && !title.trim()) || (options && !chosen) ||
        (excerpt && (!body.trim() || !initial.original?.includes(body))) || (initial.action === 'draft' && !body.trim())}>
        {excerpt ? '存为项目经验' : initial.action === 'carry' ? '加入里程碑' : initial.action === 'link' ? '关联任务' : '保存'}
      </button>
      <button type="button" className="button" onClick={close} disabled={busy}>取消</button>
    </div>
  </form>
}

export default function ProjectPlanning({ project, request, retrospectives, selected, selectedItem }: {
  project: number; request: ApiRequest; retrospectives: boolean; selected?: number; selectedItem?: number
}) {
  const base = '/projects/' + project, href = '#' + base, path = base + '/planning'
  const planHref = (id: number, item?: number) => href + '/planning?milestone=' + id + (item ? '&item=' + item : '')
  const retroHref = (id: number) => href + '/planning/retrospectives?retro=' + id
  const [plan, setPlan] = useState<Plan>(), [editor, setEditor] = useState<Editor>(), [error, setError] = useState(''), [note, setNote] = useState('')
  const [busy, setBusy] = useState(false), [refresh, setRefresh] = useState(0)
  const lock = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    request<Plan>(path, { signal: controller.signal }).then(v => { if (!controller.signal.aborted) setPlan(v) })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [path, request, refresh])
  useEffect(() => { setEditor(undefined); setError(''); setNote('') }, [selected, selectedItem])

  function open(e: Omit<Editor, 'revision'>) { if (plan) { setError(''); setNote(''); setEditor({ ...e, revision: plan.revision }) } }
  function reload() { setError(''); setEditor(undefined); setRefresh(n => n + 1) }

  async function mutate(e: Edit) {
    if (!plan || lock.current) return
    lock.current = true; setBusy(true); setError(''); setNote('')
    try {
      const next = await request<Plan>(path, { method: 'POST', body: JSON.stringify({ ...e, revision: editor?.revision ?? plan.revision }) })
      setPlan(next); setEditor(undefined); setNote('已保存。')
      if (!e.id && e.action === 'milestone') location.hash = planHref(next.milestones.at(-1)!.id)
      if (!e.id && e.action === 'retro') location.hash = retroHref(next.retros[0].id)
    } catch (err) {
      setError(err instanceof ApiError && err.status === 409 ? '当前记录已变化，请保留编辑内容并刷新。' : err instanceof Error ? err.message : '保存失败。')
    } finally { lock.current = false; setBusy(false) }
  }

  async function dispatch(id: number) {
    if (lock.current) return
    lock.current = true; setBusy(true); setError(''); setNote('')
    try {
      const turn = await request<{ action?: { status: string } }>(base + '/chat/actions/' + id + '/confirm', { method: 'POST', body: '{}' })
      setPlan(await request<Plan>(path))
      setNote(turn.action?.status === 'confirmed' ? '已创建并派发，可从小任务打开执行记录。' : '草稿来源已变化，请检查后重新拟稿。')
    } catch (err) { setError(err instanceof Error ? err.message : '派发失败。') }
    finally { lock.current = false; setBusy(false) }
  }

  const button = (label: string, click: () => void, disabled = false, primary = false) =>
    <button className={'button' + (primary ? ' primary' : '')} type="button" onClick={click} disabled={busy || disabled}>{label}</button>

  if (!plan) return error
    ? <section className="panel"><EmptyState icon="AlertCircle" title="读取项目规划失败" description={error}>{button('重试', reload)}</EmptyState></section>
    : <SkeletonCard label="正在读取项目规划…" lines={5}/>

  const retro = plan.retros.find(r => r.id === selected) || plan.retros[0]
  const createRetro = (id = 0) => open({ action: 'retro', heading: '新建复盘', target: id, options: [scopeOption, ...plan.milestones.map(opt)], body: '## 实际结果\n\n\n## 偏差与原因\n\n\n## 后续建议\n' })

  const totalDeliveries = plan.milestones.reduce((sum, m) => sum + m.deliveries.length, 0)
  const doneDeliveries = plan.milestones.reduce((sum, m) => sum + m.deliveries.filter(d => d.accepted).length, 0)

  function itemDetails(d: Delivery) {
    const source = plan!.improvements.find(i => i.id === d.improvement)
    const links = plan!.tasks.filter(t => !d.tasks.some(l => l.id === t.id)).map(t => ({ value: String(t.id), label: (t.goal ? 'Goal' : '任务') + ' #' + t.id + ' · ' + t.title }))
    const draft = (goal: boolean) => open({ action: 'draft', heading: goal ? '创建 Goal 草稿' : '创建 Issue 草稿', id: d.id, flag: goal, title: d.title, body: d.body ? d.title + '\n\n' + d.body : d.title })
    return <div className="roadmap-delivery-detail" key={d.id}>
      {source && <a className="roadmap-source-link" href={retroHref(source.retro)}>来自复盘 · {source.title}</a>}
      {d.body && <MaterialReader markdown content={d.body}/>}
      {d.tasks.length > 0 && <div className="roadmap-linked-tasks">
        <span className="muted">关联任务 · 完成 {d.tasks.filter(completed).length}/{d.tasks.length}</span>
        <TaskLinks tasks={d.tasks} base={href}/>
      </div>}
      {plan!.drafts.filter(a => a.delivery === d.id).map(a => <div key={a.id} className="planning-draft">
        <details><summary>{a.goal ? 'Goal' : '任务'}草稿 · {a.title}</summary><MaterialReader markdown content={a.body}/></details>
        {a.status === 'pending' ? button('确认创建 Issue 并派发', () => void dispatch(a.id), false, true) : <span className="muted">草稿已失效</span>}
      </div>)}
      <div className="planning-actions">
        {button(d.accepted ? '重新打开' : '标记已验收', () => void mutate({ action: 'accept', id: d.id, flag: !d.accepted }), false, !d.accepted)}
        {button('编辑', () => open({ ...d, action: 'delivery', heading: '编辑小任务' }))}
        {button('关联任务', () => open({ action: 'link', heading: '关联已有任务', id: d.id, options: links }), !links.length)}
        {d.tasks.some(t => t.removable) && button('移除关联', () => open({ action: 'unlink', heading: '移除任务关联', id: d.id, options: d.tasks.filter(t => t.removable).map(t => ({ value: String(t.id), label: '#' + t.id + ' · ' + t.title })) }))}
        {button('创建 Issue', () => draft(false))}
        {button('创建 Goal', () => draft(true))}
      </div>
    </div>
  }

  const stageDetails = (mile: Milestone, index: number) => <div className="roadmap-stage-card">
    {mile.body ? <MaterialReader markdown content={mile.body}/> : <p className="muted">暂无阶段目标说明。</p>}
    <div className="planning-actions">
      {button('编辑里程碑', () => open({ ...mile, action: 'milestone', heading: '编辑里程碑' }))}
      {button('上移', () => void mutate({ action: 'order', id: mile.id, flag: true }), index === 0)}
      {button('下移', () => void mutate({ action: 'order', id: mile.id }), index === plan.milestones.length - 1)}
      {button('开始阶段复盘', () => createRetro(mile.id))}
    </div>
    {plan.retros.filter(r => r.milestone === mile.id).map(r => <p key={r.id}><a href={retroHref(r.id)}>查看复盘 · {r.title}</a></p>)}
  </div>

  const addingItem = editor?.action === 'delivery' && !editor.id
  const form = editor && <PlanningEditor key={editor.heading + '/' + editor.id + '/' + editor.target} initial={editor} busy={busy}
    error={error} close={() => { setEditor(undefined); setError('') }} submit={mutate}/>

  return <div className="planning-page">
    <PageHeading
      title={retrospectives ? '阶段复盘' : '规划时间线'}
      description={!retrospectives && plan.milestones.length > 0
        ? `${plan.milestones.length} 个里程碑 · ${doneDeliveries}/${totalDeliveries} 项小任务已验收`
        : undefined}
      actions={<>
        <button className="button" type="button" onClick={reload} disabled={busy}><Icon name="RefreshCw" size={14}/>刷新</button>
        {retrospectives
          ? <button className="button primary" type="button" onClick={() => createRetro()} disabled={busy}><Icon name="Plus" size={15}/>新建复盘</button>
          : <button className="button primary" type="button" onClick={() => open({ action: 'milestone', heading: '新建里程碑' })} disabled={busy}><Icon name="Plus" size={15}/>新建里程碑</button>}
      </>}/>

    {!addingItem && form}
    {!editor && error && <p className="detail-problem" role="alert">{error}</p>}
    {note && <p className="detail-notice" role="status">{note}</p>}

    {!retrospectives && <section className="roadmap-overview-strip" aria-label="规划总览">
      <div className="roadmap-overview-content">
        {plan.overview ? <MaterialReader markdown content={plan.overview}/> : <p className="muted">写下本项目阶段目标、范围与优先级…</p>}
      </div>
      <button className="roadmap-ghost-btn" type="button" disabled={busy}
        onClick={() => open({ action: 'overview', heading: '编辑规划总览', body: plan.overview })}>
        编辑总览
      </button>
    </section>}

    {!retrospectives ? <PlanningTimeline
      milestones={plan.milestones}
      selected={selected}
      selectedItem={selectedItem}
      href={planHref}
      details={stageDetails}
      itemDetails={itemDetails}
      onAddItem={m => open({ action: 'delivery', target: m.id, heading: '添加小任务' })}
      renderAddForm={m => (addingItem && editor.target === m.id ? form : null)}
    /> : retro ? <div className="planning-columns">
      <nav className="panel planning-index" aria-label="复盘列表">
        <h2>复盘记录</h2>
        {plan.retros.map(r => <a key={r.id} href={retroHref(r.id)} aria-current={r.id === retro.id ? 'page' : undefined}>
          <strong>{r.title}</strong><small>{formatTime(r.created_at)}</small>
        </a>)}
      </nav>
      <article className="panel planning-panel">
        <header><h2>{retro.title}</h2>{button('编辑复盘', () => open({ ...retro, action: 'retro', heading: '编辑复盘' }))}</header>
        <p className="muted">{plan.milestones.find(m => m.id === retro.milestone)?.title || '整个项目'} · {formatTime(retro.created_at)}</p>
        <details className="planning-snapshot"><summary>复盘时的里程碑与小任务进度</summary>
          {!retro.snapshot.length && <p className="muted">创建复盘时还没有里程碑。</p>}
          {retro.snapshot.map(m => <section key={m.id}><h3>{m.title}</h3>{m.body && <MaterialReader markdown content={m.body}/>}
            {m.deliveries.map(d => <div key={d.id}><h4>{d.title} · {d.accepted ? '已验收' : '待验收'}</h4>{(d.start_date || d.end_date) && <small>{dateRange(d, false)}</small>}{d.body && <MaterialReader markdown content={d.body}/>}<TaskLinks tasks={d.tasks} base={href}/></div>)}</section>)}
        </details>
        <MaterialReader markdown content={retro.body}/>
        <div className="planning-actions">{button('摘录为项目经验', () => open({ action: 'excerpt', id: retro.id, heading: '选存项目经验', original: retro.body }), !retro.body.trim())}</div>
        {plan.excerpts.filter(e => e.retro === retro.id).map(e => <p key={e.entry}><a href={href + '/knowledge/experience/' + e.entry}>项目经验 · {e.title}</a></p>)}
        <header><h2>改进项</h2>{button('新增改进项', () => open({ action: 'improvement', heading: '新增改进项', target: retro.id }))}</header>
        {plan.improvements.filter(i => i.retro === retro.id).map(i => {
          const options = plan.milestones.slice(plan.milestones.findIndex(m => m.id === retro.milestone) + 1).map(opt)
          return <section className="planning-delivery" key={i.id}><h3>{i.title}</h3>{i.body && <MaterialReader markdown content={i.body}/>}
            {i.destination ? <a href={planHref(i.destination)}>已加入 · {plan.milestones.find(m => m.id === i.destination)?.title}</a> : <div className="planning-actions">
              {button('编辑改进项', () => open({ ...i, heading: '编辑改进项', action: 'improvement' }))}
              {button('加入后续里程碑', () => open({ action: 'carry', heading: '安排改进项', id: i.id, options }), !options.length)}
              {!options.length && <a href={href + '/planning'}>先添加后续里程碑</a>}
            </div>}</section>
        })}
      </article>
    </div> : <section className="panel"><EmptyState icon="Layers3" title="还没有项目复盘"
      description="创建一次阶段或项目复盘，记录结果和改进项。"/></section>}
  </div>
}
