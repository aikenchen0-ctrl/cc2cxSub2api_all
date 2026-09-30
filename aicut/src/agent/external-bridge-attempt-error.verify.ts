// Verify: an editor-registration/poll HTTP 409 is a recoverable retry condition
// (runBridge re-registers within ~1s) and must not flash a blocking "close the
// other window" message, navigate away, or reload. Non-409 errors still surface.
import assert from 'node:assert/strict';

let reloadCalls = 0;
(globalThis as unknown as { window?: unknown }).window = {
  location: { reload: () => { reloadCalls += 1; } },
};

const { handleExternalBridgeAttemptError } = await import('./external-bridge-attempt-error.ts');
const { EditorBridgeRequestError } = await import('./external-bridge-registration.ts');

function bridgeError(operation: string, status: number): unknown {
  return new EditorBridgeRequestError(operation, status);
}

const noopSignal = { aborted: false } as unknown as AbortSignal;
const errors: string[] = [];
const onError = (message: string | null) => { if (message) errors.push(message); };

reloadCalls = 0;
for (let i = 0; i < 100; i++) {
  handleExternalBridgeAttemptError(bridgeError('registration', 409), noopSignal, onError);
  handleExternalBridgeAttemptError(bridgeError('poll', 409), noopSignal, onError);
  handleExternalBridgeAttemptError(bridgeError('cancellation', 409), noopSignal, onError);
}
assert.equal(reloadCalls, 0, 'recoverable 409 must not navigate away from the editor');
assert.equal(errors.length, 0,
  'a transient 409 must not surface a blocking "close other window" message; runBridge retries it');

const before = reloadCalls;
handleExternalBridgeAttemptError(new Error('boom'), noopSignal, onError);
assert.match(errors.at(-1) ?? '', /boom/, 'a genuine non-409 error still surfaces');
assert.equal(reloadCalls, before, 'generic error does not reload');

console.log('external-bridge-attempt-error.verify: OK (409 retried silently, others surfaced)');

const { fetchEditorBridge, registerEditorBridge, sendEditorBridgeResult } = await import('./external-bridge-registration.ts');
const { externalBridgeErrorText } = await import('../components/chat/externalBridgeErrorText.ts');
const translate = (key: string, params?: Record<string, string | number>) =>
  key.replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? name));
const originalFetch = globalThis.fetch;
try {
  const failure = new TypeError('Failed to fetch');
  globalThis.fetch = async () => { throw failure; };
  const signal = new AbortController().signal;
  for (const operation of ['registration', 'poll', 'cancellation poll', 'result']) {
    await assert.rejects(fetchEditorBridge(operation, '/api/external-agent/test', { signal }), (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.equal(error.cause, failure);
      assert.match(externalBridgeErrorText(error.message, translate), /失败：无法连接服务，正在自动重连/);
      assert.doesNotMatch(externalBridgeErrorText(error.message, translate), /Failed to fetch/);
      return true;
    });
  }
  await assert.rejects(registerEditorBridge('project', 'editor', 'revision', signal), /registration failed: network/);
  await assert.rejects(sendEditorBridgeResult('call', 'applied', {}, signal, 'capability'), /result failed: network/);
  const aborted = new AbortController();
  aborted.abort();
  await assert.rejects(fetchEditorBridge('poll', '/api/external-agent/poll', { signal: aborted.signal }), (error) => error === failure);
  const previousErrors = errors.length;
  handleExternalBridgeAttemptError(failure, aborted.signal, onError);
  assert.equal(errors.length, previousErrors, 'unmount/cancel must not surface a network error');
  for (const message of ['Failed to fetch', 'Load failed', 'NetworkError when attempting to fetch resource.']) {
    assert.match(externalBridgeErrorText(message, translate), /同步编辑器连接失败/);
  }
  assert.match(externalBridgeErrorText('registration failed: HTTP 401', translate), /登录已失效/);
  assert.match(externalBridgeErrorText('poll failed: HTTP 403', translate), /无访问权限/);
  assert.match(externalBridgeErrorText('poll failed: HTTP 503', translate), /接收编辑任务失败（HTTP 503）/);
  assert.match(externalBridgeErrorText('invalid editor registration response', translate), /编辑器同步失败/);
  const response = new Response(null, { status: 204 });
  globalThis.fetch = async () => response;
  assert.equal(await fetchEditorBridge('poll', '/api/external-agent/poll', { signal }), response);
} finally {
  globalThis.fetch = originalFetch;
}
console.log('external-bridge network stages, localized alerts, cancellation and success: OK');
