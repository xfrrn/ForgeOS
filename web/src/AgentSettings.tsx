import { ThemedSelect } from './ThemedSelect'
import { useEffect, useMemo, useState } from 'react'
import { Fact, formatTime, Icon, type ApiRequest } from './shared'
import { ModelSelect } from './ModelSelect'
import { PageHeading, SkeletonCard } from './ui'
import './agent-settings.css'

import type { Skill } from './SkillLibrary'
type Settings = { prompt: string; harness: string; model: string; skills?: number[] }
type Config = Settings & { key: string; source: 'builtin' | 'global' | 'project'; revision: number; packages?: { name: string }[] }
type Run = { project_id: number; node: string; at: string; session: string; model: string; harness: string; outcome: string; config: Config }
type Entry = { key: string; role: string; title: string; harnesses: string[]; prompt: string; effective: Config; local_revision: number; last?: Run }

const sources = { builtin: '内置默认', global: '全局配置', project: '项目覆盖' }
const sourceTones = { builtin: 'neutral', global: 'info', project: 'positive' }
const harnessNames: Record<string, string> = { codex: 'Codex CLI', claude: 'Claude Code', task: '跟随任务配置' }

function version(config: Config) {
  return sources[config.source] + (config.revision ? ' · v' + config.revision : ' · v1')
}

function skillDisplayName(skill: Skill, catalog: Skill[]) {
  return catalog.some(s => s.name === skill.name && s.id !== skill.id) ? `${skill.name} · #${skill.id}` : skill.name
}

export function AgentSettings({ projectId, request }: { projectId?: number; request: ApiRequest }) {
  const path = projectId ? `/projects/${projectId}/agents` : '/projects/agent-settings'
  const [entries, setEntries] = useState<Entry[]>([])
  const [catalog, setCatalog] = useState<Skill[]>([])
  const [key, setKey] = useState('management-chat')
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const c = new AbortController()
    setError('')
    Promise.all([request<Entry[]>(path, { signal: c.signal }), request<Skill[]>('/projects/agent-skills', { signal: c.signal })]).then(([data, skills]) => {
      if (!c.signal.aborted) { setEntries(data); setCatalog(skills) }
    }).catch(e => { if (!c.signal.aborted) setError(e.message) })
    return () => c.abort()
  }, [path, request, reload])

  const selected = entries.find(row => row.key === key) || entries[0]
  const choose = (entry: Entry) => { setKey(entry.key); setNotice('') }
  const roleEntries = selected ? entries.filter(e => e.role === selected.role) : []

  return <div className="agent-settings">
    <PageHeading title="Agent 管理" description={projectId ? '查看本项目各角色的模型、Skill 与提示词配置及最近运行回执。' : '管理各项目继承的全局 Agent 角色默认配置。'}
      actions={<div className="agent-actions">
        <a className="button" href="#/skills"><Icon name="Sparkles" size={14}/>Skill 库</a>
        {projectId && <a className="button" href="#/agents"><Icon name="Bot" size={14}/>全局默认</a>}
      </div>}/>

    {error && <p className="detail-problem" role="alert">{error} <button className="button" onClick={() => setReload(n => n + 1)}>重新读取</button></p>}
    {!selected && !error && <SkeletonCard label="正在读取 Agent 配置…" lines={5}/>}

    {selected && <div className="agent-layout">
      <nav className="panel agent-roles" aria-label="Agent 角色">
        <span className="agent-roles-label">角色列表</span>
        {Array.from(new Set(entries.map(e => e.role))).map(role => {
          const items = entries.filter(e => e.role === role)
          return <button type="button" key={role} aria-pressed={selected.role === role} onClick={() => choose(items[0])}>
            <strong>{role}</strong>
            <span className="agent-role-count">{items.length}</span>
          </button>
        })}
      </nav>

      <div className="agent-content">
        <section className="panel agent-panel">
          <header className="agent-panel-head">
            <div>
              <div className="agent-panel-eyebrow">
                <span className={'agent-source-badge ' + sourceTones[selected.effective.source]}>{version(selected.effective)}</span>
                <span className="muted">{selected.title}</span>
              </div>
              <h2>{selected.role}</h2>
            </div>
            {roleEntries.length > 1 && <div className="agent-purpose-tabs" role="tablist" aria-label="角色用途">
              {roleEntries.map(item => <button key={item.key} type="button" role="tab"
                aria-selected={item.key === selected.key} onClick={() => choose(item)}>
                {item.title}
              </button>)}
            </div>}
          </header>

          {notice && <p className="detail-notice" role="status">{notice}</p>}

          <AgentForm
            key={[selected.key, selected.local_revision, selected.effective.revision].join('/')}
            entry={selected}
            catalog={catalog}
            project={!!projectId}
            request={request}
            path={path}
            onSaved={async message => { const rows = await request<Entry[]>(path); setEntries(rows); setNotice(message) }}
            onReload={() => setReload(n => n + 1)}
          />
        </section>

        <section className="panel agent-panel">
          <div className="agent-section-head">
            <h3><Icon name="Activity" size={15}/>最近一次运行</h3>
            {selected.last && <span className={'agent-source-badge ' + (selected.last.outcome === 'completed' ? 'positive' : 'danger')}>
              {selected.last.outcome === 'completed' ? '执行成功' : '执行失败'}
            </span>}
          </div>
          {selected.last
            ? <RunDetails run={selected.last} catalog={catalog} projectChat={selected.key === 'management-chat'}/>
            : <p className="muted">此用途尚无新版 Worker 的运行回执。</p>}
        </section>
      </div>
    </div>}
  </div>
}

function SkillPicker({
  catalog,
  selectedIds,
  disabled,
  onToggle,
  onClear,
}: {
  catalog: Skill[]
  selectedIds: number[]
  disabled: boolean
  onToggle: (skill: Skill, checked: boolean) => void
  onClear: () => void
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [tab, setTab] = useState<'all' | 'selected' | 'unselected'>('all')

  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds])
  const assignedSkills = useMemo(
    () => selectedIds.map(id => catalog.find(s => s.id === id)).filter((s): s is Skill => Boolean(s)),
    [selectedIds, catalog],
  )
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return catalog.filter(skill => {
      if (tab === 'selected' && !selectedSet.has(skill.id)) return false
      if (tab === 'unselected' && selectedSet.has(skill.id)) return false
      if (!q) return true
      return skill.name.toLowerCase().includes(q) ||
        skill.description.toLowerCase().includes(q) ||
        skill.entry.toLowerCase().includes(q)
    })
  }, [catalog, query, tab, selectedSet])

  return <section className="agent-block" aria-label="Skill 分配">
    <div className="agent-block-head">
      <div>
        <h3><Icon name="Sparkles" size={14}/>已分配 Skill <span className="agent-count-pill">{selectedIds.length} / {catalog.length}</span></h3>
        <p className="muted">任务启动时，仅向该 Agent 挂载已分配的 Skill 目录与入口文件。</p>
      </div>
      <div className="agent-block-actions">
        {selectedIds.length > 0 && <button type="button" className="agent-ghost-btn" disabled={disabled} onClick={onClear}>清空</button>}
        {catalog.length > 0 && <button type="button" className="button" disabled={disabled} aria-expanded={open}
          onClick={() => setOpen(v => !v)}>
          <Icon name={open ? 'ChevronDown' : 'Plus'} size={14}/>{open ? '收起选择器' : '选择 Skill'}
        </button>}
      </div>
    </div>

    {!catalog.length ? <p className="muted">Skill 库为空。<a href="#/skills">前往导入 Skill 文件夹</a></p> : <>
      {assignedSkills.length > 0 ? <ul className="agent-skill-chips" aria-label="已选 Skill">
        {assignedSkills.map(skill => <li key={skill.id} className="agent-skill-chip" title={skill.description}>
          <span className="agent-skill-chip-name">{skillDisplayName(skill, catalog)}</span>
          <button type="button" className="agent-skill-chip-remove" disabled={disabled}
            aria-label={'移除 ' + skill.name} onClick={() => onToggle(skill, false)}>
            <Icon name="X" size={12}/>
          </button>
        </li>)}
      </ul> : <p className="agent-skill-empty">尚未分配任何 Skill，点击右侧「选择 Skill」按需添加。</p>}

      {open && <div className="agent-skill-drawer" onKeyDown={e => { if (e.key === 'Escape') setOpen(false) }}>
        <div className="agent-skill-toolbar">
          <label className="search agent-skill-search">
            <Icon name="Search" size={14}/>
            <input type="search" value={query} placeholder="按名称、描述或入口文件筛选 Skill…"
              onChange={e => setQuery(e.target.value)}/>
          </label>
          <div className="agent-skill-tabs" role="tablist" aria-label="Skill 筛选">
            <button type="button" role="tab" aria-selected={tab === 'all'} onClick={() => setTab('all')}>全部 {catalog.length}</button>
            <button type="button" role="tab" aria-selected={tab === 'selected'} onClick={() => setTab('selected')}>已选 {selectedIds.length}</button>
            <button type="button" role="tab" aria-selected={tab === 'unselected'} onClick={() => setTab('unselected')}>未选 {catalog.length - selectedIds.length}</button>
          </div>
        </div>

        <div className="agent-skill-scroll">
          {filtered.length > 0 ? filtered.map(skill => {
            const checked = selectedSet.has(skill.id)
            return <label key={skill.id} className={'agent-skill-item' + (checked ? ' checked' : '')}>
              <input type="checkbox" checked={checked} disabled={disabled} onChange={e => onToggle(skill, e.target.checked)}/>
              <span className="agent-skill-item-copy">
                <span className="agent-skill-item-top">
                  <strong>{skillDisplayName(skill, catalog)}</strong>
                  <span className="agent-skill-meta">{skill.file_count} 文件 · <code>{skill.entry}</code></span>
                </span>
                <small>{skill.description}</small>
              </span>
            </label>
          }) : <div className="agent-skill-no-match">
            <span>没有匹配的 Skill</span>
            {(query || tab !== 'all') && <button type="button" className="button" onClick={() => { setQuery(''); setTab('all') }}>清除筛选</button>}
          </div>}
        </div>
      </div>}
    </>}
  </section>
}

function AgentForm({ entry, project, path, request, onSaved, onReload, catalog }: {
  entry: Entry; catalog: Skill[]; project: boolean; path: string; request: ApiRequest; onSaved: (s: string) => Promise<void>; onReload: () => void
}) {
  const current = entry.effective
  const [prompt, setPrompt] = useState(current.prompt)
  const [harness, setHarness] = useState(current.harness)
  const [model, setModel] = useState(current.model)
  const [skills, setSkills] = useState(current.skills ?? [])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const own = project ? current.source === 'project' : current.source === 'global'
  const changed = prompt !== current.prompt || harness !== current.harness || model.trim() !== current.model || skills.join() !== (current.skills ?? []).join()

  function selectSkill(skill: Skill, checked: boolean) {
    setSkills(ids => {
      if (checked && ids.includes(skill.id)) return ids
      const retained = ids.filter(id => id !== skill.id && (!checked || catalog.find(s => s.id === id)?.name !== skill.name))
      return checked ? [...retained, skill.id] : retained
    })
  }

  const save = async (reset: boolean) => {
    setSaving(true); setError('')
    try {
      await request(path, {
        method: 'PUT',
        body: JSON.stringify({
          key: entry.key, previous: entry.local_revision, effective: current.revision, reset,
          settings: reset ? null : { prompt, harness: entry.key === 'management-chat' ? 'codex' : harness, model: entry.key === 'management-chat' ? '' : model.trim(), skills },
        }),
      })
      await onSaved(reset ? '已恢复继承。' : '已保存，新领取的任务将使用此版本。')
    } catch (e) { setError(e instanceof Error ? e.message : '保存失败。') }
    finally { setSaving(false) }
  }

  return <form className="agent-form" onSubmit={e => { e.preventDefault(); void save(false) }}>
    {/* Section 1: Harness & Model */}
    <section className="agent-block" aria-label="运行模型与 Harness">
      <div className="agent-block-head">
        <h3><Icon name="Cpu" size={14}/>运行模型与 Harness</h3>
      </div>
      {project && ['curator', 'knowledge-review', 'share'].includes(entry.key) && <p className="detail-notice">
        本页配置用于 CLI。项目的直连模型在 <a href={path.replace(/\/agents$/, '/knowledge/model-settings').replace(/^\//, '#/')}>知识库模型设置</a> 中选择。
      </p>}
      {entry.key === 'management-chat' ? <p className="muted">
        项目聊天使用 ForgeOS 内核。{' '}
        <a href={project ? path.replace(/^\/projects\/(\d+)\/agents$/, '#/settings/credentials?project=$1') : '#/settings/credentials'}>管理共享聊天模型</a>
      </p> : <div className="agent-fields">
        <ThemedSelect label="执行 Harness" value={harness}
          options={entry.harnesses.map(value => ({ value, label: harnessNames[value] }))}
          onChange={value => { setHarness(value); setModel('') }}/>
        <ModelSelect key={harness} request={request} harness={harness} value={model} onChange={(id, provider) => {
          setModel(id)
          if (harness === 'task' && provider) setHarness(provider)
        }}/>
      </div>}
      {(entry.key === 'planner' || entry.key === 'operations') && <p className="muted">新派发的规划任务使用此 Harness，子任务沿用任务实现的配置。</p>}
      {entry.key === 'implementation' && <p className="muted">Harness 选择在新任务派发时生效；派发时明确指定的 Harness 优先。</p>}
    </section>

    {/* Section 2: Scalable Skill Picker */}
    <SkillPicker
      catalog={catalog}
      selectedIds={skills}
      disabled={saving}
      onToggle={selectSkill}
      onClear={() => setSkills([])}
    />

    {/* Section 3: System Prompt */}
    <section className="agent-block" aria-label="启动提示词">
      <div className="agent-block-head">
        <h3><Icon name="Terminal" size={14}/>启动提示词</h3>
        <span className="muted">{prompt.length} 字符</span>
      </div>
      <textarea aria-label="启动提示词" value={prompt} rows={7} required disabled={saving} onChange={e => setPrompt(e.target.value)}/>
      <p className="muted">Worker 会在运行时自动追加本次任务的上下文、输出格式与执行约束。</p>
    </section>

    {/* Actions */}
    <div className="agent-footer-bar">
      <div className="agent-actions">
        <button className="button primary" disabled={saving || !changed || !prompt.trim()}>
          {saving ? '正在保存…' : project ? '保存项目配置' : '保存全局默认'}
        </button>
        {own && <button className="button" type="button" disabled={saving} onClick={() => void save(true)}>
          {project ? '恢复继承全局' : '恢复内置默认'}
        </button>}
      </div>
      {changed && <span className="agent-unsaved-hint">有未保存更改</span>}
    </div>
    {error && <p className="detail-problem" role="alert">{error} <button type="button" className="button" onClick={onReload}>重新读取配置</button></p>}
  </form>
}

function RunDetails({ run, catalog, projectChat }: { run: Run; catalog: Skill[]; projectChat: boolean }) {
  return <>
    <dl className="agent-facts">
      <Fact label="完成时间">{formatTime(run.at)}</Fact>
      <Fact label="执行节点">{run.node}</Fact>
      <Fact label="实际 Harness">{projectChat && run.harness === 'codex' ? 'Codex 内核' : harnessNames[run.harness] || run.harness}</Fact>
      <Fact label="实际模型">{run.model || 'Harness 未报告'}</Fact>
      <Fact label="本次 Skills">{run.config.packages?.map(s => s.name).join('、') || run.config.skills?.map(id => catalog.find(s => s.id === id)?.name ?? '#' + id).join('、') || '无'}</Fact>
      <Fact label="提示词版本">{version(run.config)}</Fact>
      <Fact label="所属项目">#{run.project_id}</Fact>
      {run.session && <Fact label="原生会话" code>{run.session}</Fact>}
    </dl>
    <details className="agent-prompt-drawer">
      <summary>查看本次启动提示词<Icon name="ChevronDown" size={14}/></summary>
      <pre>{run.config.prompt || '（沿用默认提示词）'}</pre>
    </details>
  </>
}
