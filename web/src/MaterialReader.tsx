import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { Element, Root, RootContent } from 'hast'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import './material-reader.css'

type MaterialReaderProps = {
  content: string
  markdown: boolean
  onDocumentLink?: (relative: string) => void
  initialAnchor?: string
}

function headingText(node: RootContent): string {
  if (node.type === 'text') return node.value
  if (node.type !== 'element') return ''
  return node.children.map(headingText).join('')
}

// Author-supplied anchors are local values, never global DOM ids, selectors or HTML.
function documentAnchors(scope: string) {
  return (tree: Root) => {
    const used = new Set<string>()
    const suffixes = new Map<string, number>()
    function mark(element: Element, name: string) {
      if (!name) return
      let anchor = name, suffix = suffixes.get(name) || 0
      while (used.has(anchor)) anchor = name + '-' + (++suffix)
      suffixes.set(name, suffix)
      used.add(anchor)
      element.properties.dataMaterialAnchor = anchor
      element.properties.tabIndex = -1
    }
    function walk(parent: Root | Element) {
      for (const [i, child] of parent.children.entries()) {
        if (child.type === 'raw') {
          // Common Markdown extensions use a bare <a id="…"></a> to name the next section.
          const match = /^<a\s+(?:id|name)\s*=\s*(["'])([^"'<>\s]+)\1\s*>\s*(?:<\/a>)?$/i.exec(child.value.trim())
          if (match) {
            const marker: Element = {type:'element', tagName:'span', properties:{}, children:[]}
            mark(marker, match[2])
            parent.children[i] = marker
          }
        } else if (child.type === 'element') {
          const {id, ariaDescribedBy} = child.properties
          if (typeof id === 'string') {
            // GFM supplies ids for footnote links and definitions. Keep their labels within this reader.
            mark(child, id)
            child.properties.id = scope + id
          } else if (/^h[1-6]$/.test(child.tagName)) {
            const slug = headingText(child).trim().toLowerCase().replace(/[^\p{L}\p{M}\p{N}\s_-]/gu, '').replace(/ /g, '-')
            mark(child, slug)
          }
          if (Array.isArray(ariaDescribedBy)) child.properties.ariaDescribedBy = ariaDescribedBy.map(value => scope + value)
          walk(child)
        }
      }
    }
    walk(tree)
  }
}

const previewLimit = 48 * 1024
function openingSection(content: string) {
  if (content.length <= previewLimit) return content
  const paragraph = content.lastIndexOf('\n\n', previewLimit)
  if (paragraph > previewLimit / 2) return content.slice(0, paragraph)
  const cut = /[\uD800-\uDBFF]/.test(content[previewLimit - 1]) ? previewLimit - 1 : previewLimit
  return content.slice(0, cut)
}

export function MaterialReader({ content, markdown, onDocumentLink, initialAnchor }: MaterialReaderProps) {
  const [source, setSource] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [jump, setJump] = useState<{anchor: string}>()
  const [error, setError] = useState('')
  const scope = useId() + '-'
  const prose = useRef<HTMLDivElement>(null)
  const displayed = expanded ? content : openingSection(content)
  const partial = displayed.length < content.length
  useEffect(() => {
    setJump(initialAnchor ? {anchor:initialAnchor} : undefined)
    setError('')
  }, [initialAnchor, content])
  useEffect(() => {
    if (!jump || source || !prose.current) return
    const target = Array.from(prose.current.querySelectorAll<HTMLElement>('[data-material-anchor]'))
      .find(element => element.dataset.materialAnchor === jump.anchor)
    if (!target && partial) { setExpanded(true); return }
    if (!target) { setError('当前版本中找不到章节「' + jump.anchor + '」。'); return }
    setError('')
    target.focus({preventScroll:true})
    target.scrollIntoView({block:'start'})
  }, [jump, partial, source, displayed])

  const preview = useMemo(() =>
      <Markdown skipHtml remarkPlugins={[remarkGfm]} rehypePlugins={[[documentAnchors, scope]]} components={{
        a({ href, title, children, id, node, 'aria-label': label, 'aria-describedby': description }) {
          if (href && /^(https?:\/\/|mailto:)/i.test(href)) {
            return <a href={href} title={title} target="_blank" rel="noopener noreferrer">{children}</a>
          }
          if (href && href.length > 1 && href.startsWith('#')) {
            return <button type="button" className="material-reader-link" title={title || href} id={id}
              data-material-anchor={node?.properties.dataMaterialAnchor} aria-label={label} aria-describedby={description} onClick={() => {
              try { setJump({anchor:decodeURIComponent(href.slice(1))}) }
              catch { setError('无法解析章节链接。') }
            }}>{children}</button>
          }
          if (href && onDocumentLink && !/^(?:[a-z][a-z\d+.-]*:|\/\/|[?#])/i.test(href) && !href.includes('\\')) {
            return <button type="button" className="material-reader-link" title={title || href}
              onClick={() => onDocumentLink(href)}>{children}</button>
          }
          return <span title={title || href}>{children}</span>
        },
        img({ alt, title }) {
          return <span className="material-reader-image" title={title}>{alt ? `图片 · ${alt}` : '图片'}</span>
        },
        table({ children }) {
          return <div className="material-reader-table"><table>{children}</table></div>
        },
      }}>{displayed}</Markdown>, [displayed, onDocumentLink, scope])
  const original = <pre className="material-reader-original">{content}</pre>
  if (!markdown) return original
  return <div className="material-reader">
    <div className="material-reader-modes" role="group" aria-label="正文显示方式">
      <button type="button" className="button" aria-pressed={!source} onClick={() => setSource(false)}>预览</button>
      <button type="button" className="button" aria-pressed={source} onClick={() => setSource(true)}>原文</button>
    </div>
    {error && <p role="status">{error}</p>}
    {source ? original : <><div ref={prose} className="material-reader-prose">{preview}</div>
      {partial && <button type="button" className="button" onClick={()=>setExpanded(true)}>展开完整预览</button>}</>}
  </div>
}
