const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

const playwrightPath = process.env.PLAYWRIGHT_MODULE_PATH
  || 'playwright';
const { chromium } = require(playwrightPath);

async function verifyGalleryStability() {
  const { createLab } = await import(pathToFileURL(path.join(__dirname, 'server.mjs')).href);
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-gallery-stability-'));
  const lab = await createLab({ dataDir, admin: {}, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'msedge' });
    const page = await browser.newPage({ viewport: { width: 1440, height: 2000 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const successID = '11111111-1111-4111-8111-111111111111';
    const runningID = '22222222-2222-4222-8222-222222222222';
    const failedID = '33333333-3333-4333-8333-333333333333';
    const addedID = '44444444-4444-4444-8444-444444444444';
    const now = new Date().toISOString();
    const baseResult = {
      model: 'mock-svg-model', prompt_id: 'ak47', prompt_label: 'AK47',
      reasoning_effort: 'low', source_url: 'https://mock.invalid',
      started_at: now, created_at: now, duration_ms: 1250, http_status: 200,
    };
    const completed = (id, jobName) => ({
      ...baseResult, id, job_name: jobName, status: 'success', has_svg: true,
    });
    let results = [
      completed(successID, 'stable-history'),
      { ...baseResult, id: runningID, job_name: 'running-to-complete', status: 'running', live: true, has_svg: false },
      { ...baseResult, id: failedID, job_name: 'failed-without-svg', status: 'failed', has_svg: false, failure_kind: 'upstream_error', error: '模拟上游失败' },
    ];
    let resultsRequests = 0;
    const artifactRequests = new Map();
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 180"><rect width="400" height="180" fill="#142035"/><circle cx="40" cy="90" r="24" fill="#74dfbc"><animate attributeName="cx" values="40;360;40" dur="8s" repeatCount="indefinite"/></circle></svg>';

    // 所有 API 都由本地模拟返回，禁止触发真实上游请求或计费。
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      assert.equal(url.origin, origin, '测试请求只能访问本地服务');
      if (url.pathname === '/api/config') {
        return route.fulfill({
          json: {
            management_required: true, authenticated: false, reasoning_effort: 'low',
            models: ['mock-svg-model'], prompts: [{ id: 'ak47', label: 'AK47', text: 'mock prompt' }],
          },
        });
      }
      if (url.pathname === '/api/results') {
        resultsRequests += 1;
        return route.fulfill({ json: { items: results } });
      }
      const artifact = url.pathname.match(/^\/api\/results\/([^/]+)\/artifact$/);
      if (artifact) {
        const id = artifact[1];
        artifactRequests.set(id, (artifactRequests.get(id) || 0) + 1);
        return id === failedID
          ? route.fulfill({ status: 404, json: { error: '此结果没有完整 SVG' } })
          : route.fulfill({ contentType: 'image/svg+xml; charset=utf-8', body: svg });
      }
      return route.fulfill({ status: 404, json: { error: '测试中出现未定义接口' } });
    });
    await page.goto(origin);
    const successFrame = page.locator(`.result-thumb[src*="${successID}"]`);
    await successFrame.waitFor();
    await successFrame.scrollIntoViewIfNeeded();
    await successFrame.contentFrame().locator('svg').waitFor();
    await page.evaluate(id => {
      const frame = document.querySelector(`.result-thumb[src*="${id}"]`);
      window.__galleryStableFrame = frame;
      window.__galleryStableWindow = frame.contentWindow;
    }, successID);
    const elapsed = page.locator('.result-card.is-live .result-meta').filter({ hasText: '已用' }).first();
    const elapsedBefore = (await elapsed.innerText()).match(/已用\s+([\d.]+)\s*秒/)?.[1];
    assert.ok(elapsedBefore, '运行中卡片应显示已用秒数');

    async function assertStable(reason) {
      assert.equal(await page.evaluate(id => {
        const frame = document.querySelector(`.result-thumb[src*="${id}"]`);
        return frame === window.__galleryStableFrame
          && frame?.contentWindow === window.__galleryStableWindow
          && frame?.isConnected;
      }, successID), true, reason);
      assert.equal(artifactRequests.get(successID), 1, `${reason}：已有缩略图不应重新请求`);
    }

    await page.waitForFunction(previous => {
      const card = document.querySelector('.result-card.is-live');
      const current = card?.textContent.match(/已用\s+([\d.]+)\s*秒/)?.[1];
      return current && Number(current) > Number(previous);
    }, elapsedBefore, { timeout: 5000 });
    await assertStable('每秒更新耗时后必须保留 iframe 和浏览上下文');
    assert.equal(await page.locator(`.result-thumb[src*="${failedID}"]`).count(), 0, '无 SVG 的失败结果不能创建 iframe');
    assert.equal(artifactRequests.has(failedID), false, '无 SVG 的失败结果不能请求不存在的文件');

    // 等待真实的两轮十秒刷新，检查轮询不会重载 SVG 或重启动画。
    const pollingStart = resultsRequests;
    await page.waitForFunction(() => document.querySelector('.result-card.is-live') !== null);
    const pollingDeadline = Date.now() + 26000;
    while (resultsRequests < pollingStart + 2 && Date.now() < pollingDeadline) {
      await page.waitForTimeout(250);
    }
    assert.ok(resultsRequests >= pollingStart + 2, '应完成至少两轮十秒自动刷新');
    const observedPolls = resultsRequests - pollingStart;
    await assertStable('两轮自动刷新后必须保留历史 SVG');

    results = [completed(addedID, 'new-complete'), ...results];
    await page.locator('#refreshButton').click();
    await page.locator(`.result-thumb[src*="${addedID}"]`).waitFor();
    await assertStable('插入新结果后必须保留历史 SVG');

    results = results.map(result => result.id === runningID ? completed(runningID, 'running-to-complete') : result);
    await page.locator('#refreshButton').click();
    await page.locator(`.result-thumb[src*="${runningID}"]`).waitFor();
    await assertStable('运行中结果完成后必须保留历史 SVG');
    assert.equal(await page.locator('.result-card.is-live').count(), 0, '已完成结果不能继续显示运行中');

    // 同一结果的业务元数据更新、以及结果排序变化，都不能重建已有 SVG iframe。
    results = results
      .map(result => result.id === successID
        ? { ...result, job_name: 'stable-history-renamed', http_status: 206 }
        : result)
      .reverse();
    await page.locator('#refreshButton').click();
    await page.locator('.result-job-name').filter({ hasText: 'stable-history-renamed' }).waitFor();
    await assertStable('同 ID 元数据变化后必须保留 SVG iframe');
    assert.equal(await page.locator(`.result-job-name`).filter({ hasText: 'stable-history-renamed' }).count(), 1, '更新后的任务名应显示');
    assert.equal(await page.locator(`.result-open[data-result-id="${successID}"]`).innerText().then(text => text.includes('HTTP 206')), true, '更新后的 HTTP 状态应显示');
    const renderedOrder = await page.locator('.result-card').evaluateAll(cards => cards
      .sort((left, right) => Number(getComputedStyle(left).order) - Number(getComputedStyle(right).order))
      .map(card => card.dataset.resultKey));
    assert.deepEqual(renderedOrder, results.map(result => result.id), '后台结果顺序变化后应按新顺序显示');

    await page.locator('#resultSearch').fill('no-matching-task');
    assert.equal(await successFrame.isVisible(), false, '不匹配结果应隐藏');
    assert.equal(await page.locator('.result-card:visible').count(), 0, '搜索无匹配时不应显示结果卡片');
    assert.equal(await page.locator('#resultsList .empty-state:visible').count(), 1, '搜索无匹配时只能显示一个空提示');
    await assertStable('筛选隐藏时必须保留 iframe 节点');
    await page.locator('#resultSearch').fill('stable-history');
    await successFrame.waitFor({ state: 'visible' });
    await assertStable('恢复搜索结果后必须保留 SVG');
    await page.locator('#resultSearch').fill('');
    await assertStable('清空搜索后必须保留 SVG');

    await page.locator(`.result-open[data-result-id="${successID}"]`).click();
    await page.locator('#artifactDialog[open]').waitFor();
    await page.locator('#artifactFrame').contentFrame().locator('svg').waitFor();
    assert.equal(await page.locator('#artifactError').isVisible(), false, 'SVG 详情应正常打开');
    assert.equal(await page.locator('#downloadArtifact').isEnabled(), true, '完整 SVG 应允许下载');
    await page.locator('#closeArtifact').click();
    assert.deepEqual(errors, [], '前端不能产生未处理异常');
    console.log(`画廊稳定性回归通过：耗时递增、节点及浏览上下文稳定、${observedPolls} 轮自动刷新无重载、元数据及顺序更新稳定、新结果插入、运行转成功、搜索恢复、失败结果占位、详情打开。`);
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => lab.server.close(resolve));
    assert.ok(path.resolve(dataDir).startsWith(path.resolve(os.tmpdir()) + path.sep), '清理目录必须位于系统临时目录');
    await fs.rm(dataDir, { recursive: true, force: true });
  }
}

verifyGalleryStability().catch(error => {
  console.error(`画廊稳定性回归失败：${error.message}`);
  process.exitCode = 1;
});
