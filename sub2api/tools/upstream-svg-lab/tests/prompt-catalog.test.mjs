import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { Store } from '../lib/store.mjs';
import { Scheduler } from '../lib/scheduler.mjs';
import { PromptCatalog } from '../lib/prompt-catalog.mjs';
import { getPrompt } from '../lib/prompts.mjs';
import { createLab } from '../server.mjs';

const pastedText = 'https://relay.test/v1 api_key=sk-prompt-catalog_123456';

async function fixture(t) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-prompt-catalog-'));
  const store = await new Store(directory).init();
  const calls = [];
  const scheduler = new Scheduler({
    store,
    admin: {},
    probeImpl: async input => {
      calls.push(input);
      return {
        source_url: input.url,
        request_url: `${input.url}/responses`,
        model: input.model,
        status: 'success',
        response_text: '<svg/>',
        svg: '<svg xmlns="http://www.w3.org/2000/svg"/>'
      };
    }
  });
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  return { directory, store, scheduler, calls };
}

test('新建提示词可立即创建计划并执行，原文和低思考参数保持一致', async t => {
  const { store, scheduler, calls, directory } = await fixture(t);
  const catalog = new PromptCatalog(store);
  const custom = await catalog.create({ label: '自定义 AK47', text: 'Write a measured AK47 SVG.' });
  const job = await scheduler.create({
    pasted_text: pastedText,
    prompt_id: custom.id,
    models: ['model-custom'],
    interval_minutes: 0
  });

  await scheduler.run(job.id, true);

  assert.equal(calls.length, 1);
  assert.equal(calls[0].prompt_id, custom.id);
  assert.equal(calls[0].prompt_label, custom.label);
  assert.equal(calls[0].prompt_version, 1);
  assert.equal(calls[0].prompt, custom.text);
  assert.equal(store.state.results[0].prompt, custom.text);
  assert.equal(store.state.results[0].reasoning_effort, 'low');
  assert.doesNotMatch(JSON.stringify(store.publicJob(store.state.jobs[0])), /sk-prompt-catalog/);
  assert.doesNotMatch(await fs.readFile(path.join(directory, 'state.json'), 'utf8'), /sk-prompt-catalog/);
});

test('内置提示词可编辑但拒绝陈旧版本，历史计划仍使用创建时快照', async t => {
  const { store, scheduler, calls } = await fixture(t);
  const catalog = new PromptCatalog(store);
  const original = getPrompt('ak47');
  const oldJob = await scheduler.create({ pasted_text: pastedText, models: ['old-model'] });

  const edited = await catalog.update('ak47', {
    version: 1,
    label: 'AK47 新版本',
    text: 'Write an edited AK47 SVG.'
  });
  assert.equal(edited.built_in, true);
  assert.equal(edited.version, 2);
  await assert.rejects(
    catalog.update('ak47', { version: 1, label: '覆盖失败', text: 'must not overwrite' }),
    error => error?.status === 409 && error?.code === 'PROMPT_VERSION_CONFLICT'
  );

  await scheduler.run(oldJob.id, true);
  const newJob = await scheduler.create({ pasted_text: pastedText, models: ['new-model'] });
  await scheduler.run(newJob.id, true);

  assert.equal(calls[0].prompt_id, 'ak47');
  assert.equal(calls[0].prompt_version, 1);
  assert.equal(calls[0].prompt, original.text);
  assert.equal(calls[1].prompt_id, 'ak47');
  assert.equal(calls[1].prompt_version, 2);
  assert.equal(calls[1].prompt, edited.text);
  assert.equal(store.state.results[1].prompt, original.text);
  assert.equal(store.state.results[0].prompt, edited.text);
});

test('Store 重启后保留自定义提示词和内置提示词的编辑版本', async t => {
  const { store, directory } = await fixture(t);
  const catalog = new PromptCatalog(store);
  const custom = await catalog.create({ label: '重启测试提示词', text: 'Custom prompt survives restart.' });
  const editedBuiltIn = await catalog.update('ak47', {
    version: 1,
    label: 'AK47 重启版本',
    text: 'Edited built-in prompt survives restart.'
  });

  const restored = await new Store(directory).init();
  const restoredCatalog = new PromptCatalog(restored);
  const restoredCustom = restoredCatalog.get(custom.id);
  const restoredBuiltIn = restoredCatalog.get('ak47');

  assert.equal(restoredCustom.text, custom.text);
  assert.equal(restoredCustom.version, custom.version);
  assert.equal(restoredCustom.built_in, false);
  assert.equal(restoredBuiltIn.text, editedBuiltIn.text);
  assert.equal(restoredBuiltIn.label, editedBuiltIn.label);
  assert.equal(restoredBuiltIn.version, editedBuiltIn.version);
  assert.equal(restoredBuiltIn.built_in, true);
});

test('提示词 HTTP 接口支持新建、编辑和版本冲突，任务结果不泄露上游密钥', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-prompt-http-'));
  const lab = await createLab({
    dataDir: directory,
    admin: { list: async () => [] },
    startScheduler: false
  });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => {
    await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(directory, { recursive: true, force: true });
  });
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  const config = await fetch(`${origin}/api/config`).then(response => response.json());
  const headers = { 'content-type': 'application/json', 'x-lab-token': config.csrf_token };

  const createdResponse = await fetch(`${origin}/api/prompts`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ label: '接口提示词', text: 'Write an interface prompt.' })
  });
  assert.equal(createdResponse.status, 201);
  const created = await createdResponse.json();
  assert.equal(created.built_in, false);

  const updatedResponse = await fetch(`${origin}/api/prompts/${encodeURIComponent(created.id)}`, {
    method: 'PATCH',
    headers,
    body: JSON.stringify({ label: '接口提示词二版', text: 'Write the edited interface prompt.', version: 1 })
  });
  assert.equal(updatedResponse.status, 200);
  const updated = await updatedResponse.json();
  assert.equal(updated.version, 2);
  assert.equal(updated.text, 'Write the edited interface prompt.');

  const staleResponse = await fetch(`${origin}/api/prompts/${encodeURIComponent(created.id)}`, {
    method: 'PATCH',
    headers,
    body: JSON.stringify({ label: '不应覆盖', text: 'must not overwrite', version: 1 })
  });
  assert.equal(staleResponse.status, 409);
  assert.equal((await staleResponse.json()).code, 'PROMPT_VERSION_CONFLICT');

  const jobResponse = await fetch(`${origin}/api/jobs`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      pasted_text: pastedText,
      prompt_id: created.id,
      models: ['interface-model'],
      interval_minutes: 0,
      enabled: false
    })
  });
  assert.equal(jobResponse.status, 201);
  const job = await jobResponse.json();
  assert.equal(job.prompt, 'Write the edited interface prompt.');
  assert.equal(job.secret, undefined);

  lab.scheduler.probeImpl = async input => ({
    source_url: input.url,
    request_url: `${input.url}/responses`,
    model: input.model,
    status: 'success',
    reasoning_effort: 'low',
    prompt: input.prompt,
    svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
    response_text: '<svg/>'
  });
  const runResponse = await fetch(`${origin}/api/jobs/${job.id}/run`, { method: 'POST', headers });
  assert.equal(runResponse.status, 202);
  while (lab.scheduler.active) await new Promise(resolve => setImmediate(resolve));

  const results = await fetch(`${origin}/api/results`).then(response => response.json());
  assert.equal(results.items.length, 1);
  assert.equal(results.items[0].prompt, 'Write the edited interface prompt.');
  assert.doesNotMatch(JSON.stringify(results), /sk-prompt-catalog_123456/);
  assert.doesNotMatch(await fs.readFile(path.join(directory, 'state.json'), 'utf8'), /sk-prompt-catalog_123456/);
});
