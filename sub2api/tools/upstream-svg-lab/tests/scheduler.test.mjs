import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { Store } from '../lib/store.mjs';
import { Scheduler } from '../lib/scheduler.mjs';
import { getPrompt } from '../lib/prompts.mjs';

async function fixture(t, admin = {}, options = {}) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-schedule-'));
  t.after(() => fs.rm(directory, { force: true, recursive: true }));
  const store = await new Store(directory).init();
  const calls = [];
  let clock = Date.now();
  const scheduler = new Scheduler({ store, admin, now: () => clock, ...options, probeImpl: async input => {
    calls.push(input);
    if (options.probeImpl) return options.probeImpl(input);
    return { source_url: input.url, request_url: `${input.url}/responses`, model: input.model,
      reasoning_effort: 'low', status: 'success', svg: '<svg xmlns="http://www.w3.org/2000/svg"/>', response_text: '<svg/>' };
  } });
  return { scheduler, store, calls, directory, advance: minutes => { clock += minutes * 60000; } };
}

const pasted_text = 'https://relay.test/v1 api_key=sk-fixture_123456';

function gate() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

async function waitFor(condition) {
  const deadline = Date.now() + 5000;
  while (!condition()) {
    if (Date.now() > deadline) assert.fail('等待测试条件超时');
    await new Promise(resolve => setTimeout(resolve, 5));
  }
}

function completed(input) {
  return { source_url: input.url, request_url: `${input.url}/responses`, model: input.model,
    status: 'success', response_text: '<svg/>', svg: '<svg/>' };
}

test('定时运行两个模型，重启保留计划，密钥不出现在状态明文或公开任务', async t => {
  const { scheduler, store, calls, directory, advance } = await fixture(t);
  const job = await scheduler.create({ pasted_text, interval_minutes: 5 });
  assert.equal(store.publicJob(store.state.jobs[0]).secret, undefined);
  assert.doesNotMatch(await fs.readFile(path.join(directory, 'state.json'), 'utf8'), /sk-fixture_123456/);
  await scheduler.tick();
  assert.equal(calls.length, 0);
  advance(5);
  await scheduler.tick();
  assert.equal(calls.length, 2);
  assert.deepEqual(calls.map(call => call.model), ['gpt-6.1-sol', 'gpt-6-astra']);
  await scheduler.tick();
  assert.equal(calls.length, 2);
  const restored = await new Store(directory).init();
  assert.equal(restored.state.jobs[0].id, job.id);
  assert.equal(restored.state.results.length, 2);
  assert.equal(restored.open(restored.state.jobs[0].secret), 'sk-fixture_123456');
});

test('暂停计划不自动执行，手动执行可运行且不会重叠', async t => {
  const { scheduler, calls, advance } = await fixture(t);
  const job = await scheduler.create({ pasted_text, interval_minutes: 5, enabled: false });
  advance(10);
  await scheduler.tick();
  assert.equal(calls.length, 0);
  const barrier = gate();
  let started = 0;
  scheduler.probeImpl = async () => {
    started++;
    await barrier.promise;
    return { status: 'failed', response_text: '', svg: '' };
  };
  const first = scheduler.run(job.id, true);
  try {
    await waitFor(() => started === job.models.length);
    await assert.rejects(scheduler.run(job.id, true), /正在运行/);
    scheduler.stop();
  } finally {
    barrier.resolve();
    await first;
  }
  assert.equal(scheduler.active, false);
});

test('模型请求运行期间暴露实时结果并计算已用时间，完成后移除', async t => {
  const barrier = gate();
  const { scheduler, calls } = await fixture(t, {}, {
    probeImpl: async input => {
      await barrier.promise;
      return completed(input);
    }
  });
  const job = await scheduler.create({ pasted_text, models: ['model-live'] });
  const running = scheduler.run(job.id, true);
  await waitFor(() => calls.length === 1);
  const live = scheduler.getLiveResults();
  assert.equal(live.length, 1);
  assert.equal(live[0].status, 'running');
  assert.equal(live[0].model, 'model-live');
  assert.equal(live[0].live, true);
  assert.ok(Number.isFinite(live[0].duration_ms));
  barrier.resolve();
  await running;
  assert.deepEqual(scheduler.getLiveResults(), []);
});

test('账号模式每轮读取当前来源，结果记录真正使用的新地址，权限失败不发请求', async t => {
  let allowed = true;
  const admin = {
    account: async id => ({ id, name: '目标上游', platform: 'openai', type: 'apikey', status: 'active',
      credentials: { base_url: 'https://old.test/v1' }, credentials_status: { has_api_key: true } }),
    source: async id => {
      assert.equal(id, 17);
      if (!allowed) throw new Error('管理员密钥没有导出权限');
      return { url: 'https://new.test/v1', key: 'sk-current-key_12345', name: '目标上游' };
    }
  };
  const { scheduler, store, calls } = await fixture(t, admin);
  const job = await scheduler.create({ account_ids: [17] });
  await scheduler.run(job.id, true);
  assert.equal(calls.length, 2);
  assert.equal(store.state.results[0].source_url, 'https://new.test/v1');
  assert.equal(store.state.results[0].request_url, 'https://new.test/v1/responses');
  allowed = false;
  await scheduler.run(job.id, true);
  assert.equal(calls.length, 2);
  assert.equal(store.state.results[0].status, 'failed');
  assert.match(store.state.results[0].error, /未发送请求/);
});

test('重复任务名自动追加序号保持唯一', async t => {
  const { scheduler, store } = await fixture(t);
  const first = await scheduler.create({ pasted_text, name: '同一上游', interval_minutes: 0 });
  const second = await scheduler.create({ pasted_text, name: '同一上游', interval_minutes: 0 });
  assert.equal(first.name, '同一上游');
  assert.equal(second.name, '同一上游 (2)');
  assert.equal(new Set(store.state.jobs.map(job => job.name)).size, 2);
});

test('任务间隔可单独调整为自动或仅手动', async t => {
  const { scheduler } = await fixture(t);
  const job = await scheduler.create({ pasted_text, interval_minutes: 30 });
  const changed = await scheduler.update(job.id, { interval_minutes: 60 });
  assert.equal(changed.interval_minutes, 60);
  assert.equal(changed.enabled, true);
  const manual = await scheduler.update(job.id, { interval_minutes: 0 });
  assert.equal(manual.interval_minutes, 0);
  assert.equal(manual.enabled, false);
  await assert.rejects(scheduler.update(job.id, { interval_minutes: 3 }), /5 至 10080/);
});

test('两种提示词分别存入计划并原样传给测试，结果保留本轮快照', async t => {
  const { scheduler, store, calls } = await fixture(t, {}, {
    probeImpl: async input => ({ ...completed(input), prompt_id: '错误标识', prompt: '错误文本' })
  });
  const basic = await scheduler.create({ pasted_text, models: ['model-a'] });
  const composite = await scheduler.create({ pasted_text, prompt_id: 'composite', models: ['model-b'] });
  assert.equal(basic.name, `${getPrompt('ak47').label} 上游对比`);
  assert.equal(composite.name, `${getPrompt('composite').label} 上游对比`);
  assert.equal(composite.prompt, getPrompt('composite').text);
  await scheduler.run(basic.id, true);
  await scheduler.run(composite.id, true);
  for (const [index, promptId] of ['ak47', 'composite'].entries()) {
    assert.equal(calls[index].prompt_id, promptId);
    assert.equal(calls[index].prompt, getPrompt(promptId).text);
    const result = store.state.results.find(item => item.prompt_id === promptId);
    assert.equal(result.prompt, getPrompt(promptId).text);
    assert.equal(result.prompt_label, getPrompt(promptId).label);
    assert.equal(result.reasoning_effort, 'low');
  }
  await assert.rejects(scheduler.create({ pasted_text, prompt_id: 'unknown' }));
  await assert.rejects(scheduler.update(composite.id, { prompt_id: 'ak47' }), /新建任务/);
});

test('旧计划默认归为 AK47 并保留原有提示词，来源失败也记录正确提示词', async t => {
  const admin = {
    account: async id => ({ id, name: '权限测试', platform: 'openai', type: 'apikey', status: 'active',
      credentials: { base_url: 'https://old.test/v1' }, credentials_status: { has_api_key: true } }),
    source: async () => { throw new Error('无法导出上游'); }
  };
  const { scheduler, store, calls } = await fixture(t, admin);
  const legacy = await scheduler.create({ pasted_text, models: ['model-a'] });
  await store.change(state => {
    delete state.jobs[0].prompt_id;
    delete state.jobs[0].prompt_label;
    state.jobs[0].prompt = 'legacy prompt retained';
  });
  await scheduler.run(legacy.id, true);
  assert.equal(calls[0].prompt_id, 'ak47');
  assert.equal(calls[0].prompt, 'legacy prompt retained');
  assert.equal(store.state.results[0].prompt, 'legacy prompt retained');
  const restricted = await scheduler.create({ account_ids: [17], prompt_id: 'composite', models: ['model-b'] });
  await scheduler.run(restricted.id, true);
  assert.equal(calls.length, 1);
  assert.equal(store.state.results[0].status, 'failed');
  assert.equal(store.state.results[0].prompt_id, 'composite');
  assert.equal(store.state.results[0].prompt, getPrompt('composite').text);
});

test('默认三个任务实际同时运行，第四个与同任务重入均明确拒绝', async t => {
  const barrier = gate();
  const { scheduler, store, calls } = await fixture(t, {}, {
    maxParallelJobs: 3,
    probeImpl: async input => { await barrier.promise; return completed(input); }
  });
  const jobs = [];
  for (let index = 0; index < 4; index++) jobs.push(await scheduler.create({ pasted_text, models: [`model-${index}`] }));
  const running = jobs.slice(0, 3).map(job => scheduler.run(job.id, true));
  assert.equal(scheduler.activeCount, 3);
  assert.equal(scheduler.active, true);
  assert.equal(scheduler.runningIdsSet.size, 3);
  assert.match(scheduler.runError(jobs[0].id), /该任务正在运行/);
  await assert.rejects(scheduler.run(jobs[0].id, true), /该任务正在运行/);
  await assert.rejects(scheduler.run(jobs[3].id, true), /3 个上限/);
  await assert.rejects(scheduler.update(jobs[0].id, { prompt_id: 'composite' }), /正在运行，不能修改提示词/);
  await waitFor(() => calls.length === 3);
  assert.equal(store.state.jobs.filter(job => job.running).length, 3);
  barrier.resolve();
  await Promise.all(running);
  assert.equal(scheduler.activeCount, 0);
  assert.equal(scheduler.active, false);
  assert.equal(store.state.results.length, 3);
  assert.equal(scheduler.runError(jobs[3].id), '');
  await scheduler.run(jobs[3].id, true);
});

test('定时扫描填满空闲名额，后续扫描补位且同一任务不会重叠', async t => {
  const gates = new Map();
  const { scheduler, calls, advance } = await fixture(t, {}, {
    maxParallelJobs: 3,
    probeImpl: async input => {
      const pending = gate();
      gates.set(input.name, pending);
      await pending.promise;
      return completed(input);
    }
  });
  for (let index = 0; index < 5; index++) {
    await scheduler.create({ pasted_text, name: `到期计划 ${index}`, models: ['model-a'], interval_minutes: 5 });
  }
  advance(5);
  const firstTick = scheduler.tick();
  await waitFor(() => calls.length === 3);
  await scheduler.tick();
  assert.equal(calls.length, 3);
  gates.get(calls[0].name).resolve();
  await waitFor(() => scheduler.activeCount === 2);
  const secondTick = scheduler.tick();
  await waitFor(() => calls.length === 4);
  assert.equal(scheduler.activeCount, 3);
  assert.equal(new Set(calls.map(input => input.name)).size, 4);
  for (const pending of gates.values()) pending.resolve();
  await Promise.all([firstTick, secondTick]);
  const thirdTick = scheduler.tick();
  await waitFor(() => calls.length === 5);
  gates.get(calls[4].name).resolve();
  await thirdTick;
  assert.equal(scheduler.activeCount, 0);
  assert.equal(new Set(calls.map(input => input.name)).size, 5);
});

test('模型调用异常写入失败记录并释放名额，不影响其他并行任务', async t => {
  const { scheduler, store } = await fixture(t, {}, {
    probeImpl: async input => {
      if (input.model === 'broken') throw new Error('模拟模型失败');
      return completed(input);
    }
  });
  const failed = await scheduler.create({ pasted_text, prompt_id: 'composite', models: ['broken'] });
  const succeeded = await scheduler.create({ pasted_text, models: ['working'] });
  await Promise.all([scheduler.run(failed.id, true), scheduler.run(succeeded.id, true)]);
  assert.equal(scheduler.activeCount, 0);
  assert.equal(store.state.results.find(item => item.job_id === failed.id).status, 'failed');
  assert.equal(store.state.results.find(item => item.job_id === failed.id).prompt_id, 'composite');
  assert.equal(store.state.results.find(item => item.job_id === succeeded.id).status, 'success');
  assert.equal(store.state.jobs.every(job => !job.running), true);
});

test('存储回滚后按编号更新计划，清理保存失败也释放并行名额', async t => {
  const barrier = gate();
  const { scheduler, store, calls } = await fixture(t, {}, {
    probeImpl: async input => { await barrier.promise; return completed(input); }
  });
  const job = await scheduler.create({ pasted_text, models: ['model-a'] });
  const running = scheduler.run(job.id, true);
  await waitFor(() => calls.length === 1);
  await assert.rejects(store.change(state => {
    state.jobs[0].name = '不应保存';
    throw new Error('模拟回滚');
  }), /模拟回滚/);
  barrier.resolve();
  await running;
  assert.equal(store.state.jobs[0].name, job.name);
  assert.equal(store.state.jobs[0].running, false);
  assert.equal(store.state.jobs[0].round_count, 1);
  const change = store.change.bind(store);
  store.change = async () => { throw new Error('模拟存储不可写'); };
  await assert.rejects(scheduler.run(job.id, true), /模拟存储不可写/);
  assert.equal(scheduler.activeCount, 0);
  assert.equal(scheduler.runningIdsSet.size, 0);
  store.change = change;
  store.saveResult = async () => { throw new Error('模拟结果保存失败'); };
  await assert.rejects(scheduler.run(job.id, true), /模拟结果保存失败/);
  assert.equal(scheduler.activeCount, 0);
  assert.equal(store.state.jobs[0].running, false);
});

test('停止后已并行发送的模型收尾，等待计划不启动且最终释放名额', async t => {
  const barrier = gate();
  const { scheduler, calls, advance } = await fixture(t, {}, {
    maxParallelJobs: 1,
    probeImpl: async input => { await barrier.promise; return completed(input); }
  });
  await scheduler.create({ pasted_text, interval_minutes: 5 });
  const queued = await scheduler.create({ pasted_text, interval_minutes: 5 });
  advance(5);
  const running = scheduler.tick();
  try {
    await waitFor(() => calls.length === 2);
    scheduler.stop();
  } finally {
    barrier.resolve();
    await running;
  }
  await scheduler.tick();
  await assert.rejects(scheduler.run(queued.id, true), /已停止/);
  assert.equal(calls.length, 2);
  assert.equal(scheduler.activeCount, 0);
});

test('并行名额只接受一至十六的整数', async () => {
  for (const maxParallelJobs of [0, 17, 1.5, NaN]) {
    assert.throws(() => new Scheduler({ store: {}, admin: {}, maxParallelJobs }), /1 至 16/);
  }
  assert.equal(new Scheduler({ store: {}, admin: {}, maxParallelJobs: 16 }).maxParallelJobs, 16);
});

test('任务使用延长后的总等待并单独传递连续无数据时限', async t => {
  const defaults = await fixture(t);
  assert.equal(defaults.scheduler.timeoutMs, 1200000);
  assert.equal(defaults.scheduler.idleTimeoutMs, 280000);
  assert.equal(defaults.scheduler.maxOutputTokens, 65536);
  const custom = await fixture(t, {}, { timeoutMs: 80000, idleTimeoutMs: 20000 });
  const job = await custom.scheduler.create({ pasted_text, models: ['model-a'], interval_minutes: 0 });
  await custom.scheduler.run(job.id, true);
  assert.equal(custom.calls[0].timeoutMs, 80000);
  assert.equal(custom.calls[0].idleTimeoutMs, 20000);
  assert.equal(custom.calls[0].maxOutputTokens, 65536);
});

test('计划保存并传递 medium/max 思考程度，结果保持实际选择', async t => {
  const { scheduler, store, calls } = await fixture(t, {}, {
    probeImpl: async input => ({ ...completed(input), reasoning_effort: input.reasoning_effort })
  });
  const medium = await scheduler.create({ pasted_text, reasoning_effort: 'medium', models: ['model-a'], interval_minutes: 0 });
  await scheduler.run(medium.id, true);
  assert.equal(calls[0].reasoning_effort, 'medium');
  assert.equal(store.state.results[0].reasoning_effort, 'medium');
  const max = await scheduler.create({ pasted_text, reasoning_effort: 'max', models: ['model-a'], interval_minutes: 0 });
  await scheduler.run(max.id, true);
  assert.equal(calls[1].reasoning_effort, 'max');
  assert.equal(store.state.results[0].reasoning_effort, 'max');
});

test('并行设置默认六、持久设置优先于环境值且降低不会改变当前运行任务', async t => {
  const barrier = gate();
  const first = await fixture(t, {}, {
    probeImpl: async input => { await barrier.promise; return completed(input); }
  });
  assert.equal(first.scheduler.maxParallelJobs, 6);
  await first.store.change(state => { state.settings = { max_parallel_jobs: 16 }; });
  const restored = new Scheduler({
    store: first.store, admin: {}, maxParallelJobs: 2,
    probeImpl: async input => completed(input)
  });
  assert.equal(restored.maxParallelJobs, 16);
  await first.scheduler.create({ pasted_text, models: ['model-a'] });
  const running = first.scheduler.run(first.store.state.jobs[0].id, true);
  await waitFor(() => first.scheduler.activeCount === 1);
  first.scheduler.setMaxParallelJobs(1);
  assert.equal(first.scheduler.activeCount, 1);
  barrier.resolve();
  await running;
});

test('来源索引按 URL 去重且不暴露密钥，快捷创建复用同一加密密钥', async t => {
  const { scheduler, store, directory } = await fixture(t);
  const original = await scheduler.create({ pasted_text, name: '原始来源', interval_minutes: 0 });
  const duplicate = await scheduler.create({ pasted_text: 'https://relay.test/v1 api_key=sk-other_123456', name: '较新来源', interval_minutes: 0 });
  const items = scheduler.sourceList();
  assert.equal(items.length, 1);
  assert.equal(items[0].source_url, 'https://relay.test/v1');
  assert.equal(items[0].source_ref, duplicate.id);
  assert.equal(items[0].key_configured, true);
  assert.equal(items[0].key_fingerprint, duplicate.key_fingerprint);
  assert.equal(items[0].prompt_ids.includes('ak47'), true);
  assert.deepEqual(items[0].jobs.map(item => item.id), [duplicate.id, original.id]);
  assert.equal(items[0].jobs[0].name, '较新来源');
  assert.equal(items[0].jobs[1].name, '原始来源');
  assert.doesNotMatch(JSON.stringify(items), /sk-(?:fixture|other)_123456/);

  const copied = await scheduler.create({ source_job_id: duplicate.id, name: '快捷来源', interval_minutes: 0 });
  assert.equal(copied.source_ref, duplicate.id);
  assert.equal(copied.source_url, duplicate.source_url);
  assert.equal(copied.key_fingerprint, duplicate.key_fingerprint);
  assert.doesNotMatch(JSON.stringify(copied), /sk-other_123456/);
  assert.equal(store.open(store.state.jobs.find(job => job.id === copied.id).secret), 'sk-other_123456');
  assert.doesNotMatch(await fs.readFile(path.join(directory, 'state.json'), 'utf8'), /sk-other_123456/);

  const copiedByURL = await scheduler.create({ source_url_ref: duplicate.source_url, name: 'URL快捷来源', interval_minutes: 0 });
  assert.equal(copiedByURL.source_ref, copied.id);
  assert.equal(copiedByURL.source_url, duplicate.source_url);
});

test('无密钥且无账号的来源不能快捷复用，账号来源按关联账号复用', async t => {
  const admin = {
    account: async id => ({ id, name: `账号${id}`, platform: 'openai', type: 'apikey', status: 'active',
      credentials: { base_url: `https://account-${id}.test/v1` }, credentials_status: { has_api_key: true } }),
    source: async id => ({ name: `账号${id}`, url: `https://account-${id}.test/v1`, source_url: `https://account-${id}.test/v1`, key: 'sk-account_123456' })
  };
  const { scheduler, store } = await fixture(t, admin);
  const accountJob = await scheduler.create({ account_ids: [17], interval_minutes: 0 });
  const source = scheduler.sourceList().find(item => item.source_url === 'https://account-17.test/v1');
  assert.equal(source.source_ref, accountJob.id);
  assert.deepEqual(source.account_ids, [17]);
  const copied = await scheduler.create({ source_url_ref: source.source_url, interval_minutes: 0 });
  assert.deepEqual(copied.account_ids, [17]);
  assert.equal(copied.source_ref, accountJob.id);
  await store.change(state => {
    state.jobs.push({ id: 'empty-source', name: '空来源', account_ids: [], sources: {}, source_url: '' });
  });
  await assert.rejects(scheduler.create({ source_job_id: 'empty-source' }), /没有可复用的密钥或账号/);
  await assert.rejects(scheduler.create({ source_job_id: 'missing-source' }), /不存在/);
  await assert.rejects(scheduler.create({ source_url_ref: 'https://missing.test/v1' }), /不存在或已失效/);
});

test('批量提示词一次提交并在满额时进入有界等待队列', async t => {
  const barrier = gate();
  const { scheduler, store, calls } = await fixture(t, {}, {
    maxParallelJobs: 1,
    probeImpl: async input => { await barrier.promise; return completed(input); }
  });
  const batch = await scheduler.createBatch({
    prompt_ids: ['ak47', 'composite'], pasted_text, models: ['model-a'], name: '批量检测', run_now: true
  });
  assert.equal(batch.items.length, 2);
  assert.equal(new Set(batch.items.map(item => item.batch_id)).size, 1);
  assert.equal(batch.items[0].name, '批量检测 - AK47');
  assert.equal(batch.items[1].name, '批量检测 - 复合提示词');
  await waitFor(() => calls.length === 1);
  assert.equal(scheduler.activeCount, 1);
  assert.equal(scheduler.queueCount, 1);
  assert.equal(scheduler.isQueued(batch.items[1].id), true);
  assert.equal(scheduler.queueRun(batch.items[1].id).status, 'queued');
  barrier.resolve();
  await waitFor(() => scheduler.activeCount === 0);
  await scheduler.tick();
  await waitFor(() => calls.length === 2);
  await waitFor(() => scheduler.activeCount === 0);
  assert.deepEqual(calls.map(item => item.prompt_id), ['ak47', 'composite']);
  assert.equal(store.state.jobs.every(job => job.batch_id === batch.batch_id), true);
});

test('批量创建校验和数量上限失败时不写入半批计划', async t => {
  const { scheduler, store } = await fixture(t);
  await assert.rejects(scheduler.createBatch({ prompt_ids: ['ak47', 'invalid'], pasted_text }), /提示词类型无效/);
  assert.equal(store.state.jobs.length, 0);
  await store.change(state => {
    state.jobs = Array.from({ length: 199 }, (_, index) => ({ id: `existing-${index}`, name: `existing-${index}` }));
  });
  await assert.rejects(scheduler.createBatch({ prompt_ids: ['ak47', 'composite'], pasted_text }), /200 个上限/);
  assert.equal(store.state.jobs.length, 199);
});
