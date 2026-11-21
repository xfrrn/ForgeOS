import { useEffect, useRef, useState, type FormEvent } from 'react'
import { materialKinds, type MaterialKind } from './MaterialCatalog'
import { ApiError, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'

export type MaterialDraft = { key: string; kind: MaterialKind; title: string; source: string; content: string }
export type StoredMaterial = MaterialDraft & { revision: number; imported_at: string; content_sha256: string }

export function MaterialEditor({project, initial, request, onSave, onClose}: {
  project: number; initial: MaterialDraft & {revision?: number}; request: ApiRequest
  onSave: (item: StoredMaterial) => void; onClose: () => void
}) {
  const [draft, setDraft] = useState<MaterialDraft>(initial)
  const [previous, setPrevious] = useState(initial.revision || 0)
  const [conflict, setConflict] = useState<StoredMaterial>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const controller = useRef<AbortController | undefined>(undefined)
  useEffect(() => () => controller.current?.abort(), [])
  async function save(event: FormEvent) {
    event.preventDefault()
    const bytes = (value: string) => new TextEncoder().encode(value).length
    if (!/^[\p{L}\p{N}][\p{L}\p{N}._-]{0,95}$/u.test(draft.key)) { setError('资料标识须以字母或数字开头，最多 96 字。'); return }
    if ([draft.title,draft.source,draft.content].some(value=>!value.trim() || value.includes('\0')) ||
      bytes(draft.content)>256*1024 || bytes(draft.title)>256 || bytes(draft.source)>2048) {
      setError('请检查标题、来源和正文。正文最多 256 KiB，标题最多 256 字节，来源最多 2048 字节。'); return
    }
    const path = '/projects/'+project+'/materials/'+encodeURIComponent(draft.key)
    const {kind,title,source,content} = draft
    const signal = (controller.current = new AbortController()).signal
    setError(''); setBusy(true)
    try {
      const saved = await request<StoredMaterial>(path, {method:'PUT', signal, body:JSON.stringify({previous_revision:previous,kind,title,source,content})})
      if (!signal.aborted) onSave(saved)
    } catch (err) {
      if (signal.aborted) return
      if (err instanceof ApiError && err.status === 409) {
        setError('已有其他修订，草稿已保留。')
        try {
          const current = await request<StoredMaterial>(path,{signal})
          if (!signal.aborted) setConflict(current)
        } catch { if (!signal.aborted) setError('无法读取冲突版本，请重试保存。') }
      } else setError(err instanceof Error ? err.message : '保存失败。')
    } finally { if (!signal.aborted) setBusy(false) }
  }
  return <form className="panel detail-card material-form" onSubmit={save}>
    <h2>{initial.revision ? '编辑资料' : draft.kind === 'instruction' ? '写项目指令' : draft.kind === 'memory' ? '记住一条项目经验' : '手动建档'}</h2>
    <fieldset disabled={busy}>
      <label>资料标识<input required readOnly={Boolean(initial.revision)} value={draft.key} onChange={e=>{setDraft({...draft,key:e.target.value});setConflict(undefined);setPrevious(0)}}/></label>
      <ThemedSelect label="资料类型" value={draft.kind} options={Object.entries(materialKinds).map(([value,label])=>({value,label}))} onChange={kind=>setDraft({...draft,kind:kind as MaterialKind})}/>
      <label>标题<input required value={draft.title} onChange={e=>setDraft({...draft,title:e.target.value})}/></label>
      <label>来源<input required value={draft.source} onChange={e=>setDraft({...draft,source:e.target.value})}/></label>
      {draft.kind === 'instruction' && <p>保存后，项目里的 CLI 可按需检索这份 Markdown 指令。</p>}
      <label>正文<textarea required rows={12} value={draft.content} onChange={e=>setDraft({...draft,content:e.target.value})}/></label>
      {error && <p role="alert">{error}</p>}
      {conflict && <section><h3>当前最新版 · {conflict.title}</h3><pre className="material-content">{conflict.content}</pre>
        <button className="button" type="button" onClick={()=>{setPrevious(conflict.revision);setConflict(undefined);setError('')}}>以修订 {conflict.revision} 为基础继续编辑</button></section>}
      <div className="detail-actions"><button className="button primary" disabled={!!conflict}>{draft.kind==='memory' && !previous ? '提交记忆请求' : '保存修订 '+(previous+1)}</button>
        <button className="button" type="button" onClick={onClose}>取消编辑</button></div>
    </fieldset>
  </form>
}
