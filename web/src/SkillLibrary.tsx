import { useEffect, useRef, useState } from 'react'
import { PageHeading } from './ui'
import type { ApiRequest } from './shared'

export type Skill = { id: number; name: string; description: string; entry: string; file_count: number }
const libraryPath = '/projects/agent-skills'

export function SkillLibrary({request}: {request: ApiRequest}) {
  const picker = useRef<HTMLInputElement>(null)
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [reload, setReload] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setError('')
    request<Skill[]>(libraryPath, {signal: controller.signal}).then(rows => {
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
      const skill = await request<Skill>(libraryPath, {method: 'POST', body})
      setNotice('已导入 ' + skill.name + '，共 ' + skill.file_count + ' 个文件。')
      setReload(value => value + 1)
    } catch (e) { setError(e instanceof Error ? e.message : '导入失败。') }
    finally { setLoading(false) }
  }

  return <div className="skill-library">
    <PageHeading title="Skill 库" description="导入完整的 Skill 文件夹，供各项目的 Agent 选择。"
      actions={<button type="button" className="button primary" disabled={loading} onClick={() => picker.current?.click()}>{loading ? '正在导入…' : '导入 Skill 文件夹'}</button>}/>
    <input type="file" multiple hidden ref={el => { picker.current = el; el?.setAttribute('webkitdirectory', '') }}
      onChange={e => { const files = Array.from(e.target.files ?? []); e.target.value = ''; void importFolder(files) }}/>
    {notice && <p role="status">{notice} <a href="#/agents">前往 Agent 管理</a></p>}
    {error && <p role="alert">{error} <button className="button" onClick={() => setReload(n => n+1)}>重新读取技能库</button></p>}
    {skills === null ? !error && <p className="panel table-message">正在读取 Skill 库…</p> : skills.length === 0 ?
      <section className="panel agent-panel"><h2>尚未导入 Skill</h2><p>选择直接包含 SKILL.md 的文件夹。脚本、参考资料和其他配套文件会保留原有目录结构。</p></section> :
      <ul className="skill-packages">{skills.map(skill => <li className="panel agent-panel" key={skill.id}>
        <h2>{skill.name}{skills.some(other => other.name === skill.name && other.id !== skill.id) && ' · #' + skill.id}</h2>
        <p>{skill.description}</p><small>{skill.file_count} 个文件</small><code>{skill.entry}</code>
      </li>)}</ul>}
  </div>
}
