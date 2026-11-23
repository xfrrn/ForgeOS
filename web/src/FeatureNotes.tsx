import { useEffect, useRef, useState } from 'react'
import { formatTime, type ApiRequest } from './shared'
import { ThemedSelect } from './ThemedSelect'

type Note = { id: number; turn_id: number; kind: 'agreement' | 'question'; body: string; closed: boolean }
type Source = { text: string; answer: string; created_at: string; requested_model: string; native_backend?: string }
type Props = { path: string; request: ApiRequest }

export function RecordFeatureNote({path, request, turn, onSaved}: Props & {turn: number; onSaved: () => void}) {
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState('agreement')
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const intent = useRef({key: '', id: ''})
  async function save() {
    if (busy || !body.trim()) return
    const key = JSON.stringify([kind, body.trim()])
    if (intent.current.key !== key) intent.current = {key, id: crypto.randomUUID()}
    setBusy(true)
    setMessage('')
    try {
      await request(path, {method: 'POST', body: JSON.stringify({turn_id: turn, kind, body: body.trim(), client_id: intent.current.id})})
      setOpen(false)
      setBody('')
      setMessage('已保存到话题记录')
      intent.current = {key: '', id: ''}
      onSaved()
    } catch (e) { setMessage(e instanceof Error ? e.message : '保存失败，请重试。') }
    finally { setBusy(false) }
  }
  return <div className="feature-note-editor">
    {open ? <form onSubmit={event => {event.preventDefault(); void save()}}>
      <ThemedSelect label="记录类型" value={kind} disabled={busy} onChange={setKind} options={[
        {value:'agreement', label:'已确认的约定'}, {value:'question', label:'待决定的问题'}
      ]}/>
      <label>记录内容<textarea autoFocus required maxLength={1200} value={body} disabled={busy} onChange={e=>setBody(e.target.value)}/></label>
      <div className="detail-actions"><button className="button primary" disabled={busy || !body.trim()}>保存记录</button>
        <button type="button" className="button" disabled={busy} onClick={()=>{setOpen(false); setMessage('')}}>收起</button></div>
    </form> : <button className="button" type="button" onClick={()=>{setOpen(true); setMessage('')}}>从本轮记一条</button>}
    {message && <span role="status">{message}</span>}
  </div>
}

function NoteItem({note, request, path, onClosed}: Props & {note:Note; onClosed:()=>void}) {
  const [source,setSource] = useState<Source>()
  const [busy,setBusy] = useState(false)
  const [error,setError] = useState('')
  async function act(action:'source'|'close') {
    setError('')
    if (action==='source' && source) { setSource(undefined); return }
    setBusy(true)
    try {
      if (action==='source') setSource(await request<Source>(path+'/'+note.id+'/source'))
      else {await request(path+'/'+note.id+'/close',{method:'POST'}); onClosed()}
    } catch(e) {setError(e instanceof Error ? e.message : '操作失败，请重试。')}
    finally {setBusy(false)}
  }
  return <li><strong>{note.kind==='agreement'?'已确认的约定':'待决定的问题'}</strong>
    <p>{note.body}</p>
    <div className="detail-actions"><button type="button" className="button" disabled={busy} aria-expanded={!!source} onClick={()=>void act('source')}>{source?'收起原文':'查看原文'} · #{note.turn_id}</button>
      {!note.closed && <button type="button" className="button" disabled={busy} onClick={()=>void act('close')}>{note.kind==='question'?'标为已处理':'结束此约定'}</button>}</div>
    {source && <blockquote><small>{formatTime(source.created_at)} · 你</small><p>{source.text}</p>
      <small>{source.native_backend==='claude'?'Claude Code · ':source.native_backend==='codex'?'Codex · ':''}{source.requested_model || '模型回复'}</small><p>{source.answer}</p></blockquote>}
    {error && <p role="alert">{error}</p>}
  </li>
}

export function FeatureNotes({path, request, version}: Props & {version:number}) {
  const [items,setItems] = useState<Note[]>()
  const [error,setError] = useState('')
  const [revision,setRevision] = useState(0)
  const [history,setHistory] = useState(false)
  const [query,setQuery] = useState('')
  const refresh = () => setRevision(n=>n+1)
  useEffect(()=>{
    const controller=new AbortController()
    request<Note[]>(path,{signal:controller.signal}).then(data=>{
      if(!controller.signal.aborted){setItems(data);setError('')}
    }).catch(e=>{if(!controller.signal.aborted)setError(e.message)})
    return ()=>controller.abort()
  },[path,request,version,revision])
  const active = items?.filter(n=>!n.closed).length
  const visible = items?.filter(n=>n.closed===history && n.body.includes(query.trim())).slice().reverse()
  return <details className="feature-notes" onToggle={e=>{if(e.currentTarget.open)refresh()}}>
    <summary>话题记录{active !== undefined && ' · '+active+' 条有效'}</summary>
    <div className="feature-notes-content">
      <div className="detail-actions"><input type="search" aria-label="搜索话题记录" placeholder="搜索话题记录" value={query} onChange={e=>setQuery(e.target.value)}/>
        <button className="button" type="button" aria-pressed={history} onClick={()=>setHistory(!history)}>{history?'返回有效记录':'查看已结束记录'}</button></div>
      {error && <p role="alert">{error} <button className="button" onClick={refresh}>重试</button></p>}
      {!items && !error && <p role="status">正在读取话题记录…</p>}
      {visible?.length===0 && <p className="muted">{query?'没有匹配的记录。':history?'暂无已结束的记录。':'在一轮已完成的讨论下，选择「从本轮记一条」保存约定或问题。'}</p>}
      <ul>{visible?.map(note=><NoteItem key={note.id} note={note} path={path} request={request} onClosed={refresh}/>)}</ul>
    </div>
  </details>
}
