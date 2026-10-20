import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { probe } from '../lib/probe.mjs';
import { createLab } from '../server.mjs';

const source = { url: 'https://upstream.invalid', key: 'sk-diagnostic_fixture_123456', model: 'model-a' };

test('完成后只有文字、没有正文与截断 SVG 分别归类，不制造图形内容', async () => {
  for (const [text, expected] of [
    ['没有文件编辑工具，无法创建 SVG 文件。', 'upstream_no_svg'],
    ['', 'upstream_empty_output'],
    ['<svg><g><path d="M0 0"/>', 'local_svg_validation']
  ]) {
    const result = await probe({
      ...source, fetchImpl: async () => new Response(JSON.stringify({ status: 'completed', output_text: text }))
    });
    assert.equal(result.failure_kind, expected);
    assert.equal(result.status, 'failed');
    assert.equal(result.svg, '');
    assert.equal(result.response_text, text);
  }
});

test('原始响应接口仅管理员可用，正文不进入公开列表，旧记录仍可核对', async t => {
  const dataDir = await fs.mkdtemp(path.join(os.tmpdir(), 'svg-response-details-'));
  const accessToken = 'test-management-access';
  const lab = await createLab({ dataDir, admin: {}, accessToken, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => {
    await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(dataDir, { recursive: true, force: true });
  });
  const base = `http://127.0.0.1:${lab.server.address().port}`;
  const result = await probe({
    ...source, fetchImpl: async () => new Response(JSON.stringify({
      status: 'completed', output_text: `没有文件编辑工具；诊断回显 ${source.key}`
    }))
  });
  const id = await lab.store.saveResult(result);
  const items = await fetch(`${base}/api/results`).then(response => response.json());
  assert.equal(items.items[0].has_response_text, true);
  assert.equal(items.items[0].response_text, undefined);
  assert.doesNotMatch(JSON.stringify(items), /没有文件编辑工具|sk-diagnostic_fixture/);
  assert.equal((await fetch(`${base}/api/results/${id}/response`)).status, 401);
  const detail = await fetch(`${base}/api/results/${id}/response`, {
    headers: { 'X-Lab-Access': accessToken }
  }).then(response => response.json());
  assert.match(detail.response_text, /没有文件编辑工具/);
  assert.doesNotMatch(JSON.stringify(detail), /sk-diagnostic_fixture/);
  assert.equal(detail.diagnostics.failure_kind, 'upstream_no_svg');
  assert.match(detail.diagnostics.analysis_hint, /没有 SVG 根标签/);
  const historical = await lab.store.saveResult({ status: 'failed', svg: '', response_text: '', failure_kind: 'upstream_error' });
  await lab.store.change(state => { delete state.results.find(item => item.id === historical).has_response_text; });
  const old = await fetch(`${base}/api/results/${historical}/response`, {
    headers: { 'X-Lab-Access': accessToken }
  }).then(response => response.json());
  assert.equal(old.has_response_text, false);
  assert.match(old.diagnostics.analysis_hint, /旧记录未保存正文/);
  const missing = await fetch(`${base}/api/results/00000000-0000-0000-0000-000000000000/response`, {
    headers: { 'X-Lab-Access': accessToken }
  });
  assert.equal(missing.status, 404);
});
