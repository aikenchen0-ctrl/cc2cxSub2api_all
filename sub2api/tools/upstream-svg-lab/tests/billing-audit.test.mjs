import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { analyzeBillingCSV, compareBillingAudits } from '../lib/billing-audit.mjs';
import { createLab } from '../server.mjs';

const csv = [
  'Model,Input Tokens,Output Tokens,Cache Read Tokens,Cache Creation 5m Tokens,Cache Creation 1h Tokens,Billed Cost,Original Cost,Input Price,Output Price,Cache Read Price,Cache Write 5m Price,Cache Write 1h Price,Rate Multiplier',
  '"model,one",1000000,100000,200000,300000,0,1.23,1.40,1.1,2.2,0.11,0.55,0.77,0.9',
  '"model,one",500000,50000,100000,0,400000,0.82,0.91,1.1,2.2,0.11,0.55,0.77,0.9',
  '"model,one",500000,50000,100000,0,400000,0.75,0.91,1.3,2.2,0.11,0.55,0.77,0.8',
].join('\n');

test('计费 CSV 解析引号字段、缓存分栏和混合费率', () => {
  const audit = analyzeBillingCSV(csv, { filename: 'quoted.csv' });
  assert.equal(audit.row_count, 3);
  assert.equal(audit.models.length, 2);
  const model = audit.models.find(item => item.rate_tier === 0.9);
  assert.equal(model.model, 'model,one');
  assert.equal(model.rates.input.per_million_tokens, 1.1);
  assert.equal(model.rates.cache_write_5m.per_million_tokens, 0.55);
  assert.equal(model.rates.cache_write_1h.per_million_tokens, 0.77);
  assert.equal(model.rate_tier, 0.9);
  assert.equal(audit.summary.cache_write_split_observed, true);
});

test('计费 CSV 没有拆分缓存写入时明确标记证据不足', () => {
  const input = [
    'Model,Input Tokens,Output Tokens,Cache Read Tokens,Cache Creation Tokens,Billed Cost',
    'gpt-test,100,10,20,30,0.5',
    'gpt-test,200,20,40,60,1.0',
  ].join('\n');
  const audit = analyzeBillingCSV(input);
  assert.equal(audit.summary.cache_write_split_observed, false);
  assert.ok(audit.models[0].issues.some(issue => issue.includes('无法区分 5 分钟与 1 小时')));
  assert.equal(audit.models[0].rates.cache_write_5m.source, 'not_identifiable');
});

test('共线特征不伪造价格，非法和负数行会跳过并计数', () => {
  const input = [
    'Model,Reasoning Effort,Input Tokens,Output Tokens,Billed Cost',
    'gpt-test,low,100,10,0.4',
    'gpt-test,low,200,20,0.8',
    'gpt-test,low,broken,30,0.9',
    'gpt-test,low,-10,20,0.2',
  ].join('\n');
  const audit = analyzeBillingCSV(input);
  const model = audit.models[0];
  assert.equal(audit.skipped_rows, 2);
  assert.equal(model.confidence, 'low');
  assert.equal(model.quality.input.zero, 0);
  assert.equal(model.quality.output.missing, 0);
  assert.equal(model.quality.input.invalid, 1);
  assert.equal(model.quality.input.negative, 1);
  assert.ok(model.issues.some(issue => issue.includes('非法或负数')));
  assert.ok(model.issues.some(issue => issue.includes('共线') || issue.includes('有效样本不足')));
  assert.equal(model.effective_rates.input.per_million_tokens, null);
});

test('基准比较只比较同一费率档位，并把不可识别价格标为证据不足', () => {
  const baseline = analyzeBillingCSV([
    'Model,Reasoning Effort,Rate Multiplier,Input Tokens,Output Tokens,Original Cost',
    'gpt-test,low,1,1000000,0,2',
    'gpt-test,low,1,0,1000000,10',
  ].join('\n'));
  const current = analyzeBillingCSV([
    'Model,Reasoning Effort,Rate Multiplier,Input Tokens,Output Tokens,Original Cost',
    'gpt-test,low,1,1000000,0,2.4',
    'gpt-test,low,1,0,1000000,10',
  ].join('\n'));
  const comparison = compareBillingAudits(current, baseline);
  assert.ok(comparison.changes.some(item => item.rate === 'input' && item.status === 'changed'));
  assert.ok(comparison.changes.some(item => item.rate === 'cache_write_5m' && item.status === 'insufficient'));
});

test('管理员计费审计接口接收 multipart 文件，只保存分析结果不保存原文', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'billing-audit-'));
  const lab = await createLab({ dataDir: directory, admin: { list: async () => [] }, startScheduler: false });
  await new Promise(resolve => lab.server.listen(0, '127.0.0.1', resolve));
  t.after(async () => {
    await new Promise(resolve => lab.server.close(resolve));
    await fs.rm(directory, { recursive: true, force: true });
  });
  const origin = `http://127.0.0.1:${lab.server.address().port}`;
  const config = await fetch(`${origin}/api/config`).then(response => response.json());
  const form = new FormData();
  form.set('file', new File([csv.replaceAll('\n', '\r\n')], 'usage.csv', { type: 'text/csv' }));
  const response = await fetch(`${origin}/api/billing-audits`, {
    method: 'POST',
    headers: { 'x-lab-token': config.csrf_token },
    body: form,
  });
  assert.equal(response.status, 201);
  const audit = await response.json();
  assert.equal(audit.filename, 'usage.csv');
  assert.equal(audit.row_count, 3);
  const stateText = await fs.readFile(path.join(directory, 'state.json'), 'utf8');
  assert.doesNotMatch(stateText, /1000000,100000,200000,300000/);
  const listed = await fetch(`${origin}/api/billing-audits`).then(result => result.json());
  assert.equal(listed.items.length, 1);
  assert.equal(listed.items[0].id, audit.id);
});
