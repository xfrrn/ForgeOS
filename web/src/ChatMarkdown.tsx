import { useMemo, useState, type ReactNode } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Icon } from './shared'

function extractText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(extractText).join('')
  if (node && typeof node === 'object' && 'props' in node) {
    return extractText((node as { props?: { children?: ReactNode } }).props?.children)
  }
  return ''
}

function CodeBlock({ children }: { children: ReactNode }) {
  const [copied, setCopied] = useState(false)
  const child = Array.isArray(children) ? children[0] : children
  const className = (child && typeof child === 'object' && 'props' in child
    ? (child as { props?: { className?: string } }).props?.className
    : '') || ''
  const language = /^language-([A-Za-z0-9_+-]+)/.exec(className)?.[1] || ''
  const raw = extractText(children).replace(/\n$/, '')

  async function copy() {
    try {
      await navigator.clipboard.writeText(raw)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch { /* Clipboard may be unavailable in restricted contexts. */ }
  }

  return <div className="chat-code-block">
    <div className="chat-code-head">
      <span className="chat-code-lang">{language || 'code'}</span>
      <button type="button" className="chat-code-copy" onClick={() => void copy()} aria-label="复制代码">
        <Icon name={copied ? 'Check' : 'Copy'} size={13}/>{copied ? '已复制' : '复制'}
      </button>
    </div>
    <pre>{children}</pre>
  </div>
}

export function ChatMarkdown({ content, streaming = false }: { content: string; streaming?: boolean }) {
  const rendered = useMemo(() => (
    <Markdown skipHtml remarkPlugins={[remarkGfm]} components={{
      a({ href, title, children }) {
        if (href && /^(https?:\/\/|mailto:)/i.test(href)) {
          return <a href={href} title={title} target="_blank" rel="noopener noreferrer">{children}</a>
        }
        return <span className="chat-md-ref" title={title || href}>{children}</span>
      },
      img({ alt, title }) {
        return <span className="chat-md-ref" title={title}>{alt ? `[图片：${alt}]` : '[图片]'}</span>
      },
      pre({ children }) {
        return <CodeBlock>{children}</CodeBlock>
      },
      table({ children }) {
        return <div className="chat-md-table"><table>{children}</table></div>
      },
    }}>{content}</Markdown>
  ), [content])

  return <div className={'chat-markdown' + (streaming ? ' chat-streaming' : '')}>{rendered}</div>
}
