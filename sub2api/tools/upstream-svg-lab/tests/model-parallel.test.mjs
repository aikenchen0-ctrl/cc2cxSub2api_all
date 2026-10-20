import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { Store } from '../lib/store.mjs';
import { Scheduler } from '../lib/scheduler.mjs';

const pasted_text = 'https://parallel.test/v1 api_key=sk-parallel_fixture_123456';

function gate() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

async function waitFor(condition) {
  const deadline = Date.now() + 5000;
  while (!condition()) {
    if (Date.now() > deadline) assert.fail('等待模型并行测试条件超时');
    await new Promise(resolve => setTimeout(resolve, 5));
  }
}

function completed(input) {
  return { source_url: input.url, model: input.model, status: 'success',
    response_text: '<svg/>', svg: '<svg/>' };
}

async function fixture(t, probeImpl, options = {}) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-model-parallel-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const store = await new Store(directory).init();
  const calls = [];
  const sourceCalls = [];
  const admin = {
    account: async id => ({
      id, name: `account-${id}`, platform: 'openai', type: 'apikey', status: 'active',
      credentials: { base_url: `https://account-${id}.test/v1` },
      credentials_status: { has_api_key: true }
    }),
    source: async id => {
      sourceCalls.push(id);
      return { url: `https://account-${id}.test/v1`, key: 'sk-account_fixture_123456', name: `account-${id}` };
    }
  };
  const scheduler = new Scheduler({
    store, admin, maxParallelJobs: 1, ...options,
    probeImpl: async input => {
      calls.push(input);
      return probeImpl(input);
    }
  });
  return { scheduler, store, calls, sourceCalls };
}

test('同一任务的 A/B 模型在任一完成前均已发出，并各自出现在实时画廊', async t => {
  const barriers = { 'model-a': gate(), 'model-b': gate() };
  const { scheduler, store, calls } = await fixture(t, async input => {
    await barriers[input.model].promise;
    return completed(input);
  });
  const job = await scheduler.create({ pasted_text, models: ['model-a', 'model-b'], interval_minutes: 0 });
  const running = scheduler.run(job.id, true);
  try {
    await waitFor(() => calls.length === 2);
    assert.equal(store.state.results.length, 0);
    assert.equal(scheduler.activeCount, 1);
    assert.deepEqual(scheduler.getLiveResults().map(result => result.model).sort(), ['model-a', 'model-b']);
    assert.equal(scheduler.getLiveResults().every(result => result.job_id === job.id && result.status === 'running'), true);
    barriers['model-a'].resolve();
    await waitFor(() => store.state.results.length === 1 && scheduler.getLiveResults().length === 1);
    assert.equal(store.state.results[0].model, 'model-a');
    assert.equal(scheduler.activeCount, 1);
    assert.deepEqual(scheduler.getLiveResults().map(result => result.model), ['model-b']);
  } finally {
    for (const barrier of Object.values(barriers)) barrier.resolve();
    await running;
  }
  assert.equal(store.state.results.length, 2);
  assert.equal(scheduler.activeCount, 0);
  assert.deepEqual(scheduler.getLiveResults(), []);
});

test('同任务一个模型抛错仍保留失败记录，另一个模型可独立完成', async t => {
  const barrier = gate();
  const { scheduler, store, calls } = await fixture(t, async input => {
    if (input.model === 'broken') throw new Error('模拟单模型失败');
    await barrier.promise;
    return completed(input);
  });
  const job = await scheduler.create({ pasted_text, models: ['broken', 'working'], interval_minutes: 0 });
  const running = scheduler.run(job.id, true);
  try {
    await waitFor(() => calls.length === 2 && store.state.results.length === 1 && scheduler.getLiveResults().length === 1);
    assert.equal(store.state.results[0].model, 'broken');
    assert.equal(store.state.results[0].status, 'failed');
    assert.equal(store.state.results[0].failure_kind, 'local_probe_error');
    assert.equal(scheduler.activeCount, 1);
    assert.deepEqual(scheduler.getLiveResults().map(result => result.model), ['working']);
  } finally {
    barrier.resolve();
    await running;
  }
  assert.equal(store.state.results.find(result => result.model === 'working').status, 'success');
  assert.equal(scheduler.activeCount, 0);
});

test('一个模型结果保存失败时，须等其余模型收尾才释放任务名额', async t => {
  const barrier = gate();
  const saveFailed = gate();
  const { scheduler, store, calls, sourceCalls } = await fixture(t, async input => {
    if (input.model === 'slow') await barrier.promise;
    return completed(input);
  });
  const job = await scheduler.create({ account_ids: [1, 2], models: ['save-fails', 'slow'], interval_minutes: 0 });
  const queued = await scheduler.create({ pasted_text, models: ['queued'], interval_minutes: 0 });
  const originalSave = store.saveResult.bind(store);
  store.saveResult = async result => {
    if (result.model === 'save-fails') {
      saveFailed.resolve();
      throw new Error('模拟结果保存失败');
    }
    return originalSave(result);
  };
  const running = scheduler.run(job.id, true);
  const outcome = running.then(() => null, error => error);
  try {
    await saveFailed.promise;
    await waitFor(() => calls.length === 2);
    // 给拒绝传播充分的事件循环机会，验证未使用会提前返回的 Promise.all。
    await new Promise(resolve => setImmediate(resolve));
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(scheduler.activeCount, 1);
    assert.equal(store.state.jobs.find(item => item.id === job.id).running, true);
    assert.deepEqual(sourceCalls, [1]);
    assert.equal(scheduler.queueRun(queued.id).status, 'queued');
    await scheduler.tick();
    assert.equal(calls.length, 2);
    assert.equal(scheduler.queueCount, 1);
    assert.deepEqual(scheduler.getLiveResults().map(result => result.model), ['slow']);
  } finally {
    scheduler.stop();
    barrier.resolve();
    await outcome;
  }
  assert.match((await outcome)?.message || '', /模拟结果保存失败/);
  assert.equal(store.state.results.find(result => result.model === 'slow').status, 'success');
  assert.equal(scheduler.activeCount, 0);
  assert.deepEqual(scheduler.getLiveResults(), []);
  assert.deepEqual(sourceCalls, [1]);
});

test('停止后本账号已发送的所有模型完成，后续账号和排队任务均不启动', async t => {
  const barrier = gate();
  const { scheduler, store, calls, sourceCalls } = await fixture(t, async input => {
    await barrier.promise;
    return completed(input);
  });
  const job = await scheduler.create({ account_ids: [1, 2], models: ['model-a', 'model-b'], interval_minutes: 0 });
  const queued = await scheduler.create({ pasted_text, models: ['queued'], interval_minutes: 0 });
  const running = scheduler.run(job.id, true);
  try {
    await waitFor(() => calls.length === 2);
    assert.equal(scheduler.queueRun(queued.id).status, 'queued');
    scheduler.stop();
    assert.equal(scheduler.activeCount, 1);
    assert.equal(scheduler.queueCount, 0);
  } finally {
    scheduler.stop();
    barrier.resolve();
    await running;
  }
  await scheduler.tick();
  await assert.rejects(scheduler.run(queued.id, true), /已停止/);
  assert.equal(calls.length, 2);
  assert.deepEqual(sourceCalls, [1]);
  assert.equal(store.state.results.length, 2);
  assert.equal(scheduler.activeCount, 0);
  assert.deepEqual(scheduler.getLiveResults(), []);
});

test('单任务八模型同时运行，下一账号等待本账号全部结束后才启动', async t => {
  const barriers = { 'https://account-1.test/v1': gate(), 'https://account-2.test/v1': gate() };
  const { scheduler, store, calls, sourceCalls } = await fixture(t, async input => {
    await barriers[input.url].promise;
    return completed(input);
  });
  const models = Array.from({ length: 8 }, (_, index) => `model-${index}`);
  await assert.rejects(scheduler.create({ pasted_text, models: [...models, 'model-8'] }), /1 至 8/);
  const job = await scheduler.create({ account_ids: [1, 2], models, interval_minutes: 0 });
  const running = scheduler.run(job.id, true);
  try {
    await waitFor(() => calls.length === 8);
    assert.equal(scheduler.getLiveResults().length, 8);
    assert.deepEqual(sourceCalls, [1]);
    assert.equal(calls.every(input => input.url === 'https://account-1.test/v1'), true);
    barriers['https://account-1.test/v1'].resolve();
    await waitFor(() => calls.length === 16);
    assert.equal(store.state.results.length, 8);
    assert.equal(scheduler.getLiveResults().length, 8);
    assert.deepEqual(sourceCalls, [1, 2]);
    assert.equal(calls.slice(8).every(input => input.url === 'https://account-2.test/v1'), true);
  } finally {
    for (const barrier of Object.values(barriers)) barrier.resolve();
    await running;
  }
  assert.equal(store.state.results.length, 16);
  assert.equal(scheduler.activeCount, 0);
});
