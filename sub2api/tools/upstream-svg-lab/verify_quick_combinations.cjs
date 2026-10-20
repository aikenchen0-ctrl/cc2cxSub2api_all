const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH
  || 'playwright');

async function verifyQuickCombinations() {
  const { createLab } = await import(pathToFileURL(path.join(__dirname, 'server.mjs')).href);
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-quick-combinations-'));
  const lab = await createLab({ dataDir, admin: {}, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'msedge' });
    const page = await browser.newPage({ viewport: { width: 1440, height: 1800 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const prompts = [
      { id: 'ak47', label: 'AK47', text: 'First prompt.', version: 1 },
      { id: 'composite', label: '复合提示词', text: 'Composite prompt.', version: 1 },
      { id: 'custom', label: '自定义动画', text: 'Custom animation prompt.', version: 1 },
    ];
    const site = 'https://fixture.invalid/v1';
    const sources = [{
      source_url: site, source_job_id: 'old-key-task', job_name: '旧凭据任务',
      key_configured: true, key_fingerprint: 'olderfingerprint',
      jobs: [
        { id: 'latest-task', name: '最近来源任务', prompt_id: 'ak47', key_configured: false },
        { id: 'old-key-task', name: '旧凭据任务', prompt_id: 'composite', key_configured: true },
      ],
    }];
    const submissions = [];
    // 模拟全部 API，组合测试不访问真实上游或消耗测试额度。
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      assert.equal(url.origin, origin);
      if (url.pathname === '/api/config') return route.fulfill({ json: {
        management_required: false, authenticated: true, csrf_token: 'fixture-csrf',
        prompt_id: 'ak47', prompts, models: ['model-a', 'model-b'],
        max_parallel_jobs: 6, reasoning_effort: 'low',
      } });
      if (url.pathname === '/api/sources') return route.fulfill({ json: { items: sources } });
      if (url.pathname === '/api/accounts' || url.pathname === '/api/results' || url.pathname === '/api/jobs') {
        return route.fulfill({ json: { items: [], active_count: 0 } });
      }
      if (url.pathname === '/api/settings') return route.fulfill({ json: { max_parallel_jobs: 6 } });
      if (url.pathname === '/api/connection') return route.fulfill({ json: { base_url: '', key_configured: false } });
      if (url.pathname === '/api/jobs/batch') {
        const body = route.request().postDataJSON();
        submissions.push(body);
        return route.fulfill({ json: { items: body.prompt_ids.map(id => ({ id })) } });
      }
      if (url.pathname === '/api/prompts' && route.request().method() === 'POST') {
        const preset = { id: 'new-custom', ...route.request().postDataJSON(), version: 1 };
        prompts.push(preset);
        return route.fulfill({ json: { item: preset } });
      }
      return route.fulfill({ status: 404, json: { error: '测试中出现未定义接口' } });
    });
    await page.goto(origin);
    await page.locator('#quickPromptSelect [data-quick-prompt-id="custom"]').waitFor();
    assert.equal(await page.locator('#quickPromptSelect input').count(), 3, '快捷区应包含全部内置与自定义提示词');
    await page.locator('#modelA').fill('selected-model-a');
    await page.locator('#modelB').fill('selected-model-b');
    await page.locator('#quickPromptSelect [value="composite"]').check();
    await page.locator('#quickPromptSelect [value="custom"]').check();
    await page.locator('#quickPromptSelect [value="ak47"]').uncheck();
    const selected = async selector => page.locator(`${selector} input:checked`).evaluateAll(inputs => inputs.map(input => input.value).sort());
    assert.deepEqual(await selected('#promptSelect'), ['composite', 'custom'], '快捷多选应同步到顶部');
    await page.locator('#quickSourceSelect').selectOption(site);
    assert.equal(await page.locator('#quickSourceJobSelect').inputValue(), '', '站点多凭据时应允许不指定历史任务');
    const defaultText = await page.locator('#quickSourceJobSelect option').first().innerText();
    assert.match(defaultText, /最近来源任务/);
    assert.doesNotMatch(defaultText, /olderfingerprint/, '默认描述不能误用其他任务指纹');
    await page.locator('#createJobButton').click();
    await page.waitForFunction(() => document.querySelector('#notice').textContent.includes('已创建'));
    assert.equal(submissions[0].source_url_ref, site, '未指定任务时应提交站点来源引用');
    assert.equal(submissions[0].source_job_id, undefined);
    assert.deepEqual(submissions[0].prompt_ids.sort(), ['composite', 'custom']);
    assert.deepEqual(submissions[0].models, ['selected-model-a', 'selected-model-b']);

    await page.locator('#quickSourceSelect').selectOption(site);
    await page.locator('#quickSourceJobSelect').selectOption('old-key-task');
    assert.deepEqual(await selected('#quickPromptSelect'), ['composite', 'custom'], '历史任务只能改变凭据');
    await page.locator('#promptSelect [value="ak47"]').check();
    assert.deepEqual(await selected('#quickPromptSelect'), ['ak47', 'composite', 'custom'], '顶部多选应同步到快捷区');
    await page.locator('#createRunJobButton').click();
    await page.waitForFunction(() => document.querySelector('#notice').textContent.includes('已启动'));
    assert.equal(submissions[1].source_job_id, 'old-key-task');
    assert.equal(submissions[1].source_url_ref, undefined);
    assert.deepEqual(submissions[1].prompt_ids.sort(), ['ak47', 'composite', 'custom']);
    assert.deepEqual(submissions[1].models, ['selected-model-a', 'selected-model-b']);
    assert.equal(submissions[1].run_now, true);

    await page.locator('#promptCreateButton').click();
    await page.locator('#promptLabelInput').fill('新增快捷选项');
    await page.locator('#promptTextInput').fill('New prompt.');
    await page.locator('#promptSaveButton').click();
    await page.locator('#quickPromptSelect [value="new-custom"]').waitFor();
    assert.equal(await page.locator('#quickPromptSelect input').count(), 4, '新建提示词应立即同步到快捷区');
    assert.deepEqual(errors, []);
    console.log('快捷组合回归通过：全目录多选、双向同步、站点默认引用、历史凭据独立、模型保持、新建提示词同步。');
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => lab.server.close(resolve));
    assert.ok(path.resolve(dataDir).startsWith(path.resolve(os.tmpdir()) + path.sep));
    await fs.rm(dataDir, { recursive: true, force: true });
  }
}

verifyQuickCombinations().catch(error => {
  console.error(`快捷组合回归失败：${error.message}`);
  process.exitCode = 1;
});
