import { useCallback, useEffect, useRef, useState } from 'react'
import { Icon, type ApiRequest } from './shared'
import { EmptyState, PageHeading } from './ui'
import { projectDisplayName, type WorkspaceProject } from './ProjectWorkspace'
import { pendingProjectClone, ProjectRegistration, type ProjectRegistrationMethod } from './ProjectRegistration'
import { RuntimeSummary } from './RuntimeStatus'

type Page = { items: WorkspaceProject[]; next_before?: number }
type Directory = Page & { query: string }

const projectMetrics = [
  { key: 'running', label: '执行中', destination: '/tasks?status=RUNNING' },
  { key: 'pending_approvals', label: '待审批', destination: '/reviews?tab=approvals' },
  { key: 'awaiting_acceptance', label: '待验收', destination: '/reviews?tab=acceptance' },
] as const

export function Projects({ request }: { request: ApiRequest }) {
  const [directory, setDirectory] = useState<Directory>()
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [showRegistration, setShowRegistration] = useState(() => Boolean(pendingProjectClone()))
  const [loading, setLoading] = useState(true)
  const [listRevision, setListRevision] = useState(0)
  const [error, setError] = useState('')
  const [registered, setRegistered] = useState<{project: WorkspaceProject; method: ProjectRegistrationMethod}>()
  const listing = useRef<AbortController | null>(null)

  const load = useCallback(async (signal: AbortSignal, before?: number) => {
    setLoading(true); setError('')
    const params = new URLSearchParams()
    if (query) params.set('q', query)
    if (before) params.set('before', String(before))
    try {
      const page = await request<Page>('/projects' + (params.size ? '?' + params : ''), { signal })
      if (signal.aborted) return
      setDirectory(old => ({
        ...page,
        query,
        items: before && old?.query === query ? [...old.items, ...page.items.filter(item => !old.items.some(row => row.id === item.id))] : page.items,
      }))
    } catch (e) {
      if (!signal.aborted) setError(e instanceof Error ? e.message : '无法读取项目。')
    } finally {
      if (!signal.aborted) setLoading(false)
    }
  }, [request, query])

  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(input.trim()), 230)
    return () => window.clearTimeout(timer)
  }, [input])

  useEffect(() => {
    const controller = new AbortController()
    listing.current = controller
    void load(controller.signal)
    return () => controller.abort()
  }, [load, listRevision])

  const onRegistered = useCallback((project: WorkspaceProject, method: ProjectRegistrationMethod) => {
    setRegistered({project, method})
    setShowRegistration(false)
    setInput(''); setQuery('')
    setListRevision(n => n + 1)
  }, [])

  const page = directory?.query === query ? directory : undefined
  const refresh = (before?: number) => { if (listing.current) void load(listing.current.signal, before) }
  const clearSearch = () => { setInput(''); setQuery('') }

  return <div className="project-home">
    <PageHeading title="项目"/>
    <RuntimeSummary request={request}/>
    <div className="project-home-toolbar">
      <button className="button primary" type="button" aria-expanded={showRegistration}
        aria-controls={showRegistration ? 'project-registration' : undefined} onClick={() => {
          if (!showRegistration) setRegistered(undefined)
          setShowRegistration(open => !open)
        }}>
        <span className="project-add-mark" aria-hidden="true">+</span>接入项目
      </button>
      <label className="search project-search"><Icon name="Search" size={17}/>
        <input type="search" aria-label="搜索项目或仓库" placeholder="搜索项目或仓库"
          maxLength={120} value={input} onChange={e => setInput(e.target.value)}/>
      </label>
    </div>
    <ProjectRegistration open={showRegistration} request={request} onRegistered={onRegistered}/>
    {registered && <p className="detail-notice" role="status">{registered.method === 'clone' ? '仓库已克隆并登记。' : '仓库已登记。'}{' '}
      <a href={`#/projects/${registered.project.id}`}>打开 {projectDisplayName(registered.project)}</a>
      {registered.method === 'clone' && <span className="project-registration-path">克隆位置 <code>{registered.project.path}</code></span>}
    </p>}
    <section className="project-directory-list" aria-label="已登记项目" aria-busy={loading}>
      {error && <div className="panel" role="alert">
        <EmptyState icon="AlertCircle" title="读取项目失败" description={error}>
          <button className="button" disabled={loading} onClick={() => refresh()}>重试</button>
        </EmptyState>
      </div>}
      {page?.items.map(project => {
        const base = `#/projects/${project.id}`
        const name = projectDisplayName(project)
        return <article key={project.id} className="panel project-directory-card">
          <div className="project-card-content">
            <div className="project-card-identity">
              <Icon name="Layers3" size={23}/>
              <div>
                <h2><a href={base}>{name}</a></h2>
                <p className="project-repository"><Icon name="GitBranch" size={15}/><span>{project.repository || project.path}</span></p>
              </div>
            </div>
            <div className="project-card-metrics" aria-label={name + '的工作进度'}>
              {projectMetrics.map(metric => <a key={metric.key} href={base + metric.destination}>
                {metric.label} <strong>{project.metrics?.[metric.key] ?? '—'}</strong>
              </a>)}
            </div>
          </div>
          <a className="project-card-open" href={base} aria-label={'打开项目 ' + name}>打开项目 <Icon name="ArrowRight"/></a>
        </article>
      })}
      {!page && loading && !error && <div className="panel" role="status">
        <EmptyState icon="Layers3" title="正在读取项目…"/>
      </div>}
      {page && !page.items.length && !loading && !error && <div className="panel">
        <EmptyState icon={query ? 'Search' : 'GitBranch'} title={query ? '没有匹配的项目' : '尚未登记仓库'}>
          {query && <button className="button" type="button" onClick={clearSearch}>清除搜索</button>}
        </EmptyState>
      </div>}
      {page?.next_before && <button className="button project-more" disabled={loading} onClick={() => refresh(page.next_before)}>更多项目</button>}
    </section>
  </div>
}
