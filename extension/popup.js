const statusLine = document.querySelector('#status')
const pair = document.querySelector('#pair')
const pick = document.querySelector('#pick')
const page = document.querySelector('#page')

async function prepare() {
  const [tab] = await chrome.tabs.query({active: true, currentWindow: true})
  if (!tab?.url) throw new Error('请从工作台或候选预览标签页打开扩展。')
  const url = new URL(tab.url)
  const local = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  const workbench = (url.protocol === 'http:' && local) || (url.protocol === 'https:' && url.hostname.startsWith('forgeos.'))
  if (workbench && ['/', '/ui/'].includes(url.pathname)) {
    statusLine.textContent = url.origin
    pair.hidden = false
    pair.onclick = () => {
      localStorage.setItem('workbench', url.origin)
      statusLine.textContent = '已连接 ' + url.origin
      pair.hidden = true
    }
    return
  }
  const paired = localStorage.getItem('workbench')
  if (!paired) throw new Error('请先在 ForgeOS 工作台打开扩展，点击“连接此工作台”。')
  const owner = new URL(paired)
  const preview = /^p-([0-9a-f]{48})\.(.+)$/.exec(url.hostname)
  const localPreview = preview?.[2] === 'localhost' && url.protocol === 'http:' && owner.protocol === 'http:'
  const gatewayPreview = preview && url.protocol === 'https:' && owner.protocol === 'https:' && owner.hostname === 'forgeos.' + preview[2] && owner.port === url.port
  if (!preview || (!localPreview && !gatewayPreview)) throw new Error('当前页面不属于已连接工作台的候选预览。')
  statusLine.textContent = '在工作台确认页面路径、填写意见并选择测试用例。'
  pick.hidden = page.hidden = false
  async function capture(elementMode) {
    try {
      const [result] = await chrome.scripting.executeScript({target: {tabId: tab.id}, func: startCapture, args: [paired, url.origin, preview[1], elementMode]})
      if (!result?.result?.ok) throw new Error(result?.result?.error || '无法核对当前候选。')
      if (result.result.destination) await chrome.tabs.create({url: result.result.destination})
      window.close()
    } catch (err) { statusLine.textContent = err.message }
  }
  pick.onclick = () => capture(true)
  page.onclick = () => capture(false)
}

async function startCapture(workbench, origin, preview, elementMode) {
  if (location.origin !== origin) return {error: '标签页已切换，请重新选择。'}
  const response = await fetch('/', {method: 'HEAD', cache: 'no-store', redirect: 'error'})
  const commit = response.headers.get('X-ForgeOS-Commit') || ''
  const task = response.headers.get('X-ForgeOS-Task') || ''
  const execution = response.headers.get('X-ForgeOS-Execution') || ''
  if (!response.ok || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(commit) || !/^[1-9]\d*$/.test(task) || !/^[1-9]\d*$/.test(execution)) {
    return {error: '当前预览已停止或缺少候选信息。'}
  }
  function destination(element) {
    const edge = /Edg\/(\d+)/.exec(navigator.userAgent)
    const chrome = /Chrome\/(\d+)/.exec(navigator.userAgent)
    const browser = edge ? 'Edge/' + edge[1] : chrome ? 'Chrome/' + chrome[1] : ''
    const capture = new URLSearchParams({commit, execution, path: location.pathname, browser,
      width: String(innerWidth), height: String(innerHeight), element})
    return workbench + '/#/tasks/' + task + '/previews/' + preview + '/feedback?' + capture
  }
  if (!elementMode) return {ok: true, destination: destination('')}
  // Only the isolated extension world retains picker state between invocations.
  globalThis.forgeosCancelPick?.()
  const tip = document.createElement('div')
  tip.setAttribute('role', 'status')
  tip.textContent = '点击要批注的元素，按 Esc 取消'
  tip.style.cssText = 'position:fixed;inset:12px auto auto 12px;z-index:2147483647;padding:8px 12px;background:#17232e;color:white;font:14px system-ui;pointer-events:none'
  document.documentElement.append(tip)
  const cleanup = () => {
    tip.remove()
    window.removeEventListener('click', select, true)
    window.removeEventListener('keydown', cancel, true)
    clearTimeout(timer)
    delete globalThis.forgeosCancelPick
  }
  function cancel(e) { if (e.key === 'Escape') cleanup() }
  function select(e) {
    if (!e.isTrusted || !(e.target instanceof Element)) return
    e.preventDefault(); e.stopImmediatePropagation()
    const steps = []
    for (let el = e.target; el; el = el.parentElement) {
      let index = 1
      for (let prev = el.previousElementSibling; prev; prev = prev.previousElementSibling) if (prev.localName === el.localName) index++
      steps.unshift(el.localName + ':nth-of-type(' + index + ')')
    }
    cleanup()
    window.open(destination(steps.join(' > ')), '_blank', 'noopener,noreferrer')
  }
  window.addEventListener('click', select, true)
  window.addEventListener('keydown', cancel, true)
  const timer = setTimeout(cleanup, 60000)
  globalThis.forgeosCancelPick = cleanup
  return {ok: true}
}

prepare().catch(err => { statusLine.textContent = err.message })
