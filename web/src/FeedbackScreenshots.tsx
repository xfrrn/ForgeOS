import { ThemedSelect } from './ThemedSelect'
import { useEffect, useRef, useState, type PointerEvent } from 'react'
import type { ApiRequest } from './shared'

export type FeedbackArtifactReceipt = { id: number; mime: string; bytes: number; sha256: string; expired?: boolean }
type Box = { x: number; y: number; width: number; height: number }
type Mark = Box & { mask: boolean }
const blank = { x: 0, y: 0, width: 0, height: 0 }

function paint(canvas: HTMLCanvasElement, image: ImageBitmap, marks: Mark[]) {
  canvas.width = image.width
  canvas.height = image.height
  const ctx = canvas.getContext('2d')!
  ctx.fillStyle = 'white'
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.drawImage(image, 0, 0)
  for (const m of marks) {
    if (m.mask) {
      ctx.fillStyle = 'black'
      ctx.fillRect(m.x, m.y, m.width, m.height)
    } else {
      ctx.strokeStyle = '#e03929'
      ctx.lineWidth = 3
      ctx.strokeRect(m.x, m.y, m.width, m.height)
    }
  }
}

export function FeedbackScreenshots({ api, artifacts, request, saved, captured }: {
  api: string; artifacts: FeedbackArtifactReceipt[]; request: ApiRequest; saved: () => void; captured?: Blob | null
}) {
  const [open, setOpen] = useState(false)
  const [image, setImage] = useState<ImageBitmap | null>(null)
  const [view, setView] = useState(0)
  const [marks, setMarks] = useState<Mark[]>([])
  const [box, setBox] = useState<Box>(blank)
  const [drag, setDrag] = useState<Box | null>(null)
  const [mask, setMask] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const start = useRef<{ x: number; y: number } | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  useEffect(() => () => image?.close(), [image])
  useEffect(() => {
    if (image && canvas.current) paint(canvas.current, image, drag ? [...marks, { ...drag, mask }] : marks)
  }, [image, marks, drag, mask])
  function reset() {
    setImage(null); setMarks([]); setBox(blank); setDrag(null); setView(0); start.current = null; setError('')
  }
  async function load(file?: Blob, receipt?: FeedbackArtifactReceipt) {
    reset(); setBusy(true)
    try {
      const blob = file ?? await request<Blob>(api + '/' + receipt!.id, {}, 'blob')
      if (blob.size > 16 * 1024 * 1024 || blob.size < 24) throw new Error('请选择 16 MiB 以内的 PNG 文件。')
      const header = new DataView(await blob.slice(0, 24).arrayBuffer())
      if (header.getUint32(0) !== 0x89504e47 || header.getUint32(4) !== 0x0d0a1a0a ||
          header.getUint32(12) !== 0x49484452 || header.getUint32(16) * header.getUint32(20) > 16 * 1024 * 1024) {
        throw new Error('请选择有效的 PNG 截图，像素总数上限为 16 Mi。')
      }
      const bitmap = await createImageBitmap(blob)
      setView(receipt?.id ?? 0); setImage(bitmap)
    } catch (e) { setError(e instanceof Error ? e.message : '无法读取截图。') }
    finally { setBusy(false) }
  }
  useEffect(() => {
    if (captured) { setOpen(true); void load(captured) }
  }, [captured])
  function point(e: PointerEvent<HTMLCanvasElement>) {
    const el = e.currentTarget
    const area = el.getBoundingClientRect()
    return { x: Math.max(0, Math.min(el.width, (e.clientX - area.left) * el.width / area.width)),
      y: Math.max(0, Math.min(el.height, (e.clientY - area.top) * el.height / area.height)) }
  }
  function rect(e: PointerEvent<HTMLCanvasElement>): Box {
    const a = start.current!, b = point(e)
    const x = Math.floor(Math.min(a.x, b.x)), y = Math.floor(Math.min(a.y, b.y))
    return { x, y, width: Math.ceil(Math.max(a.x, b.x)) - x, height: Math.ceil(Math.max(a.y, b.y)) - y }
  }
  function add(b: Box) {
    if (!image || !Object.values(b).every(Number.isInteger) || b.x < 0 || b.y < 0 || b.width < 1 || b.height < 1 ||
        b.x + b.width > image.width || b.y + b.height > image.height) { setError('矩形须在截图范围内。'); return }
    setError(''); setMarks(old => [...old, { ...b, mask }])
  }
  async function upload() {
    if (!image || view || !canvas.current || busy) return
    setBusy(true); setError('')
    try {
      const el = canvas.current
      paint(el, image, marks)
      const png = await new Promise<Blob>((resolve, reject) => el.toBlob(b => b ? resolve(b) : reject(new Error('无法导出截图。')), 'image/png'))
      await request(api, { method: 'POST', headers: { 'Content-Type': 'image/png' }, body: png })
      reset(); saved()
    } catch (e) { setError(e instanceof Error ? e.message : '截图提交失败。') }
    finally { setBusy(false) }
  }
  return <div><button className="detail-text-button" disabled={busy} aria-expanded={open}
    onClick={() => { reset(); setOpen(!open) }}>{open ? '收起截图' : `截图证据（${artifacts.length}）`}</button>
    {open && <fieldset disabled={busy}><legend>截图证据</legend>
      {artifacts.map(a => <p key={a.id}>{a.expired ? <span>截图 #{a.id} 已到期</span> :
        <button className="detail-text-button" onClick={() => void load(undefined, a)}>查看截图 #{a.id}</button>}
        {' '}{a.bytes} 字节 · SHA-256 <code>{a.sha256}</code></p>)}
      <label>选择本机 PNG 截图<input type="file" accept="image/png" onChange={e => {
        const f = e.target.files?.[0]; e.target.value = ''; if (f) void load(f)
      }}/></label>
      {captured && !image && <button className="detail-text-button" onClick={() => void load(captured)}>重新载入扩展截图</button>}
      {error && <p role="alert">{error}</p>}
      {image && <><p>{view ? `已归档截图 #${view}` : '在图上拖动矩形或输入坐标，确认敏感区域已遮挡后提交。'} · {image.width} × {image.height}</p>
        {!view && <ThemedSelect label="标注方式" value={mask ? 'mask' : 'outline'} onChange={value=>setMask(value==='mask')}
          options={[{value:'mask',label:'遮挡'},{value:'outline',label:'红框标注'}]}/>}
        <canvas ref={canvas} aria-label={view ? '已归档截图' : '截图标注预览'} style={{ display: 'block', maxWidth: '100%', height: 'auto', touchAction: view ? 'auto' : 'none' }}
          onPointerDown={e => { if (view || busy || !e.isPrimary || e.button !== 0) return
            e.currentTarget.setPointerCapture(e.pointerId); start.current = point(e) }}
          onPointerMove={e => { if (e.isPrimary && start.current) setDrag(rect(e)) }}
          onPointerUp={e => { if (e.isPrimary && start.current) { const b = rect(e); start.current = null; setDrag(null); add(b) } }}
          onPointerCancel={() => { start.current = null; setDrag(null) }} />
        {!view && <><details><summary>按像素坐标添加矩形</summary>
          {(['x','y','width','height'] as const).map(k => <label key={k}>{{ x: '左边距', y: '上边距', width: '宽度', height: '高度' }[k]}
            <input type="number" min={k === 'x' || k === 'y' ? 0 : 1} value={box[k]} onChange={e => setBox({ ...box, [k]: Number(e.target.value) })}/></label>)}
          <button className="button" onClick={() => add(box)}>添加矩形</button></details>
          <button className="detail-text-button" disabled={!marks.length} onClick={() => setMarks(m => m.slice(0, -1))}>撤销上一个矩形</button>
          <button className="button" disabled={busy || Boolean(drag)} onClick={() => void upload()}>确认并提交截图</button></>}
      </>}
    </fieldset>}
  </div>
}
