const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright')
const assert = require('node:assert/strict')
const http = require('node:http')
const fs = require('node:fs/promises')
const os = require('node:os')
const path = require('node:path')

const fixtureKey = 'sk-local-browser-fixture-123456789'
const editedAk47Label = 'AK47 本地编辑版'
const editedAk47Text = 'Write an edited AK47 SVG fixture for browser verification'
const customLabel = '本地自定义提示词'
const customV1 = 'custom prompt fixture version one'
const customV2 = 'custom prompt fixture version two'

async function waitFor(condition, timeout = 10000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await condition()) return
    await new Promise(resolve => setTimeout(resolve, 20))
  }
  throw new Error('等待浏览器条件超时')
}

async function setPromptSelection(page, ids) {
  const wanted = new Set(ids)
  const inputs = page.locator('#promptSelect input[data-prompt-id]')
  const count = await inputs.count()
  for (let index = 0; index < count; index += 1) {
    const input = inputs.nth(index)
    const id = await input.getAttribute('data-prompt-id')
    const checked = await input.isChecked()
    if (wanted.has(id) !== checked) await input.click()
  }
}

async function openEditorFor(page, id) {
  await setPromptSelection(page, [id])
  await page.locator('#promptEditButton').click()
  await page.locator('#promptEditor').waitFor({ state: 'visible' })
  await page.locator('#promptEditorSelect').selectOption(id)
}

(async () => {
  const { createLab } = await import('./server.mjs')
  const { PROMPTS } = await import('./lib/prompts.mjs')
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-prompt-editor-e2e-'))
  const calls = []
  const upstream = http.createServer(async (req, res) => {
    const chunks = []
    for await (const chunk of req) chunks.push(chunk)
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    calls.push({ body, authorization: req.headers.authorization || '' })
    res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({
      status: 'completed',
      model: body.model,
      output_text: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
    }))
  })

  let lab
  let browser
  try {
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve))
    const upstreamURL = `http://127.0.0.1:${upstream.address().port}/v1`
    lab = await createLab({
      dataDir,
      admin: { list: async () => [] },
      startScheduler: false,
      maxParallelJobs: 6
    })
    await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve))
    const baseURL = `http://127.0.0.1:${lab.server.address().port}`
    browser = await chromium.launch({ headless: true, channel: 'msedge' })
    const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
    const pageErrors = []
    page.on('pageerror', error => pageErrors.push(error.message))
    await page.goto(`${baseURL}/`, { waitUntil: 'domcontentloaded' })
    await page.waitForFunction(() => document.querySelectorAll('#promptSelect input[data-prompt-id]').length === 2)

    assert.equal(await page.locator('#promptField').inputValue(), PROMPTS[0].text)
    assert.ok(await page.locator('#quickSourceSelect').count(), '快捷来源控件不存在')
    assert.ok(await page.locator('#billingCsvFile').count(), '计费 CSV 控件不存在')
    assert.ok(await page.locator('#billingAnalyzeButton').count(), '计费分析按钮不存在')

    // 新建提示词并确认它立即进入多选列表。
    await page.locator('#promptCreateButton').click()
    await page.locator('#promptLabelInput').fill(customLabel)
    await page.locator('#promptTextInput').fill(customV1)
    await page.locator('#promptSaveButton').click()
    await page.waitForFunction(label => [...document.querySelectorAll('#promptSelect label')].some(item => item.textContent.includes(label)), customLabel)
    const customID = await page.locator('#promptSelect input[data-prompt-id]').evaluateAll((items, label) => {
      const match = items.find(item => item.parentElement?.textContent?.includes(label))
      return match?.getAttribute('data-prompt-id') || ''
    }, customLabel)
    assert.match(customID, /^custom-/)

    // 创建历史任务，保存当时的原文快照。
    await setPromptSelection(page, [customID])
    await page.locator('#pasteField').fill(`${upstreamURL} api_key=${fixtureKey}`)
    await page.locator('#createJobButton').click()
    await waitFor(async () => (await page.locator('.job-row').count()) === 1)
    const historicalJob = lab.store.state.jobs.find(job => job.prompt_id === customID)
    assert.ok(historicalJob)
    assert.equal(historicalJob.prompt, customV1)

    // 修改自定义提示词，历史任务原文必须保持不变。
    await openEditorFor(page, customID)
    await page.locator('#promptTextInput').fill(customV2)
    await page.locator('#promptSaveButton').click()
    await page.locator('#promptEditor').waitFor({ state: 'hidden' })
    assert.equal(lab.store.state.jobs.find(job => job.id === historicalJob.id).prompt, customV1)

    // 运行历史任务，确认发给上游的仍是旧快照。
    await page.locator(`[data-run-job="${historicalJob.id}"]`).click()
    await waitFor(() => !lab.scheduler.active && calls.length === 2)
    assert.ok(calls.every(call => call.body.input === customV1))
    assert.ok(calls.every(call => call.body.reasoning?.effort === 'low'))

    // 编辑内置 AK47 的名称和原文。
    await openEditorFor(page, 'ak47')
    await page.locator('#promptLabelInput').fill(editedAk47Label)
    await page.locator('#promptTextInput').fill(editedAk47Text)
    await page.locator('#promptSaveButton').click()
    await page.locator('#promptEditor').waitFor({ state: 'hidden' })
    assert.ok(await page.locator('#promptSelect label').filter({ hasText: editedAk47Label }).count())
    assert.equal(await page.locator('#promptField').inputValue(), editedAk47Text)

    // 取消编辑不得写入。
    await openEditorFor(page, 'ak47')
    await page.locator('#promptLabelInput').fill('取消不应保存')
    await page.locator('#promptTextInput').fill('cancelled text')
    await page.locator('#promptCancelButtonSecondary').click()
    await openEditorFor(page, 'ak47')
    assert.equal(await page.locator('#promptLabelInput').inputValue(), editedAk47Label)
    assert.equal(await page.locator('#promptTextInput').inputValue(), editedAk47Text)
    await page.locator('#promptCancelButtonSecondary').click()

    // 两个提示词同时选择并立即运行，验证四次模型请求均使用 max 和完整原文。
    await setPromptSelection(page, ['ak47', customID])
    await page.locator('#effortSelect').selectOption('max')
    await page.locator('#pasteField').fill(`${upstreamURL} api_key=${fixtureKey}`)
    await page.locator('#createRunJobButton').click()
    await waitFor(() => !lab.scheduler.active && calls.length === 6, 15000)
    const batchCalls = calls.slice(2)
    assert.equal(batchCalls.length, 4)
    assert.ok(batchCalls.some(call => call.body.input === editedAk47Text))
    assert.ok(batchCalls.some(call => call.body.input === customV2))
    assert.ok(batchCalls.every(call => call.body.reasoning?.effort === 'max'))
    assert.ok(batchCalls.every(call => call.authorization === `Bearer ${fixtureKey}`))
    assert.equal(lab.store.state.results.filter(result => result.status === 'success').length, 6)
    assert.deepEqual(pageErrors, [])

    console.log(JSON.stringify({
      页面: '本地浏览器端到端验收通过',
      新建提示词: '已进入多选',
      内置AK47编辑: '名称和原文已保存',
      取消编辑: '未写入',
      历史任务快照: '保持 v1',
      双提示词并行: '4 个请求',
      思考程度: '全部 max',
      快捷来源和计费面板: '存在',
      上游请求: calls.length
    }))
  } finally {
    if (browser) await browser.close()
    if (lab) {
      lab.scheduler.stop()
      await new Promise(resolve => lab.server.close(resolve))
    }
    await new Promise(resolve => upstream.close(resolve))
    await fs.rm(dataDir, { recursive: true, force: true })
  }
})().catch(error => {
  console.error(error)
  process.exitCode = 1
})
