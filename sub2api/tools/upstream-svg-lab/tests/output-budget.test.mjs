import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { createLab } from '../server.mjs';
import { probe, normalizeMaxOutputTokens } from '../lib/probe.mjs';
import { Store } from '../lib/store.mjs';
import { Scheduler } from '../lib/scheduler.mjs';

const credentials = { url: 'https://budget.invalid/v1', key: 'sk-budget-test-123456', model: 'budget-model' };

test('预算入口统一校验，不把无效配置静默替换成另一预算', async () => {
  assert.equal(normalizeMaxOutputTokens(), 65536);
  assert.equal(normalizeMaxOutputTokens('98304'), 98304);
  for (const value of [null, NaN, 0, 1023, 1.5, 'invalid', 1000001]) {
    assert.throws(() => new Scheduler({ store: {}, admin: {}, maxOutputTokens: value }), /输出预算/);
    await assert.rejects(probe({
      ...credentials, maxOutputTokens: value,
      fetchImpl: () => assert.fail('无效预算不能发出请求')
    }), /输出预算/);
  }
});

test('旧任务重启后应用新的运行预算，配置、请求、实时项和结果保持一致', async t => {
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-output-budget-'));
  let lab;
  t.after(async () => {
    if (lab) await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(dataDir, { force: true, recursive: true });
  });
  const oldStore = await new Store(dataDir).init();
  const oldScheduler = new Scheduler({ store: oldStore, admin: {}, maxOutputTokens: 16384 });
  const job = await oldScheduler.create({
    pasted_text: `${credentials.url} api_key=${credentials.key}`, models: [credentials.model],
    interval_minutes: 0, reasoning_effort: 'medium'
  });
  let resolveResponse;
  const responseGate = new Promise(resolve => { resolveResponse = resolve; });
  let requestBody;
  lab = await createLab({ dataDir, admin: {}, maxOutputTokens: '65536', startScheduler: false });
  lab.scheduler.probeImpl = options => probe({
    ...options,
    fetchImpl: async (_url, init) => {
      requestBody = JSON.parse(init.body);
      await responseGate;
      return new Response(JSON.stringify({ status: 'completed', output_text: '<svg><rect/></svg>' }));
    }
  });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${lab.server.address().port}`;
  const config = await fetch(`${base}/api/config`).then(response => response.json());
  assert.equal(config.max_output_tokens, 65536);
  const run = lab.scheduler.run(job.id, true);
  try {
    const deadline = Date.now() + 3000;
    while (!requestBody && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 5));
    assert.equal(requestBody.max_output_tokens, 65536);
    assert.equal(requestBody.reasoning.effort, 'medium');
    assert.equal(lab.scheduler.getLiveResults()[0].max_output_tokens, 65536);
  } finally {
    resolveResponse();
    await run;
  }
  assert.equal(lab.store.state.results[0].max_output_tokens, 65536);
  assert.equal(lab.store.state.results[0].status, 'success');
});

test('预算耗尽仍记为未完成，保留已输出文本和用量且不自动重试', async () => {
  const usage = { output_tokens: 65536, output_tokens_details: { reasoning_tokens: 5000 } };
  let calls = 0;
  const result = await probe({
    ...credentials,
    fetchImpl: async () => {
      calls++;
      return new Response(`data: ${JSON.stringify({
        type: 'response.incomplete', response: {
          status: 'incomplete', output_text: '<svg><rect/></svg>', usage,
          incomplete_details: { reason: 'max_output_tokens' }
        }
      })}\n\n`, { headers: { 'content-type': 'text/event-stream' } });
    }
  });
  assert.equal(calls, 1);
  assert.equal(result.max_output_tokens, 65536);
  assert.equal(result.failure_kind, 'upstream_incomplete');
  assert.equal(result.upstream_code, 'max_output_tokens');
  assert.equal(result.status, 'failed');
  assert.equal(result.svg, '');
  assert.equal(result.response_text, '<svg><rect/></svg>');
  assert.deepEqual(result.usage, usage);
});
