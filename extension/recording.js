const statusLine = document.querySelector('#status')
const stopButton = document.querySelector('#stop')
const send = document.querySelector('#send')
const query = new URLSearchParams(location.hash.slice(1))
document.querySelector('#discard').onclick = () => window.close()

async function record() {
  const nonce = crypto.randomUUID(), flag = 'data-forgeos-mask-' + nonce
  const {tabId, destination, metadata, target, style} = captureSettings(query, flag)
  const current = await chrome.tabs.getCurrent()
  const [source] = await chrome.tabs.query({active: true, windowId: current.windowId})
  if (source?.id !== tabId) throw new Error('候选标签页已切换，请重新开始录制。')
  const streamId = await chrome.tabCapture.getMediaStreamId({targetTabId: tabId, consumerTabId: current.id})
  await chrome.scripting.insertCSS(style)
  let stream, port, recorder, failure, nearLimit = false
  const ready = Promise.withResolvers(), verified = Promise.withResolvers()
  async function confirm(reply) {
    const timer = setTimeout(() => reply.resolve(false), 8000)
    try { return await reply.promise } finally { clearTimeout(timer) }
  }
  function stop(reason) {
    if (reason) failure = reason
    if (recorder?.state === 'recording') recorder.stop()
  }
  function stopFromPopup(msg, sender, respond) {
    if (sender.id === chrome.runtime.id && msg?.type === 'stop-recording' && msg.tab === tabId) {
      stop(); respond(true)
    }
  }
  function release() {
    for (const track of stream?.getTracks() ?? []) track.stop()
    port?.disconnect()
  }
  window.addEventListener('pagehide', release, {once: true})
  try {
    const [prepared] = await chrome.scripting.executeScript({target, func: prepareMasks,
      args: [flag, metadata.get('path'), query.get('origin'), true]})
    if (!prepared?.result) throw new Error('无法核对当前候选文档。')
    port = chrome.tabs.connect(tabId, {documentId: query.get('document'), name: flag})
    port.onMessage.addListener(msg => {
      if (msg === 'ready') ready.resolve(true)
      if (msg === 'valid') verified.resolve(true)
      if (msg === 'invalid') {
        stop('原文档或隐私区域发生变化，本次录屏已丢弃。')
        ready.resolve(false); verified.resolve(false)
      }
    })
    port.onDisconnect.addListener(() => {
      stop('候选文档已离开，本次录屏已丢弃。')
      ready.resolve(false); verified.resolve(false)
    })
    if (!await confirm(ready)) throw new Error(failure || '原候选文档未回应。')
    stream = await navigator.mediaDevices.getUserMedia({audio:false,
      video:{mandatory:{chromeMediaSource:'tab', chromeMediaSourceId:streamId}}})
    if (failure) throw new Error(failure)
    recorder = new MediaRecorder(stream, {mimeType:'video/webm;codecs=vp8', videoBitsPerSecond:2000000})
    const chunks = []
    let total = 0
    const stopped = new Promise(resolve => { recorder.onstop = resolve })
    recorder.ondataavailable = event => {
      total += event.data.size
      if (total > 64 * 1024 * 1024) { stop('录屏超过 64 MiB，请分段录制。'); return }
      if (event.data.size) chunks.push(event.data)
      if (total >= 60 * 1024 * 1024) { nearLimit = true; stop() }
    }
    recorder.onerror = () => stop('浏览器录制失败，请重试。')
    stream.getVideoTracks()[0].onended = () => stop('浏览器提前结束采集，本次录屏已丢弃。')
    stopButton.onclick = () => stop()
    chrome.runtime.onMessage.addListener(stopFromPopup)
    recorder.start(500)
    stopButton.hidden = false
    statusLine.textContent = '正在录制候选标签页。操作完成后，从扩展工具栏或此页停止录制。'
    await stopped
    if (failure) throw new Error(failure)
    port.postMessage('verify')
    if (!await confirm(verified) || !chunks.length) throw new Error('录屏未通过候选文档核对。')
    const clip = new Blob(chunks, {type:'video/webm'})
    const player = document.querySelector('#player')
    player.src = URL.createObjectURL(clip)
    player.hidden = send.hidden = false
    statusLine.textContent = (nearLimit ? '本段已达到录制大小上限。' : '') + '请回放核对录屏，再到工作台登记反馈。'
    metadata.set('recording', nonce)
    metadata.set('extension', location.origin)
    destination.hash = destination.hash.split('?')[0] + '?' + metadata
    offerCapture(send, statusLine, clip, destination, 'recording', nonce)
  } finally {
    stopButton.hidden = true
    chrome.runtime.onMessage.removeListener(stopFromPopup)
    window.removeEventListener('pagehide', release)
    release()
    try { await chrome.scripting.removeCSS(style) } catch { /* The target document may have closed. */ }
  }
}
record().catch(err => { statusLine.textContent = err.message }).finally(async () => {
  const current = await chrome.tabs.getCurrent()
  if (current) await chrome.tabs.update(current.id, {active:true})
})
