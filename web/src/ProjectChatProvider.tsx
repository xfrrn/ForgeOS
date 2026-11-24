import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { ThemedSelect } from './ThemedSelect'
import { projectHref, type ApiRequest } from './shared'

const endpoint = '/projects/chat-models'
export type NativeModelOption = {id: string; label?: string}
export type ChatModel = {
  id: string; revision: string; name: string; model: string; base_url: string; output_format: string
  backend?: 'claude' | 'codex'; available_models?: NativeModelOption[]
}

const presets = [
  {model: 'deepseek-flash', provider: 'DeepSeek', base_url: 'https://api.deepseek.com', output_format: 'json_object'},
  {model: 'deepseek-v4-pro', provider: 'DeepSeek', base_url: 'https://api.deepseek.com', output_format: 'json_object'},
  {model: 'gpt-5.4', provider: 'OpenAI', base_url: 'https://api.openai.com/v1', output_format: 'json_schema'},
  {model: 'gpt-5.4-mini', provider: 'OpenAI', base_url: 'https://api.openai.com/v1', output_format: 'json_schema'},
  {model: 'gpt-4.1', provider: 'OpenAI', base_url: 'https://api.openai.com/v1', output_format: 'json_schema'},
  {model: 'gpt-4.1-mini', provider: 'OpenAI', base_url: 'https://api.openai.com/v1', output_format: 'json_schema'},
  {model: 'qwen-plus', provider: '阿里云百炼 · 北京', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', output_format: 'json_object'},
]
type Preset = typeof presets[number]

export function useChatModels(request: ApiRequest) {
  const [models, setModels] = useState<ChatModel[]>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const refresh = useCallback(() => setRevision(n => n + 1), [])
  useEffect(() => {
    const controller = new AbortController()
    request<ChatModel[]>(endpoint, {signal: controller.signal}).then(list => {
      if (!controller.signal.aborted) {setModels(list); setError('')}
    }).catch(e => {if (!controller.signal.aborted) setError(e.message || '无法读取模型列表。')})
    return () => controller.abort()
  }, [request, revision])
  useEffect(() => {
    const onVisible = () => {if (!document.hidden) refresh()}
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [refresh])
  return {models, error, refresh}
}

type FeatureClient = {id:string;instance:string;backend:'claude'|'codex';model:string;available_models?:NativeModelOption[]}
export function useDiscussionModels(request:ApiRequest,projectId?:number) {
  const catalog=useChatModels(request)
  const [clients,setClients]=useState<FeatureClient[]>([])
  const [clientError,setClientError]=useState('')
  useEffect(()=>{
    if (!projectId) return
    const controller=new AbortController()
    async function refresh() {
      try {
        const items=await request<FeatureClient[]>('/projects/'+projectId+'/features/clients',{signal:controller.signal})
        if (!controller.signal.aborted){setClients(items);setClientError('')}
      } catch(e){if (!controller.signal.aborted) setClientError(e instanceof Error?e.message:'无法读取节点客户端')}
    }
    void refresh()
    window.addEventListener('forgeos-discussion-clients', refresh)
    const timer=window.setInterval(refresh,30000)
    return ()=>{window.removeEventListener('forgeos-discussion-clients',refresh);window.clearInterval(timer);controller.abort()}
  },[projectId,request])
  const native=clients.map(c=>({
    id:c.id,
    name:(c.backend==='claude'?'Claude Code':'Codex')+' · '+c.model,
    model:c.model,
    base_url:c.instance,
    revision:'',
    output_format:'native',
    backend:c.backend,
    available_models:c.available_models?.length ? c.available_models : [{id:c.model,label:c.model}],
  }))
  return {...catalog,error:catalog.error||clientError,models:projectId && (native.length||catalog.models)?[...native,...catalog.models||[]]:catalog.models}
}

export function ProjectModelSettings({request, projectId, fromKnowledge}: {request: ApiRequest; projectId?: number; fromKnowledge?:boolean}) {
  const {models, error, refresh} = useChatModels(request)
  const [editing, setEditing] = useState<ChatModel | 'new'>()
  const [removing, setRemoving] = useState<ChatModel>()
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState('')
  async function remove(model: ChatModel) {
    setBusy(true); setProblem('')
    try {
      await request(endpoint, {method: 'DELETE', body: JSON.stringify({id: model.id, revision: model.revision})})
      setRemoving(undefined); refresh()
    } catch (e) {setProblem(e instanceof Error ? e.message : '移除失败。'); refresh(); setRemoving(undefined)}
    finally {setBusy(false)}
  }

  return <section className="panel detail-card" aria-label="模型连接">
    <h2>模型连接</h2>
    <p>模型连接由各项目共用。保存后，可在项目聊天和知识库中选择。</p>
    <div className="detail-actions"><button className="button" disabled={!models || !!editing || busy} onClick={() => {setRemoving(undefined); setEditing('new'); setProblem('')}}>添加模型</button>
      {projectId && <a className="button" href={projectHref(projectId, fromKnowledge ? '/knowledge/model-settings' : '/chat')}>{fromKnowledge ? '返回知识库模型设置' : '返回项目聊天'}</a>}</div>
    {error && <p role="alert">{error} <button className="button" onClick={refresh}>重新读取</button></p>}
    {!models && !error && <p role="status">正在读取模型…</p>}
    {models?.length === 0 && !editing && <p>尚未添加模型连接。</p>}
    {editing && <ModelForm key={editing === 'new' ? 'new' : editing.id} model={editing === 'new' ? undefined : editing} models={models || []} request={request}
      onCancel={() => {setEditing(undefined); refresh()}} onSaved={() => {setEditing(undefined); refresh()}}/>}
    {models?.map(model => <article className="credential-row" key={model.id}>
      <h3>{model.model}</h3><p>{model.base_url}</p>
      {model.id === removing?.id ? <><p>移除将影响使用此连接的项目聊天和知识库。</p>
        <div className="detail-actions"><button className="button" disabled={busy} onClick={() => void remove(removing)}>确认移除</button>
          <button className="button" disabled={busy} onClick={() => setRemoving(undefined)}>取消</button></div></> :
        <div className="detail-actions"><button className="button" disabled={!!editing || busy} onClick={() => {setEditing(model); setRemoving(undefined); setProblem('')}}>编辑</button>
          <button className="button" disabled={!!editing || busy} onClick={() => {setRemoving(model); setProblem('')}}>移除</button></div>}
    </article>)}
    {problem && <p role="alert">{problem}</p>}
  </section>
}

type ModelFormProps = {model?: ChatModel; request: ApiRequest; onSaved: () => void; onCancel: () => void}

function ModelForm({models, ...props}: ModelFormProps & {models: ChatModel[]}) {
  const existing = props.model
  const available = presets.filter(p => !models.some(m => m.id !== existing?.id && m.model === p.model && m.base_url === p.base_url))
  const [selection, setSelection] = useState(existing ? 'custom' : available[0]?.model || 'custom')
  const preset = available.find(p => p.model === selection)
  const [busy, setBusy] = useState(false)
  return <div className="credential-form">
    {!existing && <ThemedSelect label="选择模型" value={selection} onChange={setSelection} disabled={busy}
      options={[...available.map(p => ({value: p.model, label: p.model, group: p.provider})), {value: 'custom', label: '自定义模型'}]}/>}
    <ModelFields key={selection} {...props} preset={preset} busy={busy} setBusy={setBusy}/>
  </div>
}

function ModelFields({model, preset, request, onSaved, onCancel, busy, setBusy}: ModelFormProps & {
  preset?: Preset; busy: boolean; setBusy: (value: boolean) => void
}) {
  const [format, setFormat] = useState(model?.output_format || 'json_object')
  const [baseURL, setBaseURL] = useState(model?.base_url || '')
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState('')
  const [testing, setTesting] = useState(false)
  const [tested, setTested] = useState('')
  const probe = useRef<AbortController | null>(null)
  useEffect(() => () => {probe.current?.abort()}, [])
  function clearResult() {setTested(''); setError('')}
  const keyRequired = !model || baseURL.trim().replace(/\/+$/, '') !== model.base_url
  const keyValid = (!keyRequired && !apiKey) || (!!apiKey && !/\s/.test(apiKey))
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!keyValid || busy) return
    const fields = Object.fromEntries(new FormData(event.currentTarget))
    const connection = preset ? {model: preset.model, base_url: preset.base_url, output_format: preset.output_format} : fields
    const test = (event.nativeEvent as SubmitEvent).submitter?.getAttribute('value') === 'test'
    const payload = JSON.stringify({...connection, api_key: apiKey, id: model?.id, revision: model?.revision})
    clearResult(); setBusy(true); setTesting(test)
    const controller = new AbortController()
    if (test) probe.current = controller
    try {
      if (test) {
        await request(endpoint + '/test', {method: 'POST', body: payload, signal: controller.signal})
        setTested('连接成功，' + connection.model + ' 已返回有效的测试回复。')
      } else {
        await request(endpoint, {method: model ? 'PUT' : 'POST', body: payload})
        onSaved()
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(e instanceof Error ? e.message : test ? '连接测试失败。' : '保存失败。')
    } finally {probe.current = null; setBusy(false); setTesting(false)}
  }
  return <form onSubmit={submit} onChange={clearResult}>
    <fieldset disabled={busy}>
      {preset ? <p>{preset.base_url}</p> : <>
        <label>模型 ID<input name="model" defaultValue={model?.model} required maxLength={128} autoComplete="off" spellCheck={false}/></label>
        <label>Base URL<input type="url" name="base_url" value={baseURL} onChange={e => setBaseURL(e.target.value)} required placeholder="https://api.example.com/v1" spellCheck={false}/></label>
        <ThemedSelect label="回复格式" name="output_format" value={format} disabled={busy} onChange={value => {setFormat(value); clearResult()}}
          options={[{value:'json_object', label:'JSON 对象'}, {value:'json_schema', label:'严格 JSON Schema'}]}/>
      </>}
      <label>API Key<input name="api_key" type="password" value={apiKey} onChange={e => setApiKey(e.target.value.trim())} required={keyRequired}
        placeholder={keyRequired ? undefined : '留空保留现有密钥'} maxLength={4096} autoComplete="new-password"/></label>
      <div className="detail-actions"><button disabled={!keyValid || busy} className="button primary">{busy && !testing ? '正在保存…' : '保存模型'}</button>
        <button type="submit" value="test" disabled={!keyValid || busy} className="button">{testing ? '正在测试…' : '测试连接'}</button>
        <button className="button" type="button" onClick={onCancel}>取消</button></div>
    </fieldset>
    {tested && <p role="status">{tested}</p>}
    {error && <p role="alert">{error}</p>}
  </form>
}
