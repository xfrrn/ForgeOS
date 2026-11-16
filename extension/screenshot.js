const statusLine = document.querySelector('#status')
const send = document.querySelector('#send')
const query = new URLSearchParams(location.hash.slice(1))
const nonce = crypto.randomUUID()
const flag = 'data-forgeos-mask-' + nonce

document.querySelector('#discard').onclick = () => window.close()

async function capture() {
  const {tabId, windowId, destination, metadata, target, style} = captureSettings(query, flag)
  async function active() {
    const [tab] = await chrome.tabs.query({active: true, windowId})
    if (tab?.id !== tabId) throw new Error('预览标签页已切换，请重新截图。')
  }
  await active()
  await chrome.scripting.insertCSS(style)
  let image, stable, switched = false
  const onActivated = tab => { if (tab.windowId === windowId && tab.tabId !== tabId) switched = true }
  chrome.tabs.onActivated.addListener(onActivated)
  try {
    const [ready] = await chrome.scripting.executeScript({target, func: prepareMasks,
      args: [flag, metadata.get('path'), query.get('origin')]})
    if (!ready?.result) throw new Error('预览页面已改变，请重新截图。')
    await active()
    image = await chrome.tabs.captureVisibleTab(windowId, {format:'png'})
    await active()
  } finally {
    try {
      const [check] = await chrome.scripting.executeScript({target, func: key => globalThis[key]?.(), args:[flag]})
      stable = check?.result && !switched
    } finally {
      chrome.tabs.onActivated.removeListener(onActivated)
      await chrome.scripting.removeCSS(style)
    }
  }
  if (!stable) throw new Error('截图期间视口或隐私区域发生变化，请重新截图。')
  const png = await (await fetch(image)).blob()
  const bitmap = await createImageBitmap(png)
  const tooLarge = bitmap.width * bitmap.height > 16 * 1024 * 1024 || png.size > 16 * 1024 * 1024
  bitmap.close()
  if (tooLarge) throw new Error('截图超过工作台限制，请缩小视口后重试。')
  const img = document.querySelector('#image')
  img.src = URL.createObjectURL(png)
  img.hidden = send.hidden = false
  statusLine.textContent = '请核对截图。表单控件、内嵌页面和私有区域已隐藏；可在工作台继续遮挡并确认上传。'
  metadata.set('screenshot', nonce)
  metadata.set('extension', location.origin)
  destination.hash = destination.hash.split('?')[0] + '?' + metadata
  offerCapture(send, statusLine, png, destination, 'screenshot', nonce)
}

capture().catch(err => { statusLine.textContent = err.message }).finally(async () => {
  const current = await chrome.tabs.getCurrent()
  if (current) await chrome.tabs.update(current.id, {active: true})
})
