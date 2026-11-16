// Runs in the isolated world of the same document that provided the candidate headers.
async function prepareMasks(flag, path, origin, recording) {
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
  let port
  const invalid = () => { changed = true; port?.postMessage('invalid') }
  function uncovered(el) {
    return el instanceof HTMLElement && !el.hasAttribute(flag) && chrome.dom.openOrClosedShadowRoot(el)
  }
  const observer = new MutationObserver(records => {
    for (const entry of records) {
      if (entry.type === 'attributes') { invalid(); return }
      for (const el of [...entry.addedNodes, ...entry.removedNodes]) {
        if (el instanceof Element && (uncovered(el) || Array.from(el.querySelectorAll('*')).some(uncovered))) {
          invalid(); return
        }
      }
    }
  })
  observer.observe(html, { subtree: true, childList: Boolean(recording), attributes: true,
    attributeFilter: [flag, 'data-forgeos-private', 'contenteditable'] })
  const start = [scrollX, scrollY, innerWidth, innerHeight].join(',')
  const clear = () => {
    observer.disconnect()
    window.removeEventListener('pagehide', invalid)
    if (recording) chrome.runtime.onConnect.removeListener(connect)
    html.removeAttribute(flag)
    for (const el of marked) el.removeAttribute(flag)
    delete globalThis[flag]
  }
  const timer = setTimeout(clear, recording ? 15000 : 6000)
  function connect(channel) {
    if (channel.name !== flag || port) return
    port = channel
    clearTimeout(timer)
    port.onDisconnect.addListener(clear)
    port.onMessage.addListener(msg => {
      if (msg === 'verify') port.postMessage(valid() ? 'valid' : 'invalid')
    })
    port.postMessage(valid() ? 'ready' : 'invalid')
  }
  if (recording) {
    window.addEventListener('pagehide', invalid, {once:true})
    chrome.runtime.onConnect.addListener(connect)
  }
  function valid() {
    const uncoveredShadow = Array.from(document.querySelectorAll('*')).some(uncovered)
    return !changed && !uncoveredShadow && document.designMode !== 'on' &&
      (recording || start === [scrollX, scrollY, innerWidth, innerHeight].join(',')) && location.pathname === path
  }
  globalThis[flag] = () => {
    const ok = valid()
    clearTimeout(timer)
    clear()
    return ok
  }
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
  return true
}


function captureSettings(query,flag) {
  const tabId = Number(query.get('tab')), windowId = Number(query.get('window'))
  const destination = new URL(query.get('destination'))
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
  return {tabId,windowId,destination,metadata,target,style}
}

function offerCapture(send, statusLine, blob, destination, kind, nonce) {
  let opened
  send.onclick = () => {
    if (opened && !opened.closed) { opened.focus(); return }
    opened = window.open(destination.href, '_blank')
    if (!opened) { statusLine.textContent = '未能打开工作台，请允许本次打开标签页后重试。'; return }
    statusLine.textContent = '请在工作台登录并登记反馈。确认接收前请保留此标签页。'
  }
  window.addEventListener('message', event => {
    if (!opened || event.source !== opened || event.origin !== destination.origin ||
        event.data?.type !== 'forgeos-' + kind + '-ready' || event.data.nonce !== nonce ||
        event.data.route !== destination.hash || event.ports.length !== 1) return
    const port = event.ports[0]
    port.postMessage(blob)
    port.close()
    statusLine.textContent = '已传给工作台，可在反馈中核对后提交。'
    document.querySelector('#discard').textContent = '关闭此页'
  })
}
