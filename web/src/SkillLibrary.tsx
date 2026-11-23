import { useEffect, useMemo, useRef, useState } from 'react'
import { EmptyState, PageHeading, SkeletonCard } from './ui'
import { Icon, type ApiRequest } from './shared'
import './agent-settings.css'

export type Skill = { id: number; name: string; description: string; entry: string; file_count: number }
const libraryPath = '/projects/agent-skills'
const PAGE_SIZE = 24

export function SkillLibrary({ request }: { request: ApiRequest }) {
  const picker = useRef<HTMLInputElement>(null)
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [query, setQuery] = useState('')
  const [limit, setLimit] = useState(PAGE_SIZE)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [reload, setReload] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setError('')
    request<Skill[]>(libraryPath, { signal: controller.signal }).then(rows => {
      if (!controller.signal.aborted) setSkills(rows)
    }).catch(e => { if (!controller.signal.aborted) setError(e.message) })
    return () => controller.abort()
  }, [request, reload])

  async function importFolder(files: File[]) {
    if (!files.length) return
    setLoading(true)
    setError('')
    setNotice('')
    try {
      if (files.length > 1024 || files.reduce((total, file) => total + file.size, 0) > 32 * 1024 * 1024) {
        throw new Error('每个 Skill 最多可包含 1024 个文件，合计 32 MiB。')
      }
      const body = new FormData()
      for (const file of files) {
        const path = file.webkitRelativePath.split('/').slice(1).join('/')
        body.append(path, file, file.name)
      }
      if (!body.has('SKILL.md')) throw new Error('请选择直接包含 SKILL.md 的文件夹。')
      const skill = await request<Skill>(libraryPath, { method: 'POST', body })
      setNotice('已导入 ' + skill.name + '，共 ' + skill.file_count + ' 个文件。')
      setReload(value => value + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '导入失败。') }
    finally { setLoading(false) }
  }

  const filtered = useMemo(() => {
    if (!skills) return []
    const q = query.trim().toLowerCase()
    if (!q) return skills
    return skills.filter(s =>
      s.name.toLowerCase().includes(q) ||
      s.description.toLowerCase().includes(q) ||
      s.entry.toLowerCase().includes(q),
    )
  }, [skills, query])

  const visible = filtered.slice(0, limit)

  return <div className="skill-library">
    <PageHeading title="Skill 库"
      description={skills ? `已收录 ${skills.length} 个 Skill 能力包，可在各项目 Agent 管理中按需检索分配。` : '导入完整的 Skill 文件夹，供各项目的 Agent 选择。'}
      actions={<>
        <a className="button" href="#/agents"><Icon name="Bot" size={14}/>Agent 管理</a>
        <button type="button" className="button primary" disabled={loading} onClick={() => picker.current?.click()}>
          <Icon name="Plus" size={15}/>{loading ? '正在导入…' : '导入 Skill 文件夹'}
        </button>
      </>}/>

    <input type="file" multiple hidden ref={el => { picker.current = el; el?.setAttribute('webkitdirectory', '') }}
      onChange={e => { const files = Array.from(e.target.files ?? []); e.target.value = ''; void importFolder(files) }}/>

    {notice && <p className="detail-notice" role="status">{notice} <a href="#/agents">前往 Agent 管理</a></p>}
    {error && <p className="detail-problem" role="alert">{error} <button className="button" onClick={() => setReload(n => n + 1)}>重新读取</button></p>}

    {skills && skills.length > 0 && <div className="skill-library-toolbar">
      <label className="search skill-library-search">
        <Icon name="Search" size={15}/>
        <input type="search" value={query} placeholder="搜索 Skill 名称、描述或入口…"
          onChange={e => { setQuery(e.target.value); setLimit(PAGE_SIZE) }}/>
      </label>
      <span className="muted">{query ? `匹配 ${filtered.length} / ${skills.length} 项` : `共 ${skills.length} 项`}</span>
    </div>}

    {skills === null ? !error && <SkeletonCard label="正在读取 Skill 库…" lines={4}/> :
      skills.length === 0 ? <section className="panel"><EmptyState icon="Sparkles" title="尚未导入 Skill"
        description="选择直接包含 SKILL.md 的文件夹，脚本、参考资料和其他配套文件会保留原有目录结构。"/></section> :
      filtered.length === 0 ? <section className="panel"><EmptyState icon="Search" title="没有匹配的 Skill">
        <button type="button" className="button" onClick={() => setQuery('')}>清除搜索</button>
      </EmptyState></section> : <>
        <ul className="skill-packages-grid">
          {visible.map(skill => <li className="panel skill-package-card" key={skill.id}>
            <div className="skill-package-head">
              <span className="skill-package-icon"><Icon name="Sparkles" size={15}/></span>
              <h2>{skill.name}{skills.some(other => other.name === skill.name && other.id !== skill.id) && ' · #' + skill.id}</h2>
            </div>
            <p className="skill-package-desc">{skill.description}</p>
            <div className="skill-package-foot">
              <code>{skill.entry}</code>
              <span>{skill.file_count} 个文件</span>
            </div>
          </li>)}
        </ul>
        {filtered.length > limit && <div className="skill-load-more">
          <button type="button" className="button" onClick={() => setLimit(n => n + PAGE_SIZE)}>
            加载更多（剩余 {filtered.length - limit} 项）
          </button>
        </div>}
      </>}
  </div>
}
