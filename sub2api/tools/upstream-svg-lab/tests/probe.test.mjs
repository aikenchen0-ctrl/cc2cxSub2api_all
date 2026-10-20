import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'

import { DEFAULT_IDLE_TIMEOUT_MS, DEFAULT_TIMEOUT_MS, DEFAULT_MAX_OUTPUT_TOKENS, MAX_RESPONSE_BYTES, MODELS, PROMPT, extractSVG, probe } from '../lib/probe.mjs'
import { getPrompt } from '../lib/prompts.mjs'

const KEY = 'sk-live_1234567890'
const SOURCE_URL = 'https://api.example.test/v1/'
const REQUEST_URL = 'https://api.example.test/v1/responses'
const EXPECTED_RESPONSE_LIMIT = 64 * 1024 * 1024
const encoder = new TextEncoder()

function fakeResponse(body, { status = 200, contentType = 'application/json' } = {}) {
  return new Response(body, {
    status,
    headers: { 'content-type': contentType },
  })
}

function jsonResponse(value, options) {
  return fakeResponse(JSON.stringify(value), options)
}

function successJSON(model = MODELS[0]) {
  return {
    status: 'completed',
    model,
    output_text: '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>',
  }
}

function successSSE(model = MODELS[0]) {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>'
  return [
    `data: ${JSON.stringify({ type: 'response.output_text.delta', delta: svg })}`,
    `data: ${JSON.stringify({ type: 'response.completed', response: { status: 'completed', model, output_text: svg } })}`,
    'data: [DONE]',
    '',
  ].join('\n\n')
}

function fetchRecorder(response) {
  const calls = []
  const fetchImpl = async (url, init) => {
    calls.push({ url, init })
    return response
  }
  return { calls, fetchImpl }
}

function streamedResponse(chunks, { status = 200, contentType = 'text/event-stream', contentLength } = {}) {
  let index = 0
  let cancelled = false
  const stream = new ReadableStream({
    pull(controller) {
      if (index >= chunks.length) {
        controller.close()
        return
      }
      const chunk = chunks[index++]
      controller.enqueue(typeof chunk === 'string' ? encoder.encode(chunk) : chunk)
    },
    cancel() {
      cancelled = true
    },
  })
  const headers = { 'content-type': contentType }
  if (contentLength !== undefined) headers['content-length'] = String(contentLength)
  return {
    // 直接提供最小 Response 兼容对象，确保测试能观察 reader.cancel() 是否
    // 传播到原始 ReadableStream；真实 fetch 返回的 Response 仍由其他测试覆盖。
    response: {
      status,
      ok: status >= 200 && status < 300,
      headers: new Headers(headers),
      body: stream,
    },
    get cancelled() {
      return cancelled
    },
    get chunksRead() {
      return index
    },
  }
}

function streamedTextResponse(text, options = {}) {
  const chunkSize = options.chunkSize || 8192
  const chunks = []
  for (let offset = 0; offset < text.length; offset += chunkSize) {
    chunks.push(encoder.encode(text.slice(offset, offset + chunkSize)))
  }
  return streamedResponse(chunks, options)
}

function streamedRepeatedResponse(chunk, count, options = {}) {
  return streamedResponse(Array.from({ length: count }, () => chunk), options)
}

function byteLength(chunks) {
  return chunks.reduce((total, chunk) => total + (typeof chunk === 'string' ? encoder.encode(chunk).byteLength : chunk.byteLength), 0)
}

function openStreamResponse(chunks = [], { neverFinishCancel = false } = {}) {
  let cancelled = false
  let index = 0
  const body = new ReadableStream({
    pull(controller) {
      if (index < chunks.length) {
        const chunk = chunks[index++]
        controller.enqueue(typeof chunk === 'string' ? encoder.encode(chunk) : chunk)
      }
    },
    cancel() {
      cancelled = true
      if (neverFinishCancel) return new Promise(() => {})
    },
  })
  return {
    response: { status: 200, ok: true, body, headers: new Headers({ 'content-type': 'text/event-stream' }) },
    get cancelled() { return cancelled },
  }
}

test('probe constants keep the model comparison contract', () => {
  assert.equal(PROMPT, 'Write an AK47 SVG at the same scale as reality')
  assert.deepEqual(MODELS, ['gpt-6.1-sol', 'gpt-6-astra'])
  assert.equal(DEFAULT_TIMEOUT_MS, 1_200_000)
  assert.equal(DEFAULT_IDLE_TIMEOUT_MS, 280_000)
  assert.equal(DEFAULT_MAX_OUTPUT_TOKENS, 65_536)
})

test('probe sends the exact SVG prompt with low reasoning and records the endpoint', async () => {
  const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.source_url, 'https://api.example.test/v1')
  assert.equal(result.request_url, REQUEST_URL)
  assert.equal(result.model, MODELS[0])
  assert.equal(result.reasoning_effort, 'low')
  assert.equal(result.prompt_id, 'ak47')
  assert.equal(result.prompt_label, 'AK47')
  assert.equal(result.prompt, PROMPT)
  assert.equal(result.response_text, result.svg)
  assert.equal(result.timeout_ms, 1_200_000)
  assert.equal(result.idle_timeout_ms, 280_000)
  assert.match(result.svg, /^<svg[\s\S]*<\/svg>$/)
  assert.equal(calls.length, 1)

  const request = calls[0]
  assert.equal(request.url, REQUEST_URL)
  assert.equal(request.init.method, 'POST')
  assert.equal(request.init.redirect, 'error')
  assert.equal(request.init.headers.Authorization, `Bearer ${KEY}`)
  const body = JSON.parse(request.init.body)
  assert.deepEqual(body, {
    model: MODELS[0],
    input: PROMPT,
    reasoning: { effort: 'low' },
    stream: true,
    store: false,
    max_output_tokens: 65536,
  })
  assert.equal(Object.hasOwn(body, 'system'), false)
})

test('probe sends each supported reasoning effort and rejects unknown values', async () => {
  for (const effort of ['low', 'medium', 'high', 'max']) {
    const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
    const result = await probe({ url: SOURCE_URL, key: KEY, model: MODELS[0], reasoning_effort: effort, fetchImpl })
    assert.equal(result.status, 'success')
    assert.equal(result.reasoning_effort, effort)
    assert.deepEqual(JSON.parse(calls[0].init.body).reasoning, { effort })
  }
  await assert.rejects(probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], reasoning_effort: 'ultra',
    fetchImpl: fetchRecorder(jsonResponse(successJSON())).fetchImpl
  }), /思考程度/)
})

test('输出预算可配置且实际请求和结果记录保持一致', async () => {
  const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], maxOutputTokens: 98304, fetchImpl
  })
  assert.equal(result.status, 'success')
  assert.equal(result.max_output_tokens, 98304)
  assert.equal(JSON.parse(calls[0].init.body).max_output_tokens, 98304)
})

test('复合提示词全文实际发送给两个模型，记录与请求一致且均为低思考程度', async () => {
  const preset = getPrompt('composite')
  for (const model of MODELS) {
    const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON(model)))
    const result = await probe({
      url: SOURCE_URL,
      key: KEY,
      model,
      prompt_id: 'composite',
      fetchImpl,
    })
    assert.equal(calls.length, 1)
    const body = JSON.parse(calls[0].init.body)
    assert.equal(body.input, preset.text)
    assert.notEqual(body.input, PROMPT)
    assert.deepEqual(body.reasoning, { effort: 'low' })
    assert.equal(body.model, model)
    assert.equal(body.stream, true)
    assert.equal(body.store, false)
    assert.equal(result.status, 'success')
    assert.equal(result.prompt_id, 'composite')
    assert.equal(result.prompt_label, preset.label)
    assert.equal(result.prompt, body.input)
    assert.equal(result.reasoning_effort, 'low')
  }
})

test('历史任务使用保存的提示词快照，请求不能悄悄改用当前预设', async () => {
  const snapshot = 'Historical SVG benchmark snapshot: <svg> with one circle.'
  const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    prompt_id: 'composite',
    prompt: snapshot,
    fetchImpl,
  })
  assert.equal(JSON.parse(calls[0].init.body).input, snapshot)
  assert.equal(result.prompt, snapshot)
  assert.equal(result.prompt_id, 'composite')
  assert.equal(result.prompt_label, getPrompt('composite').label)
})

test('无效提示词在发起收费请求前拒绝', async () => {
  const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
  for (const input of [
    { prompt_id: 'missing' },
    { prompt_id: 'composite', prompt: '' },
    { prompt_id: 'composite', prompt: ' '.repeat(10) },
    { prompt_id: 'composite', prompt: 'x'.repeat(32001) },
  ]) {
    await assert.rejects(probe({
      url: SOURCE_URL,
      key: KEY,
      model: MODELS[0],
      ...input,
      fetchImpl,
    }), /提示词/)
  }
  assert.equal(calls.length, 0)
})

test('复合任务失败记录仍保留实际提示词和模型，便于准确定位结果', async () => {
  const { calls, fetchImpl } = fetchRecorder(jsonResponse({
    error: { code: 'model_not_found', message: 'Requested model is unavailable' },
  }, { status: 404 }))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[1],
    prompt_id: 'composite',
    fetchImpl,
  })
  assert.equal(result.status, 'failed')
  assert.equal(result.prompt_id, 'composite')
  assert.equal(result.prompt_label, getPrompt('composite').label)
  assert.equal(result.prompt, JSON.parse(calls[0].init.body).input)
  assert.equal(result.model, MODELS[1])
  assert.equal(result.source_url, 'https://api.example.test/v1')
  assert.equal(result.http_status, 404)
})

test('probe accepts a completed Responses SSE stream', async () => {
  const { calls, fetchImpl } = fetchRecorder(
    fakeResponse(successSSE(MODELS[1]), { contentType: 'text/event-stream' })
  )
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[1],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.model, MODELS[1])
  assert.equal(calls.length, 1)
  assert.match(result.svg, /^<svg[\s\S]*<\/svg>$/)
})

test('probe accepts the nested output_text shape used by Responses JSON', async () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="5"/></svg>'
  const { fetchImpl } = fetchRecorder(jsonResponse({
    status: 'completed',
    output: [{ type: 'message', content: [{ type: 'output_text', text: svg }] }],
  }))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.svg, svg)
})

test('SSE wire overhead above the old 16 MB limit still succeeds without trimming the SVG', async () => {
  const oldLimit = 16 * 1024 * 1024
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>'
  const tail = [
    `\n\ndata: ${JSON.stringify({ type: 'response.output_text.delta', delta: svg })}`,
    `\n\ndata: ${JSON.stringify({ type: 'response.completed', response: { status: 'completed', model: MODELS[0], output_text: svg } })}`,
    '\n\ndata: [DONE]\n\n',
  ]
  const padding = encoder.encode('x'.repeat(1024 * 1024))
  const chunks = [encoder.encode(':'), ...Array.from({ length: 17 }, () => padding), ...tail]
  assert.ok(byteLength(chunks) > oldLimit)
  assert.ok(byteLength(chunks) < EXPECTED_RESPONSE_LIMIT)

  const source = streamedResponse(chunks)
  const { fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.svg, svg)
  assert.equal(result.response_text, svg)
  assert.equal(result.prompt, PROMPT)
  assert.equal(result.reasoning_effort, 'low')
})

test('SSE byte accounting uses UTF-8 bytes for streamed chunks, not JavaScript character count', async () => {
  const oldLimit = 16 * 1024 * 1024
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>'
  const unicodePadding = '界'.repeat(5_600_000)
  const body = `:${unicodePadding}\n\n${successSSE(MODELS[0])}`
  assert.ok(body.length < oldLimit)
  assert.ok(Buffer.byteLength(body, 'utf8') > oldLimit)
  assert.ok(Buffer.byteLength(body, 'utf8') < EXPECTED_RESPONSE_LIMIT)

  const source = streamedTextResponse(body)
  const { fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.svg, svg)
})

test('a response exactly at 64 MB is accepted because the limit is strictly greater-than', async () => {
  const limit = EXPECTED_RESPONSE_LIMIT
  const suffix = `:\n\n${successSSE(MODELS[0])}`
  const prefixBytes = limit - encoder.encode(suffix).byteLength
  const body = `${':'}${'x'.repeat(prefixBytes)}${suffix.slice(1)}`
  assert.equal(encoder.encode(body).byteLength, limit)

  const source = streamedTextResponse(body, { chunkSize: 1024 * 1024 })
  const { fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.response_bytes, limit)
  assert.equal(result.svg, '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>')
})

test('responses over 64 MB are cancelled and reported as a local response limit', async () => {
  const limit = EXPECTED_RESPONSE_LIMIT
  const multibyteChunk = encoder.encode('界'.repeat(349_525))
  assert.ok(multibyteChunk.byteLength > 0)
  const chunkCount = Math.floor(limit / multibyteChunk.byteLength) + 1
  // 保留超限后的额外块，避免底层流在读取器触发 cancel 前自行 close。
  const source = streamedRepeatedResponse(multibyteChunk, chunkCount + 4, { contentLength: 1 })
  const { calls, fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[1],
    fetchImpl,
  })

  assert.equal(calls.length, 1)
  assert.equal(source.response.headers.get('content-length'), '1')
  assert.equal(result.status, 'failed')
  assert.equal(result.failure_kind, 'local_response_limit')
  assert.equal(result.response_limit_bytes, limit)
  assert.equal(result.response_bytes, multibyteChunk.byteLength * chunkCount)
  assert.equal(result.http_status, 200)
  assert.equal(result.svg, '')
  assert.equal(source.cancelled, true)
  assert.ok(source.chunksRead >= chunkCount)
  assert.ok(source.chunksRead <= chunkCount + 4)
})

test('上游流读取异常单独归类为 upstream_stream_read_error', async () => {
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('data: {"type":"response.output_text.delta","delta":"<svg>" }\\n\\n'))
      controller.error(new Error('unexpected EOF'))
    },
  })
  const response = new Response(stream, {
    status: 200,
    headers: { 'content-type': 'text/event-stream' },
  })
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl: async () => response,
  })
  assert.equal(result.status, 'failed')
  assert.equal(result.failure_kind, 'upstream_error')
  assert.equal(result.upstream_code, 'upstream_stream_read_error')
  assert.match(result.upstream_message, /unexpected EOF/)
  assert.equal(result.http_status, 200)
})

test('本地等待响应头超时保留总时限来源且不伪造上游错误', async () => {
  let calls = 0
  let requestSignal
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 35, idleTimeoutMs: 150,
    fetchImpl: async (_url, init) => {
      calls++
      requestSignal = init.signal
      return new Promise(() => {})
    },
  })
  assert.equal(calls, 1)
  assert.equal(requestSignal.aborted, true)
  assert.equal(result.status, 'failed')
  assert.equal(result.failure_kind, 'timeout')
  assert.equal(result.timeout_kind, 'total')
  assert.equal(result.timeout_ms, 35)
  assert.equal(result.http_status, 0)
  assert.equal(result.response_bytes, 0)
  assert.equal(result.upstream_code, '')
  assert.equal(result.upstream_message, '')
  assert.match(result.error, /本地测试总时限/)
})

test('响应头之前的连续无数据也受空闲时限约束', async () => {
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 200, idleTimeoutMs: 35,
    fetchImpl: async () => new Promise(() => {}),
  })
  assert.equal(result.failure_kind, 'timeout')
  assert.equal(result.timeout_kind, 'idle')
  assert.equal(result.idle_timeout_ms, 35)
  assert.equal(result.http_status, 0)
  assert.equal(result.upstream_code, '')
  assert.match(result.error, /本地等待上游数据/)
})

test('收到 HTTP 200 后本地超时保留已读字节，取消握手不阻塞返回', async () => {
  const chunk = 'data: {"type":"response.output_text.delta","delta":"<svg>"}\n\n'
  const source = openStreamResponse([chunk], { neverFinishCancel: true })
  const { calls, fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 40, idleTimeoutMs: 150, fetchImpl,
  })
  assert.equal(calls.length, 1)
  assert.equal(result.failure_kind, 'timeout')
  assert.equal(result.timeout_kind, 'total')
  assert.equal(result.http_status, 200)
  assert.equal(result.response_bytes, Buffer.byteLength(chunk))
  assert.equal(result.upstream_code, '')
  assert.equal(result.upstream_message, '')
  assert.equal(source.cancelled, true)
})

test('连续收到实际字节会刷新空闲时限，长生成过程仍可成功', async () => {
  let timer, ticks = 0, cancelled = false
  const body = new ReadableStream({
    start(controller) {
      timer = setInterval(() => {
        ticks++
        controller.enqueue(encoder.encode(ticks === 7 ? successSSE() : ': heartbeat\n\n'))
      }, 20)
    },
    cancel() { cancelled = true; clearInterval(timer) },
  })
  try {
    const { calls, fetchImpl } = fetchRecorder(new Response(body, { headers: { 'content-type': 'text/event-stream' } }))
    const result = await probe({
      url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 1000, idleTimeoutMs: 100, fetchImpl,
    })
    assert.equal(result.status, 'success')
    assert.ok(result.duration_ms >= 100)
    assert.equal(calls.length, 1)
    assert.equal(cancelled, true)
  } finally { clearInterval(timer) }
})

test('持续心跳只刷新空闲期限，不能无限延长本地总时限', async () => {
  let timer
  const body = new ReadableStream({
    start(controller) { timer = setInterval(() => controller.enqueue(encoder.encode(': heartbeat\n\n')), 10) },
    cancel() { clearInterval(timer) },
  })
  try {
    const result = await probe({
      url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 90, idleTimeoutMs: 60,
      fetchImpl: async () => new Response(body, { headers: { 'content-type': 'text/event-stream' } }),
    })
    assert.equal(result.failure_kind, 'timeout')
    assert.equal(result.timeout_kind, 'total')
    assert.equal(result.http_status, 200)
    assert.ok(result.response_bytes > 0)
    assert.equal(result.upstream_code, '')
  } finally { clearInterval(timer) }
})

test('空数据块不刷新空闲期限', async () => {
  let timer
  const body = new ReadableStream({
    start(controller) { timer = setInterval(() => controller.enqueue(new Uint8Array()), 10) },
    cancel() { clearInterval(timer) },
  })
  try {
    const result = await probe({
      url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 250, idleTimeoutMs: 40,
      fetchImpl: async () => new Response(body, { headers: { 'content-type': 'text/event-stream' } }),
    })
    assert.equal(result.failure_kind, 'timeout')
    assert.equal(result.timeout_kind, 'idle')
    assert.equal(result.response_bytes, 0)
  } finally { clearInterval(timer) }
})

test('上游明确发送的超时错误仍属于上游，不通过错误文案猜测本地超时', async () => {
  const source = openStreamResponse([`data: ${JSON.stringify({
    type: 'error', error: { code: 'stream_read_error', message: 'The operation was aborted due to timeout' },
  })}\n\n`])
  const { calls, fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 500, idleTimeoutMs: 100, errorGraceMs: 10, fetchImpl,
  })
  assert.equal(result.failure_kind, 'upstream_error')
  assert.equal(result.timeout_kind, '')
  assert.equal(result.upstream_code, 'stream_read_error')
  assert.match(result.upstream_message, /aborted due to timeout/)
  assert.equal(result.http_status, 200)
  assert.equal(calls.length, 1)
  assert.equal(source.cancelled, true)
})

test('底层读取器自行抛出 TimeoutError 时不冒认本地计时器', async () => {
  const body = new ReadableStream({
    start(controller) { controller.error(new DOMException('upstream socket timeout', 'TimeoutError')) },
  })
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0],
    fetchImpl: async () => new Response(body),
  })
  assert.equal(result.failure_kind, 'upstream_error')
  assert.equal(result.timeout_kind, '')
  assert.equal(result.upstream_code, 'upstream_stream_read_error')
  assert.match(result.upstream_message, /upstream socket timeout/)
})

test('收到完整完成事件即返回，无须等待 EOF，最终文本不会重复叠加', async () => {
  const source = openStreamResponse([successSSE()], { neverFinishCancel: true })
  const { calls, fetchImpl } = fetchRecorder(source.response)
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 500, idleTimeoutMs: 100, fetchImpl,
  })
  assert.equal(result.status, 'success')
  assert.equal(result.response_text, successJSON().output_text)
  assert.equal(result.svg, successJSON().output_text)
  assert.equal(calls.length, 1)
  assert.equal(source.cancelled, true)
})

test('完成事件支持跨字节分块的 UTF-8、CRLF 和多行 data 字段', async () => {
  const svg = '<svg><text>界</text></svg>'
  const event = [
    'event: response.completed',
    'data: {"type":"response.completed",',
    `data: "response":{"status":"completed","output_text":${JSON.stringify(svg)}}}`,
    '', '',
  ].join('\r\n')
  const chunks = Array.from(encoder.encode(event), byte => Uint8Array.of(byte))
  const source = openStreamResponse(chunks)
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 500, idleTimeoutMs: 100,
    fetchImpl: async () => source.response,
  })
  assert.equal(result.status, 'success')
  assert.equal(result.svg, svg)
  // CR 自身即为合法行结束符；终止空行后的可选 LF 无须再读取。
  assert.equal(result.response_bytes, Buffer.byteLength(event) - 1)
  assert.equal(source.cancelled, true)
})

test('完整 SVG 和 DONE 均不能替代 response.completed 事件', async () => {
  const source = openStreamResponse([
    `data: ${JSON.stringify({ type: 'response.output_text.delta', delta: '<svg></svg>' })}\n\n`,
    'data: [DONE]\n\n',
  ])
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 200, idleTimeoutMs: 35,
    fetchImpl: async () => source.response,
  })
  assert.equal(result.status, 'failed')
  assert.equal(result.failure_kind, 'timeout')
  assert.equal(result.timeout_kind, 'idle')
  assert.equal(result.svg, '')
})

test('完成事件未收到空行分隔符时不能提前结束未关闭的流', async () => {
  const source = openStreamResponse([
    `data: ${JSON.stringify({ type: 'response.completed', response: successJSON() })}\n`,
  ])
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 200, idleTimeoutMs: 35,
    fetchImpl: async () => source.response,
  })
  assert.equal(result.status, 'failed')
  assert.equal(result.timeout_kind, 'idle')
  assert.equal(result.svg, '')
})

test('HTTP 错误体按字节截取并及时取消，不等超大正文传输结束', async () => {
  const source = openStreamResponse(['upstream failure '.repeat(3000)], { neverFinishCancel: true })
  source.response.status = 503
  source.response.ok = false
  const result = await probe({
    url: SOURCE_URL, key: KEY, model: MODELS[0], timeoutMs: 500, idleTimeoutMs: 100,
    fetchImpl: async () => source.response,
  })
  assert.equal(result.failure_kind, 'upstream_http_error')
  assert.equal(result.http_status, 503)
  assert.match(result.upstream_message, /^upstream failure/)
  assert.ok(result.upstream_body.length <= 1200)
  assert.equal(source.cancelled, true)
})

test('真实 HTTP 长连接收到响应头后由本地空闲计时器取消，不误报上游断流', async () => {
  let requests = 0
  const body = ': accepted\n\n'
  const server = createServer((_request, response) => {
    requests++
    response.writeHead(200, { 'content-type': 'text/event-stream' })
    response.write(body)
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  try {
    const result = await probe({
      url: `http://127.0.0.1:${server.address().port}`,
      key: KEY, model: MODELS[0], timeoutMs: 500, idleTimeoutMs: 100,
    })
    assert.equal(requests, 1)
    assert.equal(result.failure_kind, 'timeout')
    assert.equal(result.timeout_kind, 'idle')
    assert.equal(result.http_status, 200)
    assert.equal(result.response_bytes, Buffer.byteLength(body))
    assert.equal(result.upstream_code, '')
    assert.equal(result.upstream_message, '')
  } finally {
    server.closeAllConnections()
    await new Promise(resolve => server.close(resolve))
  }
})

test('a complete JSON Responses document larger than 2 MB still succeeds', async () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10H0z"/></svg>'
  const largeText = `${'p'.repeat(2 * 1024 * 1024)}${svg}`
  assert.ok(Buffer.byteLength(JSON.stringify({ status: 'completed', output_text: largeText }), 'utf8') > 2 * 1024 * 1024)

  const { fetchImpl } = fetchRecorder(jsonResponse({
    status: 'completed',
    model: MODELS[0],
    output_text: largeText,
  }))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'success')
  assert.equal(result.svg, svg)
  assert.equal(result.response_text, largeText)
})

test('probe rejects HTTP failures and never retries or falls back', async () => {
  const { calls, fetchImpl } = fetchRecorder(
    fakeResponse('upstream failure', { status: 503, contentType: 'text/plain' })
  )
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'failed')
  assert.equal(result.svg, '')
  assert.equal(calls.length, 1)
  assert.equal(result.request_url, REQUEST_URL)
  assert.doesNotMatch(result.error, new RegExp(KEY))
})

test('probe redacts a key that appears in a transport error', async () => {
  const fetchImpl = async () => {
    throw new Error(`transport rejected Bearer ${KEY}`)
  }
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  assert.equal(result.status, 'failed')
  assert.doesNotMatch(result.error, new RegExp(KEY))
  assert.match(result.error, /transport rejected/)
})

test('probe treats 200 error and incomplete events as failures', async () => {
  const cases = [
    [
      'error event',
      `data: ${JSON.stringify({ type: 'response.failed', error: { message: 'nope' } })}\n\n`,
    ],
    [
      'incomplete event',
      `data: ${JSON.stringify({ type: 'response.incomplete', response: { status: 'incomplete' } })}\n\n`,
    ],
    [
      'completed event with failed response status',
      `data: ${JSON.stringify({ type: 'response.completed', response: { status: 'failed', output_text: '<svg></svg>' } })}\n\n`,
    ],
    [
      'delta without completed event',
      `data: ${JSON.stringify({ type: 'response.output_text.delta', delta: '<svg></svg>' })}\n\n`,
    ],
  ]

  for (const [name, body] of cases) {
    await test(name, async () => {
      const { calls, fetchImpl } = fetchRecorder(
        fakeResponse(body, { contentType: 'text/event-stream' })
      )
      const result = await probe({
        url: SOURCE_URL,
        key: KEY,
        model: MODELS[0],
        fetchImpl,
      })
      assert.equal(result.status, 'failed')
      assert.equal(result.svg, '')
      assert.equal(calls.length, 1)
    })
  }
})

test('probe rejects incomplete JSON responses and malformed SVG', async () => {
  const incomplete = fetchRecorder(jsonResponse({
    status: 'incomplete',
    output_text: '<svg></svg>',
  }))
  const incompleteResult = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl: incomplete.fetchImpl,
  })
  assert.equal(incompleteResult.status, 'failed')
  assert.equal(incompleteResult.svg, '')

  const malformed = fetchRecorder(jsonResponse({
    status: 'completed',
    output_text: '<svg><rect></svg>',
  }))
  const malformedResult = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl: malformed.fetchImpl,
  })
  assert.equal(malformedResult.status, 'failed')
  assert.equal(malformedResult.svg, '')
})

test('probe 提取第一个完整 SVG 根并且不泄露回显密钥', async () => {
  const multipleRoots = fetchRecorder(jsonResponse({
    status: 'completed',
    output_text: '<svg></svg><svg></svg>',
  }))
  const rootsResult = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl: multipleRoots.fetchImpl,
  })
  assert.equal(rootsResult.status, 'success')
  assert.equal(rootsResult.svg, '<svg></svg>')

  const echoed = fetchRecorder(jsonResponse({
    status: 'completed',
    output_text: `<svg><text>${KEY}</text></svg>`,
  }))
  const echoedResult = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl: echoed.fetchImpl,
  })
  assert.equal(echoedResult.status, 'success')
  assert.doesNotMatch(echoedResult.response_text, new RegExp(KEY))
  assert.doesNotMatch(echoedResult.svg, new RegExp(KEY))
})

test('probe keeps source and request URLs free of credentials', async () => {
  const { calls, fetchImpl } = fetchRecorder(jsonResponse(successJSON()))
  const result = await probe({
    url: SOURCE_URL,
    key: KEY,
    model: MODELS[0],
    fetchImpl,
  })

  for (const value of [result.source_url, result.request_url, calls[0].url]) {
    assert.doesNotMatch(value, new RegExp(KEY))
  }
})

test('复合 SVG 大响应的本地上限为 64 MB', () => {
  assert.equal(MAX_RESPONSE_BYTES, 64 * 1024 * 1024)
})

test('SVG 提取支持 Markdown 围栏和合法嵌套 svg，而不把内部 svg 当成第二个根', () => {
  const source = [
    '下面是动画 SVG：',
    '```svg',
    '<svg xmlns="http://www.w3.org/2000/svg"><title>demo</title>',
    '<svg x="0" y="0" width="10" height="10"><rect width="10" height="10"/></svg>',
    '</svg>',
    '```',
  ].join('\n')
  const svg = extractSVG(source)
  assert.match(svg, /^<svg[\s\S]*<\/svg>$/)
  assert.equal((svg.match(/<svg(?:\s|>)/gi) || []).length, 2)
  assert.match(svg, /<rect width="10" height="10"\/>/)
})

test('SVG 提取支持大小写标签、CDATA 中的结束文本和自闭合根', () => {
  const mixed = '<SVG xmlns="http://www.w3.org/2000/svg"><text><![CDATA[文字 </svg> 不截断]]></text></SVG>'
  assert.equal(extractSVG(mixed), mixed)
  assert.equal(extractSVG('<svg xmlns="http://www.w3.org/2000/svg"/>'), '<svg xmlns="http://www.w3.org/2000/svg"/>')
})

test('SVG 提取忽略代码围栏外的说明和根节点后的文字，不拦截 SVG 内容', () => {
  const source = '说明文字\n```xml\n<svg onload="start()"><script>animate()</script></svg>\n```\n结束说明'
  assert.equal(extractSVG(source), '<svg onload="start()"><script>animate()</script></svg>')
})
