// Browser smoke/visual check against preview-home.mjs only. Requires Node 22+
// and an installed Chrome. Screenshots go to a fresh temporary directory.
import { spawn } from 'node:child_process'
import { mkdtemp, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import assert from 'node:assert/strict'

const base = 'http://127.0.0.1:18419'
const fixture = await (await fetch(base + '/api/v1/agent/context')).json()
assert.equal(fixture.data?.agent?.agent_id, 'visual-only', 'Use only the isolated preview fixture')
const reset = await fetch(base + '/api/v1/agent/admin/branding', {
  method: 'PUT', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ compact_home_enabled: true, home_content: '' }),
})
assert.ok(reset.ok, 'Could not reset the in-memory homepage fixture')
const output = await mkdtemp(join(tmpdir(), 'agentapi-home-browser-'))
const chrome = spawn(process.env.AGENTAPI_QA_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe', [
  '--headless', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0',
  `--user-data-dir=${join(output, 'profile')}`, 'about:blank',
], { windowsHide: true, stdio: ['ignore', 'ignore', 'pipe'] })
let socket
try {
  const endpoint = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Chrome startup timed out')), 15000)
    let logs = ''
    chrome.on('error', reject)
    chrome.stderr.on('data', chunk => {
      logs += chunk.toString()
      const match = logs.match(/DevTools listening on (ws:\/\/[^\s]+)/)
      if (match) { clearTimeout(timer); resolve(match[1]) }
    })
  })
  socket = new WebSocket(endpoint)
  await new Promise((resolve, reject) => {
    socket.addEventListener('open', resolve, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  const pending = new Map()
  let sequence = 0
  socket.addEventListener('message', event => {
    const result = JSON.parse(event.data)
    const waiter = pending.get(result.id)
    if (!waiter) return
    pending.delete(result.id)
    clearTimeout(waiter.timer)
    if (result.error) waiter.reject(new Error(JSON.stringify(result.error)))
    else waiter.resolve(result.result)
  })
  function send(method, params = {}, sessionId) {
    return new Promise((resolve, reject) => {
      const id = ++sequence
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(`${method} timed out`)) }, 15000)
      pending.set(id, { resolve, reject, timer })
      socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }))
    })
  }
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' })
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true })
  const command = (method, params) => send(method, params, sessionId)
  async function evaluate(expression) {
    const response = await command('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
    if (response.exceptionDetails) throw new Error(JSON.stringify(response.exceptionDetails))
    return response.result.value
  }
  async function waitFor(expression) {
    for (let i = 0; i < 120; i++) {
      if (await evaluate(expression)) return
      await new Promise(resolve => setTimeout(resolve, 100))
    }
    const page = await evaluate('({url:location.href, body:document.body?.innerText, html:document.documentElement.outerHTML.slice(0,2000)})')
    throw new Error(`UI condition not reached: ${expression}; page=${JSON.stringify(page)}`)
  }
  await command('Page.enable')
  async function capture(name, path, width, height, theme, selector) {
    await command('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: width < 600 })
    await command('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: theme }] })
    const navigation = await command('Page.navigate', { url: base + path })
    if (navigation.errorText) throw new Error(navigation.errorText)
    await waitFor(`Boolean(document.querySelector(${JSON.stringify(selector)}))`)
    await evaluate('document.fonts.ready')
    const metrics = await evaluate('({width:innerWidth, contentWidth:document.documentElement.scrollWidth, title:document.title})')
    assert.equal(metrics.width, width, 'Device viewport was not applied')
    assert.ok(metrics.contentWidth <= width, `Horizontal overflow on ${name}: ${metrics.contentWidth} > ${width}`)
    const { data } = await command('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(join(output, name + '.png'), Buffer.from(data, 'base64'))
    console.log(JSON.stringify({ name, ...metrics }))
  }
  await capture('compact-desktop-light', '/home', 1440, 1000, 'light', '[data-testid="compact-home"]')
  await capture('compact-mobile-light', '/home', 390, 844, 'light', '[data-testid="compact-home"]')
  await capture('compact-mobile-dark', '/home', 390, 844, 'dark', '[data-testid="compact-home"]')
  await capture('settings-desktop-dark', '/admin/settings', 1440, 1600, 'dark', '#home-content')
  async function snapshot(name) {
    await settleMotion()
    const { data } = await command('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(join(output, name + '.png'), Buffer.from(data, 'base64'))
  }
  async function settleMotion() {
    await evaluate(`new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))`)
    await waitFor(`document.getAnimations().every(animation => animation.effect?.getComputedTiming().iterations === Infinity || animation.playState !== 'running')`)
  }
  await evaluate(`document.querySelector('[aria-label="收起侧栏"]').click()`)
  await waitFor(`document.querySelector('#agent-sidebar').getBoundingClientRect().width === 72`)
  await snapshot('sidebar-desktop-collapsed')
  await command('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true })
  await evaluate(`document.querySelector('[aria-label="打开菜单"]').click()`)
  await waitFor(`document.querySelector('#agent-sidebar').getBoundingClientRect().width === 256`)
  assert.equal(await evaluate(`sessionStorage.getItem('agentapi_sidebar_collapsed')`), 'true')
  await snapshot('sidebar-mobile-from-collapsed')
  await evaluate(`document.querySelector('[aria-label="关闭菜单遮罩"]').click()`)
  await waitFor(`!document.querySelector('[aria-label="关闭菜单遮罩"]')`)
  await settleMotion()
  assert.ok(await evaluate(`document.querySelector('#agent-sidebar').getBoundingClientRect().right <= 0`), 'Mobile sidebar did not finish closing')
  console.log('Desktop collapse and mobile drawer checks passed')
  await capture('settings-mobile-dark', '/admin/settings', 390, 1800, 'dark', '#home-content')
  // Exercise the real Vue form against the in-memory fixture, then reload home.
  await evaluate(`(() => {
    const input = document.querySelector('#home-content');
    input.value = '<!doctype html><html><head><style>body{margin:0;background:#eef2ff;font:24px sans-serif;padding:40px;color:#312e81}</style></head><body><h1>本站自定义首页</h1><p>样式隔离验证</p><script>parent.document.title="UNSAFE"<\/script></body></html>';
    input.dispatchEvent(new Event('input', {bubbles:true}));
  })()`)
  await evaluate(`Array.from(document.querySelectorAll('button')).find(button => button.textContent.includes('保存品牌信息')).click()`)
  await waitFor(`document.body.textContent.includes('本站品牌信息已保存')`)
  await capture('custom-html-mobile', '/home', 390, 844, 'light', '[data-testid="custom-home"] iframe')
  assert.notEqual(await evaluate('document.title'), 'UNSAFE')
  const frame = await evaluate(`({sandbox:document.querySelector('iframe').getAttribute('sandbox'), html:document.querySelector('iframe').getAttribute('srcdoc')})`)
  assert.ok(!frame.sandbox.includes('allow-scripts') && !frame.sandbox.includes('allow-same-origin'))
  assert.ok(frame.html.includes('本站自定义首页'))
  console.log('Screenshots: ' + output)
  await send('Browser.close')
} finally {
  socket?.close()
  if (chrome.exitCode === null) chrome.kill()
}
