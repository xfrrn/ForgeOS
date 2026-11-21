import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { ApiError, Icon, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'

export const materialKinds = { material: '导入资料', profile: '项目档案', environment: '环境说明', memory: 'Owner 记忆请求' }
export type MaterialKind = keyof typeof materialKinds
export type MaterialSelection =
  | { source: 'git'; path: string; anchor?: string }
  | { source: 'forge'; key: string; revision?: number }
export type MaterialScope = 'all' | 'git' | 'forge'
type ForgeItem = { key: string; title: string; kind: MaterialKind; revision: number; source: string; excerpt: string }
type GitFile = { path: string; blob: string; excerpt?: string }
export type DocumentTree = { commit: string; branch?: string; documents: GitFile[] }
type Matches = { commit: string; items: GitFile[]; more: boolean }

// Tie the displayed response to its request even during the render before effect cleanup.
export function useLibraryResource<T>(request: ApiRequest, path: string, refresh: number | string = 0) {
  const [result, setResult] = useState<{ path: string; refresh: number | string; data?: T; error?: string; status?: number }>()
  useEffect(() => {
    if (!path) return
    const controller = new AbortController()
    request<T>(path, { signal: controller.signal }).then(data => {
      if (!controller.signal.aborted) setResult({path, refresh, data})
    }).catch(reason => {
      if (!controller.signal.aborted) setResult({path, refresh, error: reason instanceof Error ? reason.message : '读取失败',
        status: reason instanceof ApiError ? reason.status : undefined})
    })
    return () => controller.abort()
  }, [request, path, refresh])
  return result?.path === path && result.refresh === refresh ? result : undefined
}

type Folder = { children: Map<string, Folder>; files: GitFile[] }
function makeTree(files: GitFile[]) {
  const root: Folder = { children: new Map(), files: [] }
  for (const file of files) {
    let folder = root
    for (const part of file.path.split('/').slice(0,-1)) {
      if (!folder.children.has(part)) folder.children.set(part, { children: new Map(), files: [] })
      folder = folder.children.get(part)!
    }
    folder.files.push(file)
  }
  return root
}
type FolderProps = {folder: Folder; prefix: string; selectedPath?: string; renderFile: (file: GitFile) => ReactNode}
function FolderBranch({folder,prefix,selectedPath,renderFile}: FolderProps) {
  const containsSelection = selectedPath?.startsWith(prefix+'/') || false
  const [open,setOpen] = useState(containsSelection || prefix==='docs')
  useEffect(()=>{if (containsSelection) setOpen(true)},[containsSelection])
  return <details className="materials-folder" open={open} onToggle={e=>setOpen(e.currentTarget.open)}>
    <summary>{prefix.split('/').at(-1)}/</summary>
    {open && <div><FolderList folder={folder} prefix={prefix} selectedPath={selectedPath} renderFile={renderFile}/></div>}
  </details>
}
function FolderList({folder,prefix,selectedPath,renderFile}: FolderProps) {
  return <>{folder.files.map(renderFile)}{Array.from(folder.children, ([name, child]) =>
    <FolderBranch key={name} folder={child} prefix={prefix ? prefix+'/'+name : name} selectedPath={selectedPath} renderFile={renderFile}/>)}</>
}

export function MaterialCatalog({ project, request, scope, query, kind, selected, refresh, tree, onSelect, onDefault, onKind, onRetry }: {
  project: number; request: ApiRequest; scope: MaterialScope; query: string; selected?: MaterialSelection; refresh: number
  tree?: { data?: DocumentTree; error?: string }; kind: string; onKind: (kind:string) => void
  onSelect: (item: MaterialSelection) => void; onDefault: (item: MaterialSelection) => void; onRetry: () => void
}) {
  const base = '/projects/' + project
  const [gitLimit, setGitLimit] = useState(25)
  const [cursor, setCursor] = useState('')
  const [previousPages, setPreviousPages] = useState<ForgeItem[]>([])
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (kind && scope === 'forge') params.set('kind', kind)
  if (cursor) params.set('after', cursor)
  const forge = useLibraryResource<ForgeItem[]>(request, scope === 'git' ? '' : base + '/materials' + (params.size ? '?' + params : ''), refresh)
  const searchParams = new URLSearchParams({ q: query })
  const gitSearch = useLibraryResource<Matches>(request, query && tree?.data ? base + '/documents?' + searchParams : '', tree?.data?.commit)
  const files = tree?.data?.documents
  const fullTree = useMemo(() => makeTree(files || []), [files])
  const gitRows = useMemo(() => {
    if (!files) return []
    if (!query) {
      const readme = files.find(file=>/^README(?:\.|$)/i.test(file.path))
      return readme ? [readme, ...files.filter(file=>file!==readme)] : files
    }
    const hits = new Map<string, GitFile>()
    for (const hit of gitSearch?.data?.items || []) hits.set(hit.path, hit)
    for (const file of files) if (!hits.has(file.path) && file.path.toLowerCase().includes(query.toLowerCase())) hits.set(file.path, file)
    return Array.from(hits.values())
  }, [files, query, gitSearch?.data])
  const forgeRows = useMemo(() => [...previousPages, ...(forge?.data || [])], [previousPages, forge?.data])
  const firstForge = forgeRows[0]
  const firstGit = gitRows[0]
  const commit = tree?.data?.commit
  useEffect(() => {
    if (selected) return
    if (firstGit) onDefault({source:'git', path:firstGit.path})
    else if (firstForge) onDefault({source:'forge', key:firstForge.key})
  }, [selected, onDefault, firstForge, firstGit])

  function gitRow(file: GitFile) {
    const name = file.path.split('/').at(-1)!
    return <button type="button" key={'git/'+file.path} className="materials-row" aria-current={selected?.source === 'git' && selected.path === file.path || undefined}
      onClick={() => onSelect({source:'git', path:file.path})}>
      <Icon name="FileCode2"/><span><strong>{name}</strong><small>Git 仓库 · {file.path}</small>{query && file.excerpt && <span className="materials-excerpt">{file.excerpt}</span>}</span>
    </button>
  }
  function forgeRow(item: ForgeItem) {
    return <button type="button" key={'forge/'+item.key} className="materials-row" aria-current={selected?.source === 'forge' && selected.key === item.key || undefined}
      onClick={() => onSelect({source:'forge', key:item.key})}>
      <Icon name="Inbox"/><span><strong>{item.title}</strong><small>ForgeOS · {materialKinds[item.kind]} · 修订 {item.revision}</small>
        {query && <span className="materials-excerpt">{item.excerpt}</span>}</span>
    </button>
  }
  function loadMoreForge() {
    setPreviousPages(forgeRows)
    setCursor(forgeRows[forgeRows.length - 1].key)
  }
  const gitLoading = scope !== 'forge' && (!tree || Boolean(query && tree.data && !gitSearch))
  const forgeLoading = scope !== 'git' && !forge
  const gitError = tree?.error || gitSearch?.error
  const rootHeading = query ? '搜索结果' : scope === 'git' ? '仓库目录' : scope === 'forge' ? 'ForgeOS 资料' : '全部资料'
  const bothSources: ReactNode[] = []
  if (scope === 'all') {
    for (let i = 0; i < Math.max(forgeRows.length, Math.min(gitLimit,gitRows.length)); i++) {
      if (i < gitLimit && gitRows[i]) bothSources.push(gitRow(gitRows[i]))
      if (forgeRows[i]) bothSources.push(forgeRow(forgeRows[i]))
    }
  }
  return <aside className="materials-catalog" aria-label="资料目录">
    <h2>{rootHeading}</h2>
    {scope === 'all' && (tree?.data || forge?.data) && <p className="materials-counts">
      {tree?.data && (!query || gitSearch?.data) && <span>Git 仓库 {gitRows.length}{gitSearch?.data?.more ? '+' : ''}</span>}
      {forge?.data && <span>ForgeOS {forgeRows.length}{forge.data.length===30 ? '+' : ''}</span>}
    </p>}
    {scope === 'forge' && <ThemedSelect label="筛选资料类型" value={kind} options={[{value:'',label:'全部类型'},...Object.entries(materialKinds).map(([value,label])=>({value,label}))]}
      onChange={onKind}/>}
    {scope === 'git' && commit && <p className="materials-tree-head"><Icon name="GitBranch" size={14}/>{tree?.data?.branch || '提交'} · <code>{commit.slice(0,9)}</code></p>}
    {(gitLoading || forgeLoading) && <p className="muted" role="status">正在读取{gitLoading && !forgeLoading ? '仓库文档' : '资料'}…</p>}
    {gitError && <p role="alert">仓库文档 · {gitError} <button className="button" onClick={onRetry}>重试</button></p>}
    {forge?.error && <p role="alert">ForgeOS · {forge.error} <button className="button" onClick={onRetry}>重试</button></p>}
    {!gitLoading && !forgeLoading && !gitError && !forge?.error && !gitRows.length && !forgeRows.length &&
      <p className="muted">{query || kind ? '没有匹配的资料。' : '这里还没有资料。'}</p>}
    {scope === 'all' && bothSources}
    {scope === 'forge' && (!query && !kind
      ? Object.entries(materialKinds).map(([value,label]) => {
        const group = forgeRows.filter(item => item.kind === value)
        return group.length > 0 && <section key={value} className="materials-group"><h3>{label}</h3>{group.map(forgeRow)}</section>
      }) : forgeRows.map(forgeRow))}
    {forge?.data?.length === 30 && !query && <button className="button" onClick={loadMoreForge}>更多 ForgeOS 资料</button>}
    {scope === 'git' && !query ? <FolderList folder={fullTree} prefix="" selectedPath={selected?.source==='git' ? selected.path : undefined} renderFile={gitRow}/> : <>
      {scope === 'git' && gitRows.slice(0,gitLimit).map(gitRow)}
      {gitRows.length > gitLimit && <button className="button" onClick={()=>setGitLimit(n=>n+25)}>更多仓库文档</button>}
    </>}
    {gitSearch?.data?.more && <p className="muted">正文匹配较多，可缩小关键词范围。</p>}
  </aside>
}
