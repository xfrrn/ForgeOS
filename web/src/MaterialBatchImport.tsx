import { useEffect, useId, useRef, useState } from 'react'
import { MaterialReader } from './MaterialReader'
import { ThemedSelect } from './ThemedSelect'
import { ApiError, Icon, projectHref, type ApiRequest } from './shared'
import './material-import.css'

type Receipt = { key: string; revision: number }
type Header = Receipt & { kind: string; title: string; source: string; excerpt?: string; content?: string; content_sha256?: string }
type Props = { project: number; request: ApiRequest; onClose: () => void; onSaved: (item: Receipt) => void }
type Draft = {
  id: string; name: string; key: string; title: string; source: string; content: string
  choice: string; digest?: string; invalid?: string; error?: string; saved?: Receipt
}
type Plan = { row: Draft; label: string; key?: string; previous?: Header; matches: Header[]; conflict?: boolean; duplicate?: Receipt }
const limit = 256 * 1024
const bytes = (text: string) => new TextEncoder().encode(text).length
const pasteSource = '工作台粘贴'
const normalizedName = (s: string) => s.normalize('NFC')
const token = (doc: Receipt) => doc.key + '@' + doc.revision
const message = (e: unknown) => e instanceof Error ? e.message : '读取失败，请重试。'

function newDraft(name: string, source: string): Draft {
  const title = source === pasteSource ? name : name.replace(/\.(md|markdown|txt)$/i, '') || name
  const prefix = source === pasteSource ? 'paste-' : 'file-'
  const identity = normalizedName(source === pasteSource ? name : source)
  let key = prefix + identity
  if (!/^[\p{L}\p{N}._-]{1,96}$/u.test(key)) {
    // Preserve the entire input in the fingerprint when its name cannot be a key.
    let hash = 14695981039346656037n
    for (const byte of new TextEncoder().encode(identity)) hash = BigInt.asUintN(64, (hash ^ BigInt(byte)) * 1099511628211n)
    const stem = Array.from(identity.replace(/[^\p{L}\p{N}._-]+/gu, '-')).slice(0, 68).join('')
    key = prefix + stem + '-' + hash.toString(16).padStart(16, '0')
  }
  const id = Array.from(crypto.getRandomValues(new Uint32Array(3)), n => n.toString(16).padStart(8, '0')).join('')
  return { id, name, title, source, key, content: '', choice: '' }
}

function inferredTitle(body: string) {
  const line = body.split(/\r\n?|\n/).find(line => line.trim())?.trim() ?? ''
  const title = line.replace(/^#{1,6}(?:\s+|$)/, '').trim() || '粘贴资料'
  const { read } = new TextEncoder().encodeInto(title, new Uint8Array(256))
  return title.slice(0, read).trimEnd()
}

function sameIdentity(doc: Header, row: Draft) {
  if (doc.key === row.key) return true
  if (doc.kind !== 'material' || normalizedName(doc.source) !== normalizedName(row.source)) return false
  return row.source !== pasteSource || normalizedName(doc.title) === normalizedName(row.title)
}

async function setContent(row: Draft, raw: string, plain: boolean): Promise<Draft> {
  if (!raw.trim() || raw.includes('\0')) return { ...row, invalid: '文件为空或包含无效字符。' }
  if (bytes(raw) > limit) return { ...row, invalid: '正文超过 256 KiB。' }
  let content = raw
  if (plain) {
    // A literal block also preserves Markdown punctuation and embedded fences in a TXT file.
    const text = raw.replace(/\r\n?/g, '\n')
    let tildes = 2, backticks = 2
    for (const match of text.matchAll(/^ {0,3}(~+|\x60+)[ \t]*$/gm)) {
      if (match[1][0] === '~') tildes = Math.max(tildes, match[1].length)
      else backticks = Math.max(backticks, match[1].length)
    }
    const fence = tildes <= backticks ? '~'.repeat(tildes + 1) : String.fromCharCode(96).repeat(backticks + 1)
    content = fence + '\n' + text + (text.endsWith('\n') ? '' : '\n') + fence
  }
  const data = new TextEncoder().encode(content)
  if (data.length > limit) return { ...row, invalid: '转换后的 Markdown 超过 256 KiB。' }
  if (!crypto.subtle) return { ...row, content }
  try {
    const buffer = await crypto.subtle.digest('SHA-256', data)
    const digest = Array.from(new Uint8Array(buffer), n => n.toString(16).padStart(2, '0')).join('')
    return { ...row, content, digest }
  } catch { return { ...row, content } }
}

async function readFile(file: File): Promise<Draft> {
  const row = newDraft(file.name, file.webkitRelativePath || file.name)
  if (!/\.(md|markdown|txt)$/i.test(file.name)) return { ...row, invalid: '请选择 Markdown 或 TXT 文件。' }
  if (bytes(row.source) > 2048) return { ...row, invalid: '来源路径超过 2048 字节，请从较短路径重新选择。' }
  if (file.size > limit) return { ...row, invalid: '文件超过 256 KiB。' }
  try {
    const raw = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer())
    return await setContent(row, raw, /\.txt$/i.test(file.name))
  } catch { return { ...row, invalid: '无法读取 UTF-8 文本，请检查编码后重新选择。' } }
}

// Browse responses include the first 500 Unicode characters. Read the immutable
// revision only for potential duplicates whose excerpt might be truncated.
function incompleteMatch(doc: Header, row: Draft) {
  return !(row.digest && doc.content_sha256) && doc.content === undefined && !!doc.excerpt && row.content.startsWith(doc.excerpt) && Array.from(doc.excerpt).length >= 500
}

function sameContent(doc: Header, row: Draft) {
  if (doc.content_sha256 && row.digest) return doc.content_sha256 === row.digest
  if (doc.content !== undefined) return doc.content === row.content
  return doc.excerpt === row.content && Array.from(doc.excerpt).length < 500
}

function buildPlans(rows: Draft[], catalog: Header[]): Plan[] {
  const targets = new Set<string>()
  const pendingContent = new Map<string, Set<string>>()
  return rows.map(row => {
    const matches = catalog.filter(d => sameIdentity(d, row))
    const plan: Plan = { row, label: '', matches }
    if (row.saved) return { ...plan, label: '已保存 · 修订 ' + row.saved.revision }
    if (row.choice === 'skip') return { ...plan, label: '已选择跳过', conflict: true }
    if (row.invalid) return { ...plan, label: row.invalid }
    if (!row.title.trim() || bytes(row.title.trim()) > 256 || row.title.includes('\0')) return { ...plan, label: '标题须为 1 至 256 个 UTF-8 字节。' }
    const existing = matches.find(d => sameContent(d, row))
    if (existing) return { ...plan, label: '正文相同，跳过 · ' + existing.title, duplicate: existing }
    if (matches.some(d => incompleteMatch(d, row))) return { ...plan, label: '正在核对相同正文…' }
    const bodies = pendingContent.get(row.key) ?? new Set<string>()
    if (bodies.has(row.content)) return { ...plan, label: '与本批同名资料正文相同，跳过' }
    const collides = matches.length > 0 || targets.has(row.key)
    const previous = matches.find(doc => token(doc) === row.choice)
    let key = row.key
    if (row.choice === 'copy') key = 'file-copy-' + row.id
    else if (previous && !targets.has(previous.key)) key = previous.key
    else if (collides) return { ...plan, label: previous ? '前项将更新同一份资料，保存后请核对新版' : targets.has(row.key) ? '本批同名，需选择' : '同名资料，需选择', conflict: true }
    targets.add(key)
    bodies.add(row.content)
    pendingContent.set(row.key, bodies)
    return { ...plan, key, previous, conflict: collides, label: previous ? '将保存修订 ' + (previous.revision + 1) : row.choice === 'copy' ? '将另存为新资料' : '将新建' }
  })
}

export function MaterialBatchImport(props: Props) {
  return <BatchForm key={props.project} {...props}/>
}

function BatchForm({ project, request, onClose, onSaved }: Props) {
  const base = '/projects/' + project + '/materials'
  const heading = useId(), files = useRef<HTMLInputElement>(null), lifetime = useRef<AbortController | null>(null)
  const [catalog, setCatalog] = useState<Header[]>([])
  const [rows, setRows] = useState<Draft[]>([])
  const [loading, setLoading] = useState(true), [busy, setBusy] = useState(false)
  const operation = useRef(false)
  const [catalogError, setCatalogError] = useState(''), [attempt, setAttempt] = useState(0)
  const [probeError, setProbeError] = useState(''), [probeAttempt, setProbeAttempt] = useState(0)
  const [preview, setPreview] = useState(''), [paste, setPaste] = useState(false)
  const [pasteTitle, setPasteTitle] = useState(''), [pasteBody, setPasteBody] = useState(''), [pasteError, setPasteError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [project, request])
  useEffect(() => {
    const controller = new AbortController(), signal = controller.signal
    async function load() {
      setLoading(true); setCatalogError('')
      try {
        const all: Header[] = []
        let page: Header[], after = ''
        do {
          page = await request<Header[]>(base + (after ? '?after=' + encodeURIComponent(after) : ''), { signal })
          all.push(...page)
          after = page.at(-1)?.key ?? ''
        } while (!signal.aborted && page.length === 30)
        if (!signal.aborted) setCatalog(all)
      } catch (e) { if (!signal.aborted) setCatalogError(message(e)) }
      finally { if (!signal.aborted) setLoading(false) }
    }
    void load()
    return () => controller.abort()
  }, [base, request, attempt])

  const pendingProbes = JSON.stringify(catalog.filter(doc => rows.some(row => !row.saved && !row.invalid && row.choice !== 'skip' && sameIdentity(doc, row) && incompleteMatch(doc, row)))
    .map(({ key, revision }) => ({ key, revision })))
  useEffect(() => {
    const controller = new AbortController(), signal = controller.signal
    const needed: Receipt[] = JSON.parse(pendingProbes)
    setProbeError('')
    async function check() {
      try {
        const confirmed = new Map<string, Header>()
        for (const ref of needed) {
          const doc = await request<Header>(base + '/' + encodeURIComponent(ref.key) + '?revision=' + ref.revision, { signal })
          confirmed.set(token(doc), doc)
        }
        if (!signal.aborted && confirmed.size) setCatalog(old => old.map(doc => confirmed.get(token(doc)) ?? doc))
      } catch (e) { if (!signal.aborted) setProbeError(message(e)) }
    }
    void check()
    return () => controller.abort()
  }, [base, request, pendingProbes, probeAttempt])

  function update(id: string, patch: Partial<Draft>) {
    setRows(old => old.map(row => row.id === id ? { ...row, ...patch } : row))
  }
  function remember(doc: Header) {
    setCatalog(old => [...old.filter(d => d.key !== doc.key), doc])
  }
  async function addFiles(list: File[]) {
    if (!list.length || operation.current) return
    const signal = lifetime.current?.signal
    operation.current = true; setBusy(true)
    try {
      const added: Draft[] = []
      for (const file of list) added.push(await readFile(file))
      if (!signal?.aborted) { setRows(old => [...old, ...added]); setPreview(old => old || added[0].id) }
    } finally { operation.current = false; if (!signal?.aborted) setBusy(false) }
  }
  async function addPaste() {
    if (!pasteBody.trim() || operation.current) return
    const signal = lifetime.current?.signal
    operation.current = true; setBusy(true); setPasteError('')
    try {
      const row = await setContent(newDraft(pasteTitle.trim() || inferredTitle(pasteBody), pasteSource), pasteBody, false)
      if (signal?.aborted) return
      if (row.invalid) { setPasteError(row.invalid); return }
      setRows(old => [...old, row]); setPreview(row.id)
      setPaste(false); setPasteTitle(''); setPasteBody('')
    } catch (e) { if (!signal?.aborted) setPasteError(message(e)) }
    finally { operation.current = false; if (!signal?.aborted) setBusy(false) }
  }
  const plans = buildPlans(rows, catalog), ready = plans.filter(plan => plan.key)
  async function save() {
    if (!ready.length || loading || catalogError || operation.current) return
    const signal = lifetime.current?.signal
    operation.current = true; setBusy(true)
    try {
      for (const { row, key, previous } of ready) {
        if (signal?.aborted) break
        const path = base + '/' + encodeURIComponent(key!)
        try {
          update(row.id, { error: undefined })
          const doc = await request<Header>(path, { signal, method: 'PUT', body: JSON.stringify({
            previous_revision: previous?.revision ?? 0, kind: previous?.kind ?? 'material',
            title: row.title.trim(), source: row.source, content: row.content,
          }) })
          if (signal?.aborted) break
          remember(doc); update(row.id, { saved: { key: doc.key, revision: doc.revision } })
          onSaved({ key: doc.key, revision: doc.revision })
        } catch (e) {
          if (signal?.aborted) break
          if (e instanceof ApiError && e.status === 409) {
            update(row.id, { choice: '', error: '目标已有其他修订，请重新核对后选择。' })
            try {
              const latest = await request<Header>(path, { signal })
              if (!signal?.aborted) remember(latest)
            } catch { if (!signal?.aborted) setAttempt(n => n + 1) }
          } else update(row.id, { error: message(e) })
        }
      }
    } finally { operation.current = false; if (!signal?.aborted) setBusy(false) }
  }
  return <section className="panel detail-card material-batch" aria-labelledby={heading}>
    <div className="batch-heading"><h2 id={heading}>导入到 ForgeOS</h2>
      <button type="button" className="button" disabled={busy} onClick={onClose}>完成并关闭</button></div>
    <div className="batch-drop" onDragOver={e => { e.preventDefault(); e.dataTransfer.dropEffect = busy ? 'none' : 'copy' }}
      onDrop={e => { e.preventDefault(); void addFiles(Array.from(e.dataTransfer.files)) }}>
      <Icon name="Inbox" size={22}/><div><strong>将文件拖到这里</strong><p>Markdown 或 UTF-8 纯文本，每份最多 256 KiB。</p></div>
      <input ref={files} type="file" accept=".md,.markdown,.txt" multiple hidden onChange={e => {
        const chosen = Array.from(e.target.files ?? []); e.target.value = ''; void addFiles(chosen)
      }}/>
      <button type="button" className="button" disabled={busy} onClick={() => files.current?.click()}>选择文件</button>
      <button type="button" className="button" disabled={busy} aria-expanded={paste} onClick={() => setPaste(!paste)}>粘贴 Markdown</button>
    </div>
    {paste && <div className="batch-paste"><label>粘贴资料标题（可选）<input value={pasteTitle} disabled={busy} placeholder="默认采用正文首行" onChange={e => setPasteTitle(e.target.value)}/></label>
      <label>Markdown 正文<textarea rows={5} value={pasteBody} disabled={busy} onChange={e => setPasteBody(e.target.value)}/></label>
      {pasteError && <p className="batch-error" role="alert">{pasteError}</p>}
      <button type="button" className="button" disabled={busy || !pasteBody.trim()} onClick={() => void addPaste()}>加入待导入列表</button></div>}
    {loading && <p role="status">正在读取项目资料目录…</p>}
    {catalogError && <p role="alert">无法核对已有资料。{catalogError} <button type="button" className="button" onClick={() => setAttempt(n => n+1)}>重新读取</button></p>}
    {probeError && <p role="alert">无法核对疑似重复的正文。{probeError} <button type="button" className="button" onClick={() => setProbeAttempt(n => n+1)}>重试核对</button></p>}
    {plans.map(({ row, label, conflict, matches, duplicate }) => {
      const opened = preview === row.id, receipt = row.saved || duplicate
      return <article className="batch-file" key={row.id}>
        <div className="batch-heading"><div><strong>{row.name}</strong>
          <p role="status">{loading || catalogError ? row.saved ? label : '等待核对目录' : label}</p></div>
          <div className="batch-actions">{receipt && <a className="button" href={projectHref(project, '/materials?key=' + encodeURIComponent(receipt.key))}>查看已存资料</a>}
            {!row.invalid && <button type="button" className="button" aria-expanded={opened} onClick={() => setPreview(opened ? '' : row.id)}>{opened ? '收起' : '查看草稿'}</button>}
            {!row.saved && <button type="button" className="button" disabled={busy} onClick={() => setRows(old => old.filter(d => d.id !== row.id))}>移除</button>}</div></div>
        {row.error && <p className="batch-error" role="alert">{row.error}</p>}
        {conflict && !row.saved && !duplicate && <><ThemedSelect label="同名资料的处理方式"
          value={row.choice === 'skip' || row.choice === 'copy' || matches.some(doc => token(doc) === row.choice) ? row.choice : ''}
          options={[{ value: '', label: '选择处理方式' }, ...matches.map(d => ({ value: token(d), label: '更新「' + d.title + '」 · ' + d.key + ' · 修订 ' + d.revision })),
            { value: 'copy', label: '另存为新资料' }, { value: 'skip', label: '跳过此项' }]}
          disabled={busy} onChange={choice => update(row.id, { choice, error: undefined })}/>
          {matches.length ? <details><summary>核对已有资料</summary>{matches.map(d => <p key={d.key}>
            <a href={projectHref(project, '/materials?key=' + encodeURIComponent(d.key) + '&revision=' + d.revision)} target="_blank" rel="noreferrer">
              {d.title} · {d.key} · 修订 {d.revision}</a></p>)}</details>
            : row.choice !== 'skip' && <p>要更新本批同名资料，可在前一份保存后选择其修订。</p>}</>}
        {opened && !row.invalid && <div className="batch-preview"><label>资料标题<input value={row.title} disabled={busy || !!row.saved}
          onChange={e => update(row.id, { title: e.target.value, choice: '', error: undefined })}/></label>
          <MaterialReader content={row.content} markdown/></div>}
      </article>
    })}
    {rows.length > 0 && <div className="batch-actions"><button type="button" className="button primary" disabled={busy || loading || !!catalogError || !ready.length}
      onClick={() => void save()}>导入可保存的 {ready.length} 份资料</button>
      {busy && <span role="status">正在处理…</span>}</div>}
  </section>
}
