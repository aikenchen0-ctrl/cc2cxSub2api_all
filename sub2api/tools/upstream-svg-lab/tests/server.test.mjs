import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { createLab } from '../server.mjs';

test('服务完成解析、保存、运行和沙箱结果流程，不向浏览器暴露任何密钥', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-http-'));
  const lab = await createLab({ dataDir: directory, admin: { list: async () => [] }, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { await new Promise(resolve => lab.server.close(resolve)); await fs.rm(directory, { recursive: true, force: true }); });
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  const config = await fetch(`${origin}/api/config`).then(response => response.json());
  assert.equal(config.timeout_ms, 1200000);
  assert.equal(config.idle_timeout_ms, 280000);
  assert.equal(config.max_output_tokens, 65536);
  assert.deepEqual(config.reasoning_efforts, ['low', 'medium', 'high', 'max']);
  const headers = { 'content-type': 'application/json', 'x-lab-token': config.csrf_token };
  assert.equal((await fetch(`${origin}/api/jobs`, { method: 'POST', body: '{}' })).status, 403);
  const posted = await fetch(`${origin}/api/jobs`, {
    method: 'POST', headers, body: JSON.stringify({ pasted_text: 'https://relay.test/v1 key: sk-fixture_123456', enabled: false })
  });
  assert.equal(posted.status, 201);
  const job = await posted.json();
  assert.equal(job.secret, undefined);
  assert.doesNotMatch(JSON.stringify(job), /sk-fixture/);
  const sources = await fetch(`${origin}/api/sources`, { headers }).then(response => response.json());
  assert.equal(sources.items.length, 1);
  assert.equal(sources.items[0].source_url, 'https://relay.test/v1');
  assert.equal(sources.items[0].source_job_id, job.id);
  assert.equal(sources.items[0].key_configured, true);
  assert.doesNotMatch(JSON.stringify(sources), /sk-fixture/);
  const shortcut = await fetch(`${origin}/api/jobs`, {
    method: 'POST', headers, body: JSON.stringify({ source_job_id: job.id, enabled: false })
  });
  assert.equal(shortcut.status, 201);
  const shortcutJob = await shortcut.json();
  assert.equal(shortcutJob.source_ref, job.id);
  assert.equal(shortcutJob.source_url, job.source_url);
  assert.doesNotMatch(JSON.stringify(shortcutJob), /sk-fixture/);
  const invalidShortcut = await fetch(`${origin}/api/jobs`, {
    method: 'POST', headers, body: JSON.stringify({ source_job_id: 'missing-source', enabled: false })
  });
  assert.equal(invalidShortcut.status, 400);
  assert.match((await invalidShortcut.json()).error, /不存在/);
  lab.scheduler.probeImpl = async input => ({
    status: 'success', model: input.model, source_url: input.url, reasoning_effort: 'low', duration_ms: 1,
    svg: '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>', response_text: 'svg'
  });
  assert.equal((await fetch(`${origin}/api/jobs/${job.id}/run`, { method: 'POST', headers })).status, 202);
  while (lab.scheduler.active) await new Promise(resolve => setImmediate(resolve));
  const results = await fetch(`${origin}/api/results`).then(response => response.json());
  assert.equal(results.items.length, 2);
  assert.doesNotMatch(JSON.stringify(results), /sk-fixture|secret/);
  const artifact = await fetch(`${origin}/api/results/${results.items[0].id}/artifact`);
  assert.match(artifact.headers.get('content-security-policy'), /sandbox; default-src 'none'/);
  assert.match(await artifact.text(), /<svg/);
  const crossSite = await fetch(`${origin}/api/jobs`, { method: 'POST', headers: { ...headers, Origin: 'https://other.test' }, body: '{}' });
  assert.equal(crossSite.status, 403);
});

test('公开路径允许匿名读作品，管理操作要求独立口令且不返回匿名 CSRF 凭据', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-public-'));
  const accessToken = 'synthetic-management-token-1234567890';
  const lab = await createLab({ dataDir: directory, admin: { list: async () => [] }, startScheduler: false,
    publicOrigin: 'https://gallery.example.test', basePath: '/ak47', accessToken });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { await new Promise(resolve => lab.server.close(resolve)); await fs.rm(directory, { recursive: true, force: true }); });
  const base = `http://127.0.0.1:${lab.server.address().port}/ak47`;
  const anonymous = await fetch(`${base}/api/config`).then(response => response.json());
  assert.equal(anonymous.authenticated, false);
  assert.equal(anonymous.csrf_token, undefined);
  assert.equal((await fetch(`${base}/api/results`)).status, 200);
  assert.equal((await fetch(`${base}/api/jobs`)).status, 401);
  assert.equal((await fetch(`${base}/api/accounts`)).status, 401);
  const headers = { 'x-lab-access': accessToken };
  const config = await fetch(`${base}/api/config`, { headers }).then(response => response.json());
  assert.equal(config.authenticated, true);
  assert.ok(config.csrf_token);
  const post = await fetch(`${base}/api/jobs`, {
    method: 'POST', headers: { ...headers, 'x-lab-token': config.csrf_token, 'content-type': 'application/json' },
    body: JSON.stringify({ pasted_text: 'https://relay.test/v1 key: sk-fixture_123456', enabled: false })
  });
  assert.equal(post.status, 201);
  assert.equal((await fetch(`${base}/app.js`)).status, 200);
  assert.equal((await fetch(`${base}/style.css`)).status, 200);
  assert.match(await fetch(`${base}/`).then(response => response.text()), /lang="zh-CN"/);
});

test('公开部署接受用户指定的三字符管理口令', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-short-access-'));
  const lab = await createLab({
    dataDir: directory,
    admin: { list: async () => [] },
    startScheduler: false,
    publicOrigin: 'https://gallery.example.test',
    basePath: '/ak47',
    accessToken: '321'
  });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => {
    await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(directory, { recursive: true, force: true });
  });
  const base = `http://127.0.0.1:${lab.server.address().port}/ak47`;
  assert.equal((await fetch(`${base}/api/config`, { headers: { 'X-Lab-Access': '321' } })).status, 200);
  assert.equal((await fetch(`${base}/api/jobs`, { headers: { 'X-Lab-Access': '321' } })).status, 200);
  assert.equal((await fetch(`${base}/api/jobs`, { headers: { 'X-Lab-Access': 'bad' } })).status, 401);
});

test('结果画廊接口包含运行中状态和实时耗时，完成后只保留正式结果', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-live-result-'));
  let finish;
  const lab = await createLab({
    dataDir: directory,
    admin: { list: async () => [] },
    startScheduler: false,
    timeoutMs: 5000
  });
  lab.scheduler.probeImpl = async input => new Promise(resolve => {
    finish = () => resolve({
      status: 'success',
      model: input.model,
      source_url: input.url,
      reasoning_effort: 'low',
      duration_ms: 25,
      svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
      response_text: '<svg xmlns="http://www.w3.org/2000/svg"/>'
    });
  });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  t.after(async () => {
    lab.scheduler.stop();
    await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(directory, { recursive: true, force: true });
  });
  const config = await fetch(`${origin}/api/config`).then(response => response.json());
  const headers = { 'content-type': 'application/json', 'x-lab-token': config.csrf_token };
  const created = await fetch(`${origin}/api/jobs`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ pasted_text: 'https://relay.test/v1 key: sk-fixture_123456', models: ['test-model'] })
  });
  const job = await created.json();
  const running = lab.scheduler.run(job.id, true);
  while (!finish) await new Promise(resolve => setImmediate(resolve));
  const live = await fetch(`${origin}/api/results`).then(response => response.json());
  assert.equal(live.items.length, 1);
  assert.equal(live.items[0].status, 'running');
  assert.equal(live.items[0].live, true);
  assert.ok(live.items[0].duration_ms >= 0);
  assert.equal(live.items[0].response_text, undefined);
  finish();
  await running;
  const completed = await fetch(`${origin}/api/results`).then(response => response.json());
  assert.equal(completed.items.length, 1);
  assert.equal(completed.items[0].status, 'success');
  assert.equal(completed.items[0].live, undefined);
});

test('设置接口支持一至十六并持久化并行上限，错误 CSRF 返回稳定代码', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-settings-'));
  const lab = await createLab({ dataDir: directory, admin: { list: async () => [] }, startScheduler: false, maxParallelJobs: 2 });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  const close = () => new Promise(resolve => lab.server.close(resolve));
  t.after(async () => { await close(); await fs.rm(directory, { recursive: true, force: true }); });
  const config = await fetch(`${origin}/api/config`).then(response => response.json());
  assert.equal((await fetch(`${origin}/api/settings`)).status, 200);
  const invalid = await fetch(`${origin}/api/settings`, {
    method: 'PATCH', headers: { 'content-type': 'application/json', 'x-lab-token': 'expired' },
    body: JSON.stringify({ max_parallel_jobs: 8 })
  });
  assert.equal(invalid.status, 403);
  assert.equal((await invalid.json()).code, 'CSRF_EXPIRED');
  const changed = await fetch(`${origin}/api/settings`, {
    method: 'PATCH', headers: { 'content-type': 'application/json', 'x-lab-token': config.csrf_token },
    body: JSON.stringify({ max_parallel_jobs: 16 })
  });
  assert.equal(changed.status, 200);
  assert.equal((await changed.json()).max_parallel_jobs, 16);
  await close();
  const restored = await createLab({ dataDir: directory, admin: { list: async () => [] }, startScheduler: false, maxParallelJobs: 2 });
  await new Promise(resolve => restored.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { await new Promise(resolve => restored.server.close(resolve)); });
  const restoredOrigin = `http://127.0.0.1:${restored.server.address().port}`;
  assert.equal((await fetch(`${restoredOrigin}/api/settings`).then(response => response.json())).max_parallel_jobs, 16);
});
