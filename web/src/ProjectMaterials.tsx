import { useCallback, useEffect, useRef, useState } from 'react'
import { MaterialBatchImport } from './MaterialBatchImport'
import { MaterialCatalog, materialKinds, useLibraryResource, type DocumentTree, type MaterialScope, type MaterialSelection } from './MaterialCatalog'
import { MaterialEditor, type MaterialDraft, type StoredMaterial } from './MaterialEditor'
import { MaterialReader } from './MaterialReader'
import { KnowledgeCuration } from './KnowledgeCuration'
import { ExperienceSearch } from './ExperienceSearch'
import { RepositoryDocumentChanges } from './RepositoryDocumentChanges'
import { ProjectGit } from './ProjectGit'
import { ShareDraft } from './ProjectShares'
import { Icon, formatTime, projectHref, type ApiRequest } from './shared'
import { PageHeading } from './ui'
import './project-materials.css'

type GitDocument = { path: string; commit: string; blob: string; content: string }
type DisplayedGit = {commit: string} | {missingPath: string}
const sourceOptions = [{value:'all',label:'全部'}, {value:'git',label:'Git 仓库'}, {value:'forge',label:'ForgeOS'}] as const

function MaterialLink({ href }: { href: string }) {
  const [message, setMessage] = useState('')
  const field = useRef<HTMLInputElement>(null)
  const url = new URL(href, window.location.href).href
  async function copy() {
    try { await navigator.clipboard.writeText(url); setMessage('已复制资料链接。') }
    catch { field.current?.focus(); field.current?.select(); setMessage('已选中资料链接，可直接复制。') }
  }
  return <div className="materials-reference"><label>资料链接<input readOnly ref={field} value={url} onFocus={e=>e.target.select()}/></label>
    <button className="button" onClick={copy}>复制资料链接</button>{message && <small role="status">{message}</small>}</div>
}

function GitReader({ project, selection, request, refresh, onRetry, onSelect, onRead }: {
  project: number; selection: Extract<MaterialSelection,{source:'git'}>; request: ApiRequest; onSelect: (item: MaterialSelection)=>void
  refresh: string; onRetry: () => void; onRead: (observed?: DisplayedGit) => void
}) {
  const [linkError, setLinkError] = useState('')
  const params = new URLSearchParams({path:selection.path})
  const result = useLibraryResource<GitDocument>(request,'/projects/'+project+'/documents/read?'+params,refresh)
  const commit = result?.data?.commit
  useEffect(() => {
    if (commit) onRead({commit})
    else if (result?.status === 404) onRead({missingPath:selection.path})
    return () => onRead(undefined)
  },[commit,result?.status,selection.path,onRead])
  if (!result) return <p role="status">正在读取仓库原文…</p>
  if (!result.data) return <p role="alert"><code>{selection.path}</code> · {result.error} <button className="button" onClick={onRetry}>重试</button></p>
  const doc = result.data
  function openLink(target: string) {
    try {
      const origin = 'https://repository.invalid'
      const base = origin+'/'+doc.path.split('/').map(encodeURIComponent).join('/')
      const url = new URL(target,base)
      if (url.origin !== origin) throw new Error()
      const path = decodeURIComponent(url.pathname.slice(1))
      if (!path || path.includes('\\')) throw new Error()
      onSelect({source:'git',path,anchor:decodeURIComponent(url.hash.slice(1)) || undefined})
    } catch { setLinkError('无法解析文档路径。') }
  }
  const name = doc.path.split('/').at(-1)!
  return <>
    <div className="materials-origin"><Icon name="GitBranch" size={15}/> Git 仓库 <code>{doc.commit.slice(0,9)}</code></div>
    <h2 className="materials-title">{name}</h2><p className="materials-path">{doc.path}</p>
    <MaterialReader content={doc.content} markdown={/\.(?:md|mdx|markdown)$/i.test(name) || /^readme$/i.test(name)}
      initialAnchor={selection.anchor} onDocumentLink={openLink}/>
    {linkError && <p role="alert">{linkError}</p>}
    <details className="materials-meta"><summary>资料信息与链接</summary><p>当前读取的提交 <code>{doc.commit}</code></p>
      <MaterialLink href={projectHref(project,'/materials?'+params)}/></details>
  </>
}

function ForgeReader({ project, selection, request, refresh, onRetry, onSelect, onEdit }: {
  project: number; selection: Extract<MaterialSelection,{source:'forge'}>; request: ApiRequest
  refresh: string; onRetry: () => void
  onSelect: (item: MaterialSelection)=>void; onEdit: (item: StoredMaterial)=>void
}) {
  const base = '/projects/'+project+'/materials/'+encodeURIComponent(selection.key)
  const head = useLibraryResource<StoredMaterial>(request,base,refresh)
  // Read a pinned revision independently so its availability does not depend on the list or the current revision.
  const pinned = useLibraryResource<StoredMaterial>(request,selection.revision ? base+'?revision='+selection.revision : '',refresh)
  const result = selection.revision ? pinned : head
  if (!result) return <p role="status">正在读取资料修订…</p>
  if (!result.data) return <p role="alert">{result.error} <button className="button" onClick={onRetry}>重试</button></p>
  const doc = result.data
  const jump = (revision?: number) => onSelect({source:'forge',key:doc.key,revision})
  const href = projectHref(project,'/materials?'+new URLSearchParams({key:doc.key}))
  return <>
    <div className="materials-origin"><Icon name="Inbox" size={15}/> ForgeOS <span>修订 {doc.revision}</span></div>
    <h2 className="materials-title">{doc.title}</h2><p className="materials-path">{materialKinds[doc.kind]} / {doc.source} · {formatTime(doc.imported_at)}</p>
    <MaterialReader content={doc.content} markdown/>
    <div className="detail-actions">
      {doc.revision>1 && <button className="button" onClick={()=>jump(doc.revision-1)}>上一修订</button>}
      {head?.data && head.data.revision>doc.revision && <><button className="button" onClick={()=>jump(doc.revision+1)}>下一修订</button>
        <button className="button" onClick={()=>jump()}>查看最新版</button></>}
      {head?.data?.revision===doc.revision && <button className="button" onClick={()=>onEdit(doc)}>编辑此修订</button>}
    </div>
    {head?.error && <p role="alert">{head.error} <button className="button" onClick={onRetry}>重试读取最新版</button></p>}
    <details className="materials-meta"><summary>资料信息与链接</summary><p>资料标识 <code>{doc.key}</code></p>
      {selection.revision && <p>资料链接打开最新版。</p>}<MaterialLink href={href}/></details>
    <details className="materials-meta"><summary>知识提取与共享</summary>
      <KnowledgeCuration project={project} path={base+'/revisions/'+doc.revision+'/curation'} request={request}/>
      <ShareDraft project={project} material={doc.key} revision={doc.revision} request={request}/>
    </details>
  </>
}

export function ProjectMaterials({ project, initialKey, initialRevision, initialPath, request }: {
  project: number; initialKey?: string; initialRevision?: number; initialPath?: string; request: ApiRequest
}) {
  const [selected, setSelected] = useState<MaterialSelection | undefined>(() => initialKey ? {source:'forge',key:initialKey,revision:initialRevision} :
    initialPath ? {source:'git',path:initialPath} : undefined)
  const [scope, setScope] = useState<MaterialScope>(selected?.source || 'all')
  const [kind,setKind] = useState('')
  const [text, setText] = useState('')
  const [query, setQuery] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [gitRefresh, setGitRefresh] = useState(0)
  const [opened, setOpened] = useState(0)
  const [mobileRead, setMobileRead] = useState(Boolean(selected))
  const [importing, setImporting] = useState(false)
  const [draft, setDraft] = useState<(MaterialDraft & {revision?: number})>()
  const [toolsOpen, setToolsOpen] = useState(false)
  const reader = useRef<HTMLElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const add = useRef<HTMLButtonElement>(null)
  const lastImport = useRef<{key:string; revision:number} | undefined>(undefined)
  const displayedGit = useRef<DisplayedGit | undefined>(undefined)
  const observeRead = useCallback((observed?: DisplayedGit) => {displayedGit.current=observed},[])
  const browseGit = scope !== 'forge' && !importing && !draft
  const tree = useLibraryResource<DocumentTree>(request,browseGit ? '/projects/'+project+'/documents/tree' : '',refresh+'/'+gitRefresh)
  const refreshLibrary = () => setRefresh(n=>n+1)
  const refreshGit = useCallback(() => setGitRefresh(n=>n+1),[])
  useEffect(() => {
    if (!browseGit || !tree) return
    const controller = new AbortController()
    let checking = false
    async function checkHead() {
      if (document.visibilityState !== 'visible' || checking) return
      checking = true
      try {
        const head = await request<{sha:string; branch:string}>('/projects/'+project+'/git/local',{signal:controller.signal})
        if (controller.signal.aborted) return
        const observed = displayedGit.current
        const readerOutdated = observed && ('commit' in observed ? observed.commit !== head.sha :
          tree?.data?.documents.some(file => file.path === observed.missingPath))
        if (head.sha !== tree?.data?.commit || head.branch !== (tree?.data?.branch || '') || readerOutdated) refreshGit()
      } catch {
        if (!controller.signal.aborted && tree?.data) refreshGit()
      } finally { checking = false }
    }
    const interval = window.setInterval(checkHead, 30_000)
    document.addEventListener('visibilitychange',checkHead)
    window.addEventListener('focus',checkHead)
    return () => {
      controller.abort(); window.clearInterval(interval)
      document.removeEventListener('visibilitychange',checkHead); window.removeEventListener('focus',checkHead)
    }
  },[browseGit,tree,project,request,refreshGit])
  const chooseDefault = useCallback((item:MaterialSelection)=>setSelected(old=>old || item),[])
  function select(item: MaterialSelection) {
    setSelected(item);setOpened(n=>n+1);setMobileRead(true)
    if (window.matchMedia('(max-width: 700px)').matches) requestAnimationFrame(()=>reader.current?.focus())
  }
  function changeScope(next: MaterialScope) {
    if (scope === next) return
    setScope(next);setSelected(undefined);setMobileRead(false)
  }
  useEffect(()=>{
    const next = text.trim()
    if(next === query) return
    const timer = window.setTimeout(()=>{setQuery(next);setSelected(undefined);setMobileRead(false)},300)
    return ()=>window.clearTimeout(timer)
  },[text,query])
  function afterSave(item: {key:string; revision:number}) {
    setScope('forge');setKind('');setText('');setQuery('');setSelected({source:'forge',key:item.key});refreshLibrary()
  }
  function newDraft(memory: boolean) {
    const rnd = crypto.getRandomValues(new Uint32Array(3)).join('-')
    setDraft({key:(memory?'memory-':'note-')+rnd,kind:memory?'memory':'material',title:'',source:'Owner 工作台',content:''})
  }
  return <div className="project-setup project-materials">
    <PageHeading title="项目资料" actions={!importing && !draft &&
      <button className="button primary" ref={add} onClick={()=>{lastImport.current=undefined;setImporting(true)}}>添加到 ForgeOS</button>}/>
    {importing ? <MaterialBatchImport project={project} request={request}
      onSaved={item=>{lastImport.current=item}}
      onClose={()=>{setImporting(false);if(lastImport.current){afterSave(lastImport.current);setMobileRead(true)}requestAnimationFrame(()=>add.current?.focus())}}/> :
      draft ? <MaterialEditor project={project} initial={draft} request={request}
        onSave={item=>{setDraft(undefined);afterSave(item);setMobileRead(true)}} onClose={()=>setDraft(undefined)}/> :
      <>
        <section className="panel materials-library" aria-label="项目资料库">
          <div className="materials-toolbar">
            <div className="materials-scopes" role="group" aria-label="资料来源">{sourceOptions.map(item=>
              <button type="button" className="button" key={item.value} aria-pressed={scope===item.value} onClick={()=>changeScope(item.value)}>
                {item.value!=='all' && <Icon size={15} name={item.value==='git'?'GitBranch':'Inbox'}/>} {item.label}</button>)}</div>
            <form role="search" className="materials-search" onSubmit={e=>{e.preventDefault(); if(text.trim()!==query) setSelected(undefined);setQuery(text.trim());setMobileRead(false)}}>
              <input ref={search} type="search" aria-label="检索资料" placeholder="搜索标题、路径或正文" value={text} onChange={e=>setText(e.target.value)}/>
              <button className="button" aria-label="检索"><Icon name="Search"/></button>
              <button type="button" className="button" aria-label="刷新资料" title="刷新资料" onClick={refreshLibrary}><Icon name="RefreshCw"/></button>
            </form>
          </div>
          <div className={'materials-split'+(mobileRead && selected?' reading':'')}>
            <MaterialCatalog key={JSON.stringify([scope,query,kind,refresh])} project={project} request={request} scope={scope} query={query} refresh={refresh} kind={kind}
              selected={selected} tree={scope === 'forge' ? undefined : tree} onSelect={select} onDefault={chooseDefault}
              onKind={value=>{setKind(value);setSelected(undefined);setMobileRead(false)}} onRetry={refreshLibrary}/>
            <section ref={reader} tabIndex={-1} className="materials-reader-pane" aria-label="资料正文">
              <button type="button" className="button materials-back" onClick={()=>{setMobileRead(false);requestAnimationFrame(()=>search.current?.focus())}}>返回目录</button>
              {selected?.source === 'git' ? <GitReader key={selected.path} project={project} selection={selected} request={request} onSelect={select}
                refresh={refresh+'/'+gitRefresh+'/'+opened+'/'+(tree?.data?.commit || '')} onRead={observeRead} onRetry={refreshGit}/> :
                selected?.source === 'forge' ? <ForgeReader key={JSON.stringify(selected)} project={project} selection={selected} request={request} onSelect={select}
                  refresh={refresh+'/'+opened} onRetry={refreshLibrary} onEdit={setDraft}/> :
                <p className="muted">从目录选择一份资料，查看正文。</p>}
            </section>
          </div>
        </section>
        <details className="panel materials-tools" onToggle={e=>setToolsOpen(e.currentTarget.open)}><summary>资料管理与历史</summary>
          {toolsOpen && <div className="project-setup"><div className="detail-actions">
            <button className="button" onClick={()=>newDraft(false)}>手动建档</button>
            <button className="button" onClick={()=>newDraft(true)}>记住一条项目经验</button></div>
            <ExperienceSearch project={project} request={request}/><RepositoryDocumentChanges project={project} request={request}/>
            <ProjectGit projectId={project} request={request} onUpdated={refreshGit}/></div>}
        </details>
      </>}
  </div>
}
