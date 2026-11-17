import { useEffect, useState } from 'react'
import { ApiError, type ApiRequest } from './shared'
import { RolePermissions, type StructuredProfile } from './RolePermissions'

type Fact = { kind: string; value: string; path: string; quote: string }
type Analysis = { status: string; attempt: number; failure?: string; analysis?: {session_id: string; facts: Fact[]; profile?: StructuredProfile}
  truncated: number; omitted: number; sources: {path: string; blob: string; basis: string}[] }
const sections = { profile: '项目用途', stack: '技术栈', dependency: '依赖', start: '启动命令', test: '测试命令' }

export function RepositoryAnalysis({task, request}: {task: number; request: ApiRequest}) {
  const [result,setResult] = useState<Analysis>()
  const [error,setError] = useState('')
  const [loading,setLoading] = useState(true)
  const [revision,setRevision] = useState(0)
  const endpoint = `/tasks/${task}/repository-analysis`
  useEffect(() => {
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const row = await request<Analysis>(endpoint,{signal:stop.signal})
        if (stop.signal.aborted) return
        setResult(row); setError('')
        if (row.status === 'READY' || row.status === 'RUNNING') timer=setTimeout(refresh,3000)
      } catch(e) {
        if (stop.signal.aborted) return
        if (e instanceof ApiError && e.status===404) setResult(undefined)
        else setError(e instanceof Error ? e.message : '读取归纳结果失败。')
      } finally { if (!stop.signal.aborted) setLoading(false) }
    }
    void refresh()
    return () => { stop.abort(); clearTimeout(timer) }
  },[endpoint,request,revision])
  async function submit() {
    setLoading(true); setError('')
    try {
      const row=await request<Analysis>(endpoint,{method:'POST',body:JSON.stringify({previous_attempt:result?.status==='FAILED' ? result.attempt : 0})})
      setResult(row)
    } catch(e) { setError(e instanceof Error ? e.message : '派发归纳失败。') }
    finally { setLoading(false); setRevision(n=>n+1) }
  }
  return <section><h3>Agent 仓库归纳</h3>
    {(!result || result.status==='FAILED') && <button className="button" disabled={loading} onClick={()=>void submit()}>{result ? '重新归纳' : '归纳入口文档'}</button>}
    {error && <p role="alert">{error}</p>}
    {result?.status==='READY' && <p role="status">等待此项目的 Management Agent 领取。</p>}
    {result?.status==='RUNNING' && <p role="status">正在归纳固定提交的文档和包清单。</p>}
    {result?.status==='FAILED' && <p role="alert">{result.failure==='unverified_evidence' ? '归纳中的引用未通过原文核对。' : 'Agent 本次未返回可用报告。'}</p>}
    {result?.analysis && <>
      <p>Agent 会话 · <code>{result.analysis.session_id}</code></p>
      {result.analysis.profile && <RolePermissions role="Management" profile={result.analysis.profile}/>}
      {Object.entries(sections).map(([kind,title]) => {
        const facts=result.analysis!.facts.filter(f=>f.kind===kind)
        return <section key={kind}><h4>{title}</h4>{facts.length ? facts.map((f,i)=>{
          const source=result.sources.find(s=>s.path===f.path)
          return <details key={i}><summary>{kind==='start' || kind==='test' ? <code>{f.value}</code> : f.value}</summary>
            <p>{f.path} · {source?.basis==='git' ? 'Git 原文' : '归档清单'} · <code>{source?.blob}</code></p><pre>{f.quote}</pre></details>
        }) : <p>未从本次输入确认。</p>}</section>
      })}
      {(result.omitted>0 || result.truncated>0) && <p>输入范围：{result.omitted} 个条目未纳入，{result.truncated} 个条目达到读取上限。</p>}
    </>}
  </section>
}
