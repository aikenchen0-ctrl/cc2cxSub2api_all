import test from 'node:test';
import assert from 'node:assert/strict';
import { DEFAULT_ERROR_GRACE_MS, parseOutput, probe } from '../lib/probe.mjs';

const KEY = 'sk-compat-secret-123456789';
const SVG = '<svg><text>完整</text></svg>';
const encoder = new TextEncoder();
const frame = (event, header = '') => `${header ? `event: ${header}\n` : ''}data: ${JSON.stringify(event)}\n\n`;
const completed = (text = '', extra = {}) => ({
  type: 'response.completed', response: { status: 'completed', output_text: text, ...extra },
});
const options = {
  url: 'https://compat.example.test', key: KEY, model: 'gpt-6-astra',
  timeoutMs: 1000, idleTimeoutMs: 500, errorGraceMs: 25,
};

function openResponse(initial, later = null, laterMs = 0) {
  let cancelled = false, timer;
  const stream = new ReadableStream({
    start(controller) {
      for (const chunk of initial) controller.enqueue(typeof chunk === 'string' ? encoder.encode(chunk) : chunk);
      if (later) timer = setTimeout(() => controller.enqueue(encoder.encode(later)), laterMs);
    },
    cancel() { cancelled = true; clearTimeout(timer); },
  });
  return {
    response: new Response(stream, { headers: { 'content-type': 'text/event-stream' } }),
    get cancelled() { return cancelled; },
  };
}

test('错误逐字段回退兼容顶层、嵌套、字符串与未完成原因', () => {
  const cases = [
    [{ type: 'error', code: 'top_code', message: '顶层原因' }, 'top_code', '顶层原因', 'upstream_error'],
    [{ type: 'error', error: 'stream_read_error' }, '', 'stream_read_error', 'upstream_error'],
    [{ type: 'response.failed', error: {}, response: { error: { code: 'nested_code', message: '嵌套原因' } } },
      'nested_code', '嵌套原因', 'upstream_error'],
    [{ type: 'response.failed', error: { code: 'direct_code' }, response: { error: { message: '嵌套消息' } } },
      'direct_code', '嵌套消息', 'upstream_error'],
    [{ type: 'response.incomplete', response: { incomplete_details: { reason: 'max_output_tokens' } } },
      'max_output_tokens', 'max_output_tokens', 'upstream_incomplete'],
    [completed('', { status: 'failed', error: { code: 'server_error', message: '实际失败' } }),
      'server_error', '实际失败', 'upstream_error'],
    [completed('', { status: 'incomplete', incomplete_details: { reason: 'content_filter' } }),
      'content_filter', 'content_filter', 'upstream_incomplete'],
  ];
  for (const [event, code, message, kind] of cases) {
    assert.throws(() => parseOutput(frame(event), 'text/event-stream'), error => {
      assert.equal(error.upstream_code, code);
      assert.equal(error.upstream_message, message);
      assert.equal(error.failure_kind, kind);
      assert.equal(error.upstream_event_type, event.type);
      return true;
    });
  }
  assert.throws(() => parseOutput(JSON.stringify({
    status: 'incomplete', incomplete_details: { reason: 'max_output_tokens' },
  })), error => error.upstream_code === 'max_output_tokens' && error.failure_kind === 'upstream_incomplete');
});

test('事件头补全缺失类型，正文类型优先，并保留原始事件类型', () => {
  assert.equal(parseOutput(frame({ response: { status: 'completed', output_text: SVG } }, 'response.completed')).text, SVG);
  assert.equal(parseOutput(frame(completed(SVG), 'error')).text, SVG);
  assert.throws(() => parseOutput(frame({ code: 'header_code', message: '事件头原因' }, 'error')), error =>
    error.upstream_code === 'header_code' && error.upstream_event_type === 'error');
});

test('完成片段替换同索引增量，多个完成载体不重复叠加', () => {
  const body = [
    { type: 'response.output_item.added', output_index: 0, item: { id: 'msg_1', type: 'message' } },
    { type: 'response.output_text.delta', item_id: 'msg_1', content_index: 0, delta: '<svg>' },
    { type: 'response.output_text.done', output_index: 0, content_index: 0, text: SVG },
    { type: 'response.content_part.done', item_id: 'msg_1', content_index: 0, part: { type: 'output_text', text: SVG } },
    { type: 'response.output_item.done', output_index: 0, item: { id: 'msg_1', content: [{ type: 'output_text', text: SVG }] } },
    completed(),
  ].map(event => frame(event)).join('');
  assert.equal(parseOutput(body).text, SVG);
  assert.equal(parseOutput(body.replace(frame(completed()), frame(completed('<svg id="authoritative"/>')))).text,
    '<svg id="authoritative"/>');
});

test('仅完成正文、默认索引和多输出片段均可完整恢复', () => {
  const cases = [
    [{ type: 'response.output_text.done', text: SVG }],
    [{ type: 'response.content_part.done', part: { type: 'output_text', text: SVG } }],
    [{ type: 'response.output_item.done', item: { id: 'only', content: [{ type: 'output_text', text: SVG }] } }],
    [
      { type: 'response.output_text.delta', delta: '<svg>' },
      { type: 'response.output_item.done', item: { id: 'late-id', content: [{ type: 'output_text', text: SVG }] } },
    ],
    [
      { type: 'response.output_text.done', output_index: 1, content_index: 0, text: '</svg>' },
      { type: 'response.output_text.done', output_index: 0, content_index: 1, text: '<text>完整</text>' },
      { type: 'response.output_text.done', output_index: 0, content_index: 0, text: '<svg>' },
    ],
  ];
  for (const events of cases) assert.equal(parseOutput([...events, completed()].map(event => frame(event)).join('')).text, SVG);
});

test('普通错误后恢复完成可以成功，错误事件不会提前取消下一分块', async () => {
  assert.equal(DEFAULT_ERROR_GRACE_MS, 1000);
  const error = frame({ code: 'transient', message: '暂时失败' }, 'error');
  const completion = frame(completed(SVG));
  assert.equal(parseOutput(error + completion).text, SVG);
  const source = openResponse([error], completion, 15);
  const result = await probe({ ...options, errorGraceMs: 100, fetchImpl: async () => source.response });
  assert.equal(result.status, 'success');
  assert.equal(result.svg, SVG);
  assert.equal(source.cancelled, true);
});

test('错误后恢复正文允许继续完成，普通心跳不能无限续期', async () => {
  const error = frame({ type: 'error', message: '暂时失败' });
  const source = openResponse([
    error + frame({ type: 'response.output_text.delta', delta: SVG }),
  ], frame(completed()), 60);
  const recovered = await probe({ ...options, fetchImpl: async () => source.response });
  assert.equal(recovered.status, 'success');
  const heartbeat = openResponse([error], ': heartbeat\n\n' + frame({ type: 'response.ping' }), 10);
  const failed = await probe({ ...options, fetchImpl: async () => heartbeat.response });
  assert.equal(failed.failure_kind, 'upstream_error');
  assert.equal(failed.upstream_message, '暂时失败');
  assert.ok(failed.duration_ms < 300);
});

test('普通错误后连接保持开启，在宽限期结束返回真实上游原因', async () => {
  const source = openResponse([frame({ type: 'error', code: 'busy', message: '服务繁忙' })]);
  const result = await probe({ ...options, fetchImpl: async () => source.response });
  assert.equal(result.failure_kind, 'upstream_error');
  assert.equal(result.upstream_code, 'busy');
  assert.equal(result.upstream_event_type, 'error');
  assert.equal(result.timeout_kind, '');
  assert.ok(result.duration_ms >= 15 && result.duration_ms < 300);
  assert.equal(source.cancelled, true);
});

test('明确失败事件立即收尾，保留终止正文和用量', async () => {
  const source = openResponse([frame({
    response: { status: 'failed', output_text: '<svg>partial', usage: { output_tokens: 12 },
      model: 'upstream-model', error: { code: 'server_error', message: '失败' } },
  }, 'response.failed')]);
  const result = await probe({ ...options, errorGraceMs: 1000, fetchImpl: async () => source.response });
  assert.equal(result.failure_kind, 'upstream_error');
  assert.equal(result.response_text, '<svg>partial');
  assert.deepEqual(result.usage, { output_tokens: 12 });
  assert.equal(result.returned_model, 'upstream-model');
  assert.equal(result.upstream_event_type, 'response.failed');
  assert.ok(result.duration_ms < 300);
});

test('本地计时器早于错误宽限期到期，保留本地超时与部分错误证据', async () => {
  const source = openResponse([
    frame({ type: 'response.output_text.delta', delta: '<svg>partial' }),
    frame({ type: 'error', code: 'busy', message: '服务繁忙' }),
  ]);
  const result = await probe({ ...options, idleTimeoutMs: 30, errorGraceMs: 200, fetchImpl: async () => source.response });
  assert.equal(result.failure_kind, 'timeout');
  assert.equal(result.timeout_kind, 'idle');
  assert.equal(result.response_text, '<svg>partial');
  assert.equal(result.upstream_code, 'busy');
  assert.equal(result.upstream_event_type, 'error');
});

test('传输中断保留已经收到的正文，不误认为模型完成', async () => {
  let count = 0;
  const stream = new ReadableStream({
    pull(controller) {
      if (!count++) controller.enqueue(encoder.encode(frame({ type: 'response.output_text.delta', delta: '<svg>partial' })));
      else controller.error(new Error('socket failed'));
    },
  }, { highWaterMark: 0 });
  const result = await probe({ ...options, fetchImpl: async () => new Response(stream, {
    headers: { 'content-type': 'text/event-stream' },
  }) });
  assert.equal(result.failure_kind, 'upstream_error');
  assert.equal(result.upstream_code, 'upstream_stream_read_error');
  assert.equal(result.response_text, '<svg>partial');
  assert.equal(result.svg, '');
});

test('跨字节 UTF-8、CRLF、多行 data 以及大型注释使用同一解析规则', async () => {
  const completion = [
    'event: response.completed', 'data: {"response":{"status":"completed",',
    `data: "output_text":${JSON.stringify(SVG)}}}`, '', '',
  ].join('\r\n');
  const chunks = [encoder.encode(`:${'x'.repeat(1024 * 1024)}\r\n\r\n`),
    ...Array.from(encoder.encode(completion), value => Uint8Array.of(value))];
  const source = openResponse(chunks);
  const result = await probe({ ...options, fetchImpl: async () => source.response });
  assert.equal(result.status, 'success');
  assert.equal(result.svg, SVG);
});

test('错误码、事件类型、局部正文和消息保存前统一精确脱敏', async () => {
  const body = [
    frame({ type: 'response.output_text.delta', delta: `<svg>${KEY}` }),
    frame({ type: `error-${KEY}`, error: { code: KEY, message: `failed ${KEY}` }, model: KEY }),
  ].join('');
  const result = await probe({ ...options, fetchImpl: async () => new Response(body, {
    headers: { 'content-type': 'text/event-stream' },
  }) });
  for (const field of ['response_text', 'upstream_code', 'upstream_message', 'upstream_event_type', 'returned_model', 'error']) {
    assert.ok(result[field].includes('[密钥已隐藏]'), field);
    assert.ok(!result[field].includes(KEY), field);
  }
});
