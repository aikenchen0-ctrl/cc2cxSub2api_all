import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { Store } from '../lib/store.mjs';

async function fixture(t) {
  const temporaryRoot = path.resolve(os.tmpdir());
  const directory = await fs.mkdtemp(path.join(temporaryRoot, 'svg-results-test-'));
  t.after(async () => {
    const resolved = path.resolve(directory);
    assert.equal(path.dirname(resolved), temporaryRoot);
    assert.ok(path.basename(resolved).startsWith('svg-results-test-'));
    await fs.rm(resolved, { recursive: true, force: true });
  });
  return { directory, store: await new Store(directory).init() };
}

function result(index) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg"><text>${index}</text></svg>`;
  return {
    status: 'success',
    job_id: `job-${index}`,
    prompt_id: index % 2 ? 'composite' : 'ak47',
    model: index % 2 ? 'gpt-6-astra' : 'gpt-6.1-sol',
    svg,
    response_text: svg,
  };
}

test('五十个并行结果全部保留独立产物，重启后仍能逐一打开', async t => {
  const { directory, store } = await fixture(t);
  const inputs = Array.from({ length: 50 }, (_, index) => result(index));
  const saved = await Promise.allSettled(inputs.map(input => store.saveResult(input)));
  assert.equal(saved.filter(item => item.status === 'rejected').length, 0);
  const ids = saved.map(item => item.value);
  assert.equal(new Set(ids).size, inputs.length);
  assert.equal(store.state.results.length, inputs.length);
  assert.equal((await fs.readdir(path.join(directory, 'artifacts'))).length, inputs.length);
  const restored = await new Store(directory).init();
  assert.equal(restored.state.results.length, inputs.length);
  for (const [index, id] of ids.entries()) {
    assert.deepEqual(await restored.artifact(id), {
      svg: inputs[index].svg,
      response_text: inputs[index].response_text,
    });
    assert.equal(restored.state.results.find(item => item.id === id).job_id, `job-${index}`);
  }
});

test('结果登记失败会向调用方报错、清理孤立产物，后续保存可以恢复', async t => {
  const { directory, store } = await fixture(t);
  const originalChange = store.change.bind(store);
  let failOnce = true;
  store.change = async mutator => {
    if (failOnce) {
      failOnce = false;
      throw new Error('模拟状态文件写入失败');
    }
    return originalChange(mutator);
  };
  await assert.rejects(store.saveResult(result(0)), /模拟状态文件写入失败/);
  assert.equal(store.state.results.length, 0);
  assert.deepEqual(await fs.readdir(path.join(directory, 'artifacts')), []);
  const next = result(1);
  const id = await store.saveResult(next);
  assert.deepEqual(await store.artifact(id), {
    svg: next.svg,
    response_text: next.response_text,
  });
  assert.equal(store.state.results.length, 1);
});
