const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

const playwrightPath = process.env.PLAYWRIGHT_MODULE_PATH
  || 'playwright';
const { chromium } = require(playwrightPath);

async function verifyResponseDiagnostics() {
  const { createLab } = await import(pathToFileURL(path.join(__dirname, 'server.mjs')).href);
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-response-diagnostics-'));
  const lab = await createLab({ dataDir, admin: {}, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'msedge' });
    const page = await browser.newPage({ viewport: { width: 1440, height: 2000 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const ids = {
      success: '11111111-1111-4111-8111-111111111111',
      text: '22222222-2222-4222-8222-222222222222',
      legacy: '33333333-3333-4333-8333-333333333333',
      empty: '44444444-4444-4444-8444-444444444444',
      live: '55555555-5555-4555-8555-555555555555',
    };
    const rawText = '模型说明文本\n<script>window.responseExecuted = true</script>\n<svg onload="window.responseExecuted=true"><path d="M 1 1"></svg>';
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 80"><rect width="100" height="80" fill="#26907a"/></svg>';
    const now = new Date().toISOString();
    const base = {
      model: 'mock-svg-model', prompt_id: 'ak47', prompt_label: 'AK47',
      reasoning_effort: 'max', source_url: 'https://mock.invalid',
      started_at: now, created_at: now, duration_ms: 600490, http_status: 200,
    };
    const results = [
      { ...base, id: ids.success, job_name: '成功结果', status: 'success', has_svg: true },
      { ...base, id: ids.text, job_name: '文字结果', status: 'failed', has_svg: false, failure_kind: 'upstream_no_svg', upstream_event_type: 'response.completed' },
      { ...base, id: ids.legacy, job_name: '旧失败记录', status: 'failed', has_svg: false, failure_kind: 'local_svg_validation' },
      { ...base, id: ids.empty, job_name: '空输出记录', status: 'failed', has_svg: false, failure_kind: 'upstream_empty_output' },
      { ...base, id: ids.live, job_name: '运行中记录', status: 'running', has_svg: false, live: true },
    ];
    let artifactRequests = 0;
    let responseRequests = 0;
    let failResponse = false;
    let delayedID = '';
    let releaseDelayed;

    // 仅使用本地模拟接口，不向上游发起模型请求。
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      assert.equal(url.origin, origin, '测试只能访问本地服务');
      const authenticated = route.request().headers()['x-lab-access'] === 'mock-management-token';
      if (url.pathname === '/api/config') return route.fulfill({ json: {
        management_required: true, authenticated, csrf_token: authenticated ? 'mock-csrf' : '',
        reasoning_effort: 'low', models: ['mock-svg-model'],
        prompts: [{ id: 'ak47', label: 'AK47', text: 'mock prompt' }],
      } });
      if (url.pathname === '/api/results') return route.fulfill({ json: { items: results } });
      if (['/api/accounts', '/api/jobs', '/api/sources'].includes(url.pathname)) return route.fulfill({ json: { items: [] } });
      if (url.pathname === '/api/settings') return route.fulfill({ json: { max_parallel_jobs: 6 } });
      if (url.pathname === '/api/connection') return route.fulfill({ json: { base_url: '', key_configured: false } });
      const artifact = url.pathname.match(/^\/api\/results\/([^/]+)\/artifact$/);
      if (artifact) {
        artifactRequests++;
        return route.fulfill({ contentType: 'image/svg+xml; charset=utf-8', body: svg });
      }
      const response = url.pathname.match(/^\/api\/results\/([^/]+)\/response$/);
      if (response) {
        responseRequests++;
        if (!authenticated) return route.fulfill({ status: 401, json: { error: '请先登录管理区' } });
        if (failResponse) return route.fulfill({ status: 500, json: { error: '模拟原文读取失败' } });
        const id = response[1];
        if (id === delayedID) await new Promise(resolve => { releaseDelayed = resolve; });
        const result = results.find(item => item.id === id);
        const text = id === ids.success ? svg : id === ids.text ? rawText : '';
        const hint = id === ids.text ? '模型返回的是说明文字，没有完整 SVG 根。'
          : id === ids.empty ? '上游已结束，但未返回模型文本。' : '';
        return route.fulfill({ json: {
          id, response_text: text, has_response_text: Boolean(text),
          diagnostics: {
            status: result.status, failure_kind: result.failure_kind,
            upstream_event_type: result.upstream_event_type || 'response.failed',
            upstream_message: id === ids.empty ? '上游未输出' : '',
            http_status: 200, response_bytes: Buffer.byteLength(text),
            timeout_ms: 1200000, idle_timeout_ms: 280000, analysis_hint: hint,
          },
        } });
      }
      return route.fulfill({ status: 404, json: { error: '测试中出现未定义接口' } });
    });
    await page.goto(origin);
    await page.locator('.result-card').first().waitFor();
    assert.equal(await page.locator('[data-response-id]').count(), 0, '匿名访问不能显示原始响应入口');
    assert.equal(responseRequests, 0, '匿名访问不能请求原文');
    const frame = page.locator(`.result-thumb[src*="${ids.success}"]`);
    await frame.scrollIntoViewIfNeeded();
    await frame.contentFrame().locator('svg').waitFor();
    await page.evaluate(id => {
      const frame = document.querySelector(`.result-thumb[src*="${id}"]`);
      window.stableResponseFrame = frame;
      window.stableResponseWindow = frame.contentWindow;
    }, ids.success);

    await page.locator('#authToggle').click();
    await page.locator('#accessToken').fill('mock-management-token');
    await page.locator('#authForm button').click();
    await page.locator(`[data-response-id="${ids.success}"]`).waitFor();
    assert.equal(await page.locator('[data-response-id]').count(), 4, '成功与失败结果都应显示原文入口');
    assert.equal(await page.locator(`[data-response-id="${ids.live}"]`).count(), 0, '运行中不能显示原文入口');
    assert.equal(await page.locator('.result-open [data-response-id]').count(), 0, '按钮不能嵌套在打开 SVG 按钮内');
    assert.equal(await page.locator(`[data-result-key="${ids.text}"]`).innerText().then(text => text.includes('response.completed')), true, '卡片应显示上游结束事件');

    await page.locator(`[data-response-id="${ids.text}"]`).click();
    await page.locator('#responseDialog[open]').waitFor();
    await page.waitForFunction(() => document.querySelector('#responseText').textContent.includes('<script>'));
    assert.equal(await page.locator('#responseText').textContent(), rawText, '模型响应应完整原样显示');
    assert.equal(await page.locator('#responseText script, #responseText svg').count(), 0, '原文不能解析为 DOM 内容');
    assert.equal(await page.evaluate(() => Boolean(window.responseExecuted)), false, '模型原文不能执行');
    assert.equal(await page.locator('#responseHint').textContent(), '模型返回的是说明文字，没有完整 SVG 根。', '应优先展示后台分析说明');
    assert.ok((await page.locator('#responseDiagnostics').textContent()).includes('1200000'), '诊断应保留总等待限制');
    const downloadPromise = page.waitForEvent('download');
    await page.locator('#downloadResponse').click();
    const download = await downloadPromise;
    const downloadPath = path.join(dataDir, 'downloaded-response.txt');
    await download.saveAs(downloadPath);
    assert.equal(await fs.readFile(downloadPath, 'utf8'), rawText, '下载内容应与模型原文相同');
    await page.locator('#closeResponse').click();
    await page.waitForFunction(() => document.querySelector('#responseText').textContent === '');
    assert.equal(await page.locator('#responseText').textContent(), '', '关闭弹窗后应清除原文');

    await page.locator(`[data-response-id="${ids.legacy}"]`).click();
    await page.locator('#responseEmpty').waitFor({ state: 'visible' });
    assert.match(await page.locator('#responseEmpty').textContent(), /历史记录未保存模型原文/, '旧记录未保存原文时应明确说明');
    assert.equal(await page.locator('#downloadResponse').isDisabled(), true, '空正文不能下载');
    await page.locator('#closeResponse').click();
    await page.locator(`[data-response-id="${ids.empty}"]`).click();
    await page.locator('#responseEmpty').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#responseEmpty').textContent(), '上游已结束，但未返回模型文本。', '新空输出不能误称为旧记录遗失');
    await page.locator('#closeResponse').click();

    failResponse = true;
    await page.locator(`[data-response-id="${ids.success}"]`).click();
    await page.locator('#responseError').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#responseError').textContent(), '模拟原文读取失败', '接口错误应清楚显示');
    await page.locator('#closeResponse').click();
    failResponse = false;

    delayedID = ids.legacy;
    await page.locator(`[data-response-id="${ids.legacy}"]`).click();
    await page.waitForFunction(() => !document.querySelector('#responseLoading').hidden);
    const deadline = Date.now() + 5000;
    while (!releaseDelayed && Date.now() < deadline) await page.waitForTimeout(20);
    assert.ok(releaseDelayed, '延迟响应请求应已发出');
    await page.locator('#closeResponse').click();
    await page.locator(`[data-response-id="${ids.success}"]`).click();
    await page.waitForFunction(() => document.querySelector('#responseText').textContent.startsWith('<svg'));
    releaseDelayed();
    await page.waitForTimeout(150);
    assert.equal(await page.locator('#responseText').textContent(), svg, '较早的慢响应不能覆盖当前结果');
    await page.locator('#closeResponse').click();

    await page.locator('#refreshButton').click();
    await page.locator(`[data-response-id="${ids.success}"]`).waitFor();
    assert.equal(await page.evaluate(id => {
      const frame = document.querySelector(`.result-thumb[src*="${id}"]`);
      return frame === window.stableResponseFrame && frame.contentWindow === window.stableResponseWindow;
    }, ids.success), true, '登录及原文查看不能重建 SVG 预览');
    assert.equal(artifactRequests, 1, '查看原文和刷新不应重复请求已有 SVG');
    await page.locator('#authToggle').click();
    await page.waitForFunction(() => document.querySelectorAll('[data-response-id]:not([hidden])').length === 0);
    assert.deepEqual(errors, [], '前端不能出现未处理异常');
    console.log('原始响应回归通过：匿名隐藏、成功失败入口、运行中无入口、事件展示、纯文本原样显示、诊断与空输出区别、下载、错误提示、慢响应隔离、SVG 节点稳定。');
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => lab.server.close(resolve));
    assert.ok(path.resolve(dataDir).startsWith(path.resolve(os.tmpdir()) + path.sep), '清理目录必须位于系统临时目录');
    await fs.rm(dataDir, { recursive: true, force: true });
  }
}

verifyResponseDiagnostics().catch(error => {
  console.error(`原始响应回归失败：${error.message}`);
  process.exitCode = 1;
});
