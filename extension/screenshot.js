const statusLine = document.querySelector('#status')
const send = document.querySelector('#send')
const query = new URLSearchParams(location.hash.slice(1))
const nonce = crypto.randomUUID()
const flag = 'data-forgeos-mask-' + nonce
let opened, png, destination

document.querySelector('#discard').onclick = () => window.close()

// Runs in the isolated world of the same document that provided the candidate headers.
async function prepareMasks(flag, path, origin) {
  if (location.origin !== origin || location.pathname !== path || document.designMode === 'on') return false
  const html = document.documentElement
  const marked = []
  for (const el of document.querySelectorAll('*')) {
    if (el instanceof HTMLElement && chrome.dom.openOrClosedShadowRoot(el)) {
      el.setAttribute(flag, '')
      marked.push(el)
    }
  }
  html.setAttribute(flag, '')
  let changed = false
  const observer = new MutationObserver(() => { changed = true })
  observer.observe(html, { subtree: true, attributes: true, attributeFilter: [flag, 'data-forgeos-private', 'contenteditable'] })
  const start = [scrollX, scrollY, innerWidth, innerHeight].join(',')
  const clear = () => {
    observer.disconnect()
    html.removeAttribute(flag)
    for (const el of marked) el.removeAttribute(flag)
    delete globalThis[flag]
  }
  const timer = setTimeout(clear, 6000)
  globalThis[flag] = () => {
    const uncoveredShadow = Array.from(document.querySelectorAll('*')).some(el =>
      el instanceof HTMLElement && !el.hasAttribute(flag) && chrome.dom.openOrClosedShadowRoot(el))
    const valid = !changed && !uncoveredShadow && document.designMode !== 'on' &&
      start === [scrollX, scrollY, innerWidth, innerHeight].join(',') && location.pathname === path
    clearTimeout(timer)
    clear()
    return valid
  }
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
  return true
}

async function capture() {
  const tabId = Number(query.get('tab')), windowId = Number(query.get('window'))
  destination = new URL(query.get('destination'))
  const paired = localStorage.getItem('workbench')
  if (destination.origin !== paired) throw new Error('工作台连接已改变，请重新采集。')
  const [route, params] = destination.hash.split('?')
  if (!/^#\/tasks\/[1-9]\d*\/previews\/[0-9a-f]{48}\/feedback$/.test(route)) throw new Error('候选反馈地址无效。')
  const metadata = new URLSearchParams(params)
  const target = { tabId, documentIds: [query.get('document')] }
  const privateElements = 'input, textarea, select, iframe, frame, object, embed, [contenteditable], [data-forgeos-private], [' + flag + ']'
  const style = { target, origin: 'USER', css: `
    html[${flag}]:is([contenteditable], [data-forgeos-private]),
    html[${flag}] :is(${privateElements}), html[${flag}] :is(${privateElements}) * {
      transition:none !important; animation:none !important; opacity:0 !important; visibility:hidden !important;
    }` }
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
  png = await (await fetch(image)).blob()
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
  destination.hash = route + '?' + metadata
}

send.onclick = () => {
  if (opened && !opened.closed) { opened.focus(); return }
  opened = window.open(destination.href, '_blank')
  if (!opened) { statusLine.textContent = '未能打开工作台，请允许本次打开标签页后重试。'; return }
  statusLine.textContent = '请在工作台登录并登记反馈。截图保存在此标签页内，关闭后丢弃。'
}
window.addEventListener('message', event => {
  if (!png || !opened || event.source !== opened || event.origin !== destination.origin ||
      event.data?.type !== 'forgeos-screenshot-ready' || event.data.nonce !== nonce ||
      event.data.route !== destination.hash || event.ports.length !== 1) return
  const port = event.ports[0]
  port.postMessage(png)
  port.close()
  statusLine.textContent = '截图已传给工作台，可在反馈中遮挡和标注。'
  document.querySelector('#discard').textContent = '关闭此页'
})
capture().catch(err => { statusLine.textContent = err.message }).finally(async () => {
  const current = await chrome.tabs.getCurrent()
  if (current) await chrome.tabs.update(current.id, {active: true})
})
