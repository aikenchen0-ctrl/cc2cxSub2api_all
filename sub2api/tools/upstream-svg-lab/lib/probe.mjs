import { cleanSourceURL, responsesEndpoint } from './credentials.mjs';
import { AK47_PROMPT, getPrompt } from './prompts.mjs';

export const PROMPT = AK47_PROMPT;
export const MODELS = ['gpt-6.1-sol', 'gpt-6-astra'];
export const REASONING_EFFORTS = ['low', 'medium', 'high', 'max'];
// 复合插画可能包含大量 SVG 路径；上限只防止失控响应，不应截断正常作品。
export const MAX_RESPONSE_BYTES = 64 * 1024 * 1024;
export const DEFAULT_TIMEOUT_MS = 1_200_000;
export const DEFAULT_IDLE_TIMEOUT_MS = 280_000;
export const DEFAULT_ERROR_GRACE_MS = 1_000;
export const DEFAULT_MAX_OUTPUT_TOKENS = 65_536;
const MAX_ERROR_BYTES = 16 * 1024;

export function normalizeMaxOutputTokens(value = DEFAULT_MAX_OUTPUT_TOKENS) {
  const number = Number(value);
  if (!Number.isInteger(number) || number < 1024 || number > 1_000_000) {
    throw new Error('输出预算须为 1024 至 1000000 的整数');
  }
  return number;
}

function diagnosticError(message, details = {}) {
  const error = new Error(message);
  Object.assign(error, details);
  return error;
}

// 按 SVG 根元素标签边界提取内容；页面预览由独立 iframe 承载。
export function extractSVG(text) {
  let source = String(text).replace(/(?:```|~~~)(?:svg|xml)?\s*/gi, '').replace(/(?:```|~~~)/g, '');
  if (!/<svg(?=\s|\/?>)/i.test(source) && /&lt;svg(?=[\s/>])/i.test(source)) {
    source = source.replace(/&lt;/gi, '<').replace(/&gt;/gi, '>');
  }
  const start = source.search(/<svg(?=\s|\/?>)/i);
  if (start < 0) return '';
  const tokens = /<!--[\s\S]*?-->|<!\[CDATA\[[\s\S]*?\]\]>|<\?[\s\S]*?\?>|<\/?([A-Za-z_][\w:.-]*)(?:\s+(?:[^>"']|"[^"]*"|'[^']*')*)?\s*\/?>/gi;
  const stack = [];
  let end = -1;
  let cursor = start;
  for (const match of source.slice(start).matchAll(tokens)) {
    const token = match[0];
    const at = start + match.index;
    if (source.slice(cursor, at).includes('<')) return '';
    cursor = at + token.length;
    if (!match[1]) continue;
    const name = match[1].toLowerCase();
    if (token.startsWith('</')) {
      if (stack.pop() !== name) return '';
      if (!stack.length) {
        end = cursor;
        break;
      }
    } else if (!token.endsWith('/>')) {
      stack.push(name);
    } else if (!stack.length) {
      end = cursor;
      break;
    }
  }
  if (end < 0 || stack.length) return '';
  const svg = source.slice(start, end);
  return svg;
}

function createDeadline(timeoutMs, idleTimeoutMs) {
  const controller = new AbortController();
  const abort = kind => controller.abort(diagnosticError(
    kind === 'total' ? `本地测试总时限已达 ${timeoutMs / 1000} 秒` : `本地等待上游数据已连续 ${idleTimeoutMs / 1000} 秒`,
    { failure_kind: 'timeout', timeout_kind: kind }
  ));
  const totalTimer = setTimeout(() => abort('total'), timeoutMs);
  let idleTimer;
  // 等待响应头也计入无数据时间；只有实际收到字节才延长空闲期限。
  const touch = () => {
    clearTimeout(idleTimer);
    idleTimer = setTimeout(() => abort('idle'), idleTimeoutMs);
  };
  touch();
  return {
    signal: controller.signal,
    touch,
    dispose() {
      clearTimeout(totalTimer);
      clearTimeout(idleTimer);
    },
  };
}

function waitForAbort(promise, signal) {
  return new Promise((resolve, reject) => {
    const abort = () => reject(signal.reason);
    if (signal.aborted) {
      Promise.resolve(promise).catch(() => {});
      reject(signal.reason);
      return;
    }
    signal.addEventListener('abort', abort, { once: true });
    Promise.resolve(promise).then(
      value => { signal.removeEventListener('abort', abort); resolve(value); },
      error => { signal.removeEventListener('abort', abort); reject(error); }
    );
  });
}

function cancelReader(reader) {
  // 某些上游的取消握手不会结束；清理不能反过来阻塞结果返回。
  try { Promise.resolve(reader.cancel()).catch(() => {}); } catch {}
  try { reader.releaseLock(); } catch {}
}

// 流读取和正文解析共用字段边界，避免两处对 event/data 的解释不一致。
function createSSEParser(onFrame) {
  let prefix = '', fragments = [], ignored = false, hasCharacters = false, field = '';
  let eventData = [], eventType = '', skipLF = false, beginning = true;
  const append = piece => {
    if (!piece) return;
    hasCharacters = true;
    if (ignored) return;
    let offset = 0;
    while (!field && offset < piece.length) {
      prefix += piece[offset++];
      if (prefix === 'data:' || prefix === 'event:') field = prefix.slice(0, -1);
      else if (!'data:'.startsWith(prefix) && !'event:'.startsWith(prefix)) {
        // 巨大注释与无关字段不保留，内存只随实际 data 增长。
        ignored = true;
        return;
      }
    }
    if (field) fragments.push(piece.slice(offset));
  };
  const dispatch = () => {
    if (!eventData.length && !eventType) return false;
    const frame = { payload: eventData.join('\n'), eventType };
    eventData = [];
    eventType = '';
    return onFrame(frame) === true;
  };
  const finishLine = () => {
    let stop = false;
    if (hasCharacters) {
      const value = !ignored && field ? fragments.join('').replace(/^ /, '') : '';
      if (field === 'data') eventData.push(value);
      else if (field === 'event') eventType = value.trim();
    } else stop = dispatch();
    prefix = '';
    fragments = [];
    ignored = false;
    hasCharacters = false;
    field = '';
    return stop;
  };
  return {
    write(text) {
      if (!text) return false;
      let start = skipLF && text[0] === '\n' ? 1 : 0;
      if (beginning && text.charCodeAt(start) === 0xFEFF) start++;
      beginning = false;
      skipLF = false;
      const separators = /[\r\n]/g;
      separators.lastIndex = start;
      let match;
      while ((match = separators.exec(text))) {
        append(text.slice(start, match.index));
        if (finishLine()) return true;
        start = match.index + 1;
        if (match[0] === '\r') {
          if (text[start] === '\n') { start++; separators.lastIndex = start; }
          else if (start === text.length) skipLF = true;
        }
      }
      append(text.slice(start));
      return false;
    },
    end() {
      if (hasCharacters && finishLine()) return true;
      return dispatch();
    },
  };
}

function decodeSSEFrame({ payload, eventType }) {
  if (payload.trim() === '[DONE]') return null;
  if (!payload.trim() && eventType !== 'error' && !isFailedEvent(eventType)) return null;
  let event;
  try { event = JSON.parse(payload); } catch {
    if (eventType === 'error') event = { message: payload.trim() };
    else return { invalid: true, type: eventType };
  }
  if (typeof event === 'string' && eventType === 'error') event = { error: event };
  if (!event || typeof event !== 'object' || Array.isArray(event)) return { invalid: true, type: eventType };
  return { event, type: String(firstNonEmpty(event.type, eventType) || '') };
}

function isFailedEvent(type) {
  return ['response.failed', 'response.incomplete', 'response.cancelled', 'response.canceled'].includes(type);
}

function isCompletedEvent(type, event) {
  return (type === 'response.completed' || type === 'response.done') &&
    event.response && typeof event.response === 'object' && !Array.isArray(event.response);
}

function isErrorEvent(type, event) {
  const present = value => typeof value === 'string' ? Boolean(value.trim()) :
    value && typeof value === 'object' ? Object.keys(value).length > 0 : Boolean(value);
  return type === 'error' || present(event.error) || present(event.response?.error);
}

function isOutputProgress(type, event) {
  if (!type.startsWith('response.')) return false;
  if (type.endsWith('.delta')) return typeof event.delta === 'string' && event.delta.length > 0;
  if (type === 'response.output_text.done') return typeof event.text === 'string' && event.text.length > 0;
  if (type === 'response.content_part.done' || type === 'response.content_part.added') {
    return typeof event.part?.text === 'string' && event.part.text.length > 0;
  }
  if (type === 'response.output_item.done' || type === 'response.output_item.added') {
    return Boolean(event.item?.encrypted_content) || (Array.isArray(event.item?.content) &&
      event.item.content.some(part => typeof part?.text === 'string' && part.text.length > 0));
  }
  return false;
}

function createTerminalEventDetector() {
  const decoder = new TextDecoder();
  let terminal = false, errorSeen = false, resumed = false;
  const parser = createSSEParser(frame => {
    const decoded = decodeSSEFrame(frame);
    if (!decoded || decoded.invalid) return false;
    const { event, type } = decoded;
    if (isFailedEvent(type) || isCompletedEvent(type, event)) {
      terminal = true;
      return true;
    }
    if (isErrorEvent(type, event)) { errorSeen = true; resumed = false; }
    else if (isOutputProgress(type, event)) {
      // 错误后重新输出正文时继续读取；普通心跳不能延长错误宽限期。
      resumed = true;
      errorSeen = false;
    }
    return false;
  });
  return value => {
    errorSeen = false;
    resumed = false;
    parser.write(decoder.decode(value, { stream: true }));
    return { terminal, errorSeen, resumed };
  };
}

async function readBody(response, { signal, onBytes, maxBytes = MAX_RESPONSE_BYTES, truncate = false,
  detectCompletion = true, errorGraceMs = DEFAULT_ERROR_GRACE_MS }) {
  let length = 0;
  const chunks = [];
  if (!response.body) return { body: '', bytes: 0 };
  const reader = response.body.getReader();
  const terminalEvent = detectCompletion ? createTerminalEventDetector() : null;
  let errorDeadline = 0;
  const graceExpired = Symbol('error-grace-expired');
  try {
    for (;;) {
      let part;
      let graceTimer;
      try {
        if (errorDeadline && Date.now() >= errorDeadline) break;
        const reading = waitForAbort(reader.read(), signal);
        part = errorDeadline
          ? await Promise.race([reading, new Promise(resolve => {
            graceTimer = setTimeout(() => resolve(graceExpired), Math.max(0, errorDeadline - Date.now()));
          })])
          : await reading;
        if (part === graceExpired) break;
      } catch (error) {
        if (signal.aborted) throw diagnosticError(signal.reason.message, { ...signal.reason, response_bytes: length });
        throw diagnosticError('上游响应流读取中断', {
          failure_kind: 'upstream_error',
          upstream_code: 'upstream_stream_read_error',
          upstream_message: String(error?.message || 'stream read error').slice(0, 500),
          response_bytes: length
        });
      } finally {
        clearTimeout(graceTimer);
      }
      const { done, value } = part;
      if (done) break;
      if (!value.byteLength) continue;
      const previousLength = length;
      length += value.byteLength;
      onBytes(length);
      if (length > maxBytes) {
        if (truncate) {
          chunks.push(Buffer.from(value.subarray(0, maxBytes - previousLength)));
          break;
        }
        throw diagnosticError(`响应超过本地 ${MAX_RESPONSE_BYTES / 1024 / 1024} MB 上限`, {
          failure_kind: 'local_response_limit',
          response_limit_bytes: MAX_RESPONSE_BYTES,
          response_bytes: length
        });
      }
      chunks.push(Buffer.from(value));
      // 完整终止事件已明确结果，不再等代理关闭长连接；[DONE] 和 SVG 尾标签不作完成依据。
      const detected = terminalEvent?.(value);
      if (detected?.terminal) break;
      if (detected?.resumed) errorDeadline = 0;
      else if (detected?.errorSeen && !errorDeadline) errorDeadline = Date.now() + errorGraceMs;
    }
  } catch (error) {
    error.partial_body = Buffer.concat(chunks).toString('utf8');
    throw error;
  } finally {
    cancelReader(reader);
  }
  return { body: Buffer.concat(chunks).toString('utf8'), bytes: length };
}

function responseText(response) {
  if (typeof response?.output_text === 'string' && response.output_text.trim()) return response.output_text;
  return (Array.isArray(response?.output) ? response.output : []).flatMap(item =>
    (Array.isArray(item?.content) ? item.content : [])
      .filter(part => part?.type === 'output_text' && typeof part.text === 'string').map(part => part.text)
  ).join('');
}

function firstNonEmpty(...values) {
  return values.find(value => (typeof value === 'string' || typeof value === 'number') && String(value).trim() !== '');
}

function errorDetail(event) {
  const nested = event?.response?.error;
  const direct = event?.error;
  const incomplete = event?.response?.incomplete_details || event?.incomplete_details || {};
  // 各字段分别回退，空 error 对象不能遮住 response.error 内的有效原因。
  const code = firstNonEmpty(
    direct?.code, nested?.code, event?.code, event?.response?.code,
    direct?.type, nested?.type, incomplete.reason
  ) || '';
  const message = firstNonEmpty(
    typeof direct === 'string' ? direct : null,
    direct?.message, nested?.message,
    typeof nested === 'string' ? nested : null,
    event?.message, event?.response?.message,
    direct?.detail, nested?.detail, event?.detail,
    incomplete.message, incomplete.reason, code
  ) || '上游报告错误或输出未完成';
  return { code: String(code), message: String(message) };
}

function createTextAccumulator() {
  const aliases = new Map(), groups = new Set();
  let sequence = 0;
  const groupFor = event => {
    const index = Number.isInteger(event.output_index) && event.output_index >= 0 ? event.output_index : null;
    const id = firstNonEmpty(event.item_id, event.item?.id);
    const keys = [index === null ? null : `index:${index}`, id ? `id:${id}` : null].filter(Boolean);
    if (!keys.length) keys.push('index:0');
    let group = keys.map(key => aliases.get(key)).find(Boolean);
    const defaultGroup = aliases.get('index:0');
    if (!group && index === null && id && defaultGroup && !defaultGroup.hasID) group = defaultGroup;
    if (!group) {
      group = { index, order: sequence++, parts: new Map(), hasID: false };
      groups.add(group);
    }
    if (id) group.hasID = true;
    if (index !== null) group.index = index;
    for (const key of keys) {
      const previous = aliases.get(key);
      if (previous && previous !== group) {
        group.hasID ||= previous.hasID;
        for (const [partIndex, part] of previous.parts) {
          const current = group.parts.get(partIndex);
          if (!current || (!current.snapshot && part.snapshot)) group.parts.set(partIndex, part);
          else if (!current.snapshot && !part.snapshot && part.delta.join('').length > current.delta.join('').length) {
            group.parts.set(partIndex, part);
          }
        }
        for (const [alias, value] of aliases) if (value === previous) aliases.set(alias, group);
        groups.delete(previous);
      }
      aliases.set(key, group);
    }
    return group;
  };
  const partFor = (group, index) => {
    if (!group.parts.has(index)) group.parts.set(index, { delta: [], snapshot: null });
    return group.parts.get(index);
  };
  return {
    add(event, type) {
      if (!['response.output_text.delta', 'response.output_text.done', 'response.content_part.done',
        'response.output_item.added', 'response.output_item.done'].includes(type)) return;
      const group = groupFor(event);
      const contentIndex = Number.isInteger(event.content_index) && event.content_index >= 0 ? event.content_index : 0;
      if (type === 'response.output_item.done') {
        for (const [index, part] of (Array.isArray(event.item?.content) ? event.item.content : []).entries()) {
          if (part?.type === 'output_text' && typeof part.text === 'string' && part.text) {
            partFor(group, index).snapshot = part.text;
          }
        }
      } else if (type === 'response.output_text.delta') {
        const part = partFor(group, contentIndex);
        if (part.snapshot === null && typeof event.delta === 'string') part.delta.push(event.delta);
      } else if (type === 'response.output_text.done' || type === 'response.content_part.done') {
        const value = type === 'response.output_text.done' ? event.text :
          event.part?.type === 'output_text' ? event.part.text : null;
        if (typeof value === 'string' && value) partFor(group, contentIndex).snapshot = value;
      }
    },
    text() {
      return [...groups].sort((a, b) => (a.index ?? a.order) - (b.index ?? b.order))
        .flatMap(group => [...group.parts.entries()].sort(([a], [b]) => a - b)
          .map(([, part]) => part.snapshot ?? part.delta.join(''))).join('');
    },
  };
}

export function parseOutput(body, contentType = '') {
  if (contentType.toLowerCase().includes('text/event-stream') || /^(?:\uFEFF)?(?:event:|data:)/m.test(body)) {
    const text = createTextAccumulator();
    let completed = null, failure = null, pendingFailure = null, lastEventType = '', usage = null, returnedModel = '';
    let terminalText = '';
    const details = () => ({
      partial_text: terminalText || text.text(), usage, returned_model: returnedModel,
      upstream_event_type: lastEventType,
    });
    const failureFor = (event, type) => {
      const detail = errorDetail(event);
      return diagnosticError(detail.message, {
        failure_kind: type === 'response.incomplete' || event.response?.status === 'incomplete' ? 'upstream_incomplete' : 'upstream_error',
        upstream_code: detail.code, upstream_message: detail.message, upstream_event_type: type || 'error',
      });
    };
    const parser = createSSEParser(frame => {
      const decoded = decodeSSEFrame(frame);
      if (!decoded) return false;
      if (decoded.invalid) throw diagnosticError('上游返回了无法解析的流事件', {
        failure_kind: 'upstream_invalid_sse', upstream_event_type: decoded.type,
      });
      const { event, type } = decoded;
      lastEventType = type || lastEventType;
      usage = event.response?.usage ?? event.usage ?? usage;
      returnedModel = firstNonEmpty(event.response?.model, event.model, returnedModel) || '';
      text.add(event, type);
      if (isFailedEvent(type) || (isCompletedEvent(type, event) &&
        (isErrorEvent(type, event) || (event.response.status && event.response.status !== 'completed')))) {
        terminalText = responseText(event.response);
        failure = failureFor(event, type);
        return true;
      }
      if (isCompletedEvent(type, event)) {
        completed = event.response;
        return true;
      }
      if (isErrorEvent(type, event)) pendingFailure = failureFor(event, type);
      return false;
    });
    try {
      if (!parser.write(body)) parser.end();
    } catch (error) {
      Object.assign(error, details(), { upstream_event_type: error.upstream_event_type || lastEventType });
      throw error;
    }
    if (failure) throw Object.assign(failure, details());
    if (completed) return {
      text: responseText(completed) || text.text(), usage, returned_model: returnedModel,
      upstream_event_type: lastEventType,
    };
    if (pendingFailure) throw Object.assign(pendingFailure, details(), { upstream_event_type: pendingFailure.upstream_event_type });
    throw diagnosticError('流响应缺少完成事件', { failure_kind: 'upstream_incomplete', ...details() });
  }
  let response;
  try { response = JSON.parse(body); } catch {
    throw diagnosticError('上游没有返回合法的 Responses JSON', { failure_kind: 'upstream_invalid_json' });
  }
  if (!response || typeof response !== 'object' || Array.isArray(response)) {
    throw diagnosticError('上游没有返回合法的 Responses 对象', { failure_kind: 'upstream_invalid_json' });
  }
  if (isErrorEvent(response.type, response) || (response.status && response.status !== 'completed')) {
    const detail = errorDetail(response);
    throw diagnosticError(detail.message || '上游报告错误或输出未完成', {
      failure_kind: response.status === 'incomplete' ? 'upstream_incomplete' : 'upstream_error',
      upstream_code: detail.code,
      upstream_message: detail.message,
      partial_text: responseText(response),
      usage: response.usage || null,
      returned_model: response.model || '',
      upstream_event_type: response.type || response.status || 'error',
    });
  }
  return { text: responseText(response), usage: response.usage || null, returned_model: response.model || '' };
}

export async function probe({ url, key, model, reasoning_effort = 'low', prompt_id = 'ak47', prompt_label, prompt_version = 1, prompt,
  timeoutMs = DEFAULT_TIMEOUT_MS, idleTimeoutMs = DEFAULT_IDLE_TIMEOUT_MS, maxOutputTokens = DEFAULT_MAX_OUTPUT_TOKENS,
  errorGraceMs = DEFAULT_ERROR_GRACE_MS, fetchImpl = fetch }) {
  const started = Date.now();
  const preset = /^custom-[a-f0-9-]{36}$/.test(prompt_id) && typeof prompt === 'string' && typeof prompt_label === 'string'
    ? { id: prompt_id, label: prompt_label, text: prompt }
    : getPrompt(prompt_id);
  const text = prompt === undefined ? preset.text : prompt;
  if (typeof text !== 'string' || !text.trim() || text.length > 32000) throw new Error('测试提示词为空或超过长度限制');
  if (!REASONING_EFFORTS.includes(reasoning_effort)) throw new Error('思考程度必须是 low、medium、high 或 max');
  maxOutputTokens = normalizeMaxOutputTokens(maxOutputTokens);
  const source_url = cleanSourceURL(url);
  const request_url = responsesEndpoint(source_url);
  timeoutMs = Number.isFinite(timeoutMs) && timeoutMs > 0 && timeoutMs <= 2_147_483_647 ? Math.ceil(timeoutMs) : DEFAULT_TIMEOUT_MS;
  idleTimeoutMs = Number.isFinite(idleTimeoutMs) && idleTimeoutMs > 0 && idleTimeoutMs <= 2_147_483_647 ? Math.ceil(idleTimeoutMs) : DEFAULT_IDLE_TIMEOUT_MS;
  errorGraceMs = Number.isFinite(errorGraceMs) && errorGraceMs > 0 && errorGraceMs <= 2_147_483_647
    ? Math.ceil(errorGraceMs) : DEFAULT_ERROR_GRACE_MS;
  const base = { source_url, request_url, model, reasoning_effort, prompt: text,
    prompt_id: preset.id, prompt_label: prompt_label || preset.label, prompt_version,
    response_limit_bytes: MAX_RESPONSE_BYTES, timeout_ms: timeoutMs, idle_timeout_ms: idleTimeoutMs,
    max_output_tokens: maxOutputTokens,
    started_at: new Date(started).toISOString() };
  const deadline = createDeadline(timeoutMs, idleTimeoutMs);
  const redact = value => key ? String(value ?? '').split(key).join('[密钥已隐藏]') : String(value ?? '');
  const readOptions = { signal: deadline.signal, errorGraceMs, onBytes: length => {
    base.response_bytes = length;
    base.first_byte_ms ??= Date.now() - started;
    base.last_byte_ms = Date.now() - started;
    deadline.touch();
  } };
  let response_text = '';
  let contentType = '';
  try {
    const response = await waitForAbort(fetchImpl(request_url, {
      method: 'POST', redirect: 'error', signal: deadline.signal,
      headers: { 'content-type': 'application/json', Authorization: `Bearer ${key}` },
      body: JSON.stringify({ model, input: text, reasoning: { effort: reasoning_effort }, stream: true, store: false, max_output_tokens: maxOutputTokens })
    }), deadline.signal);
    base.http_status = response.status;
    base.headers_ms = Date.now() - started;
    contentType = response.headers.get('content-type') || '';
    if (!response.ok) {
      let body = '';
      try {
        body = (await readBody(response, { ...readOptions, maxBytes: MAX_ERROR_BYTES, truncate: true, detectCompletion: false })).body;
      } catch (error) {
        if (deadline.signal.aborted) throw error;
      }
      let detail;
      try { detail = errorDetail(JSON.parse(body)); } catch {}
      const upstreamMessage = detail?.message || body.slice(0, 1200).trim();
      throw diagnosticError(`上游 HTTP ${response.status}${upstreamMessage ? `：${upstreamMessage}` : ''}`, {
        failure_kind: response.status === 404 ? 'model_or_endpoint_not_found' : response.status === 401 || response.status === 403 ? 'upstream_auth_error' : response.status === 429 ? 'upstream_rate_limited' : 'upstream_http_error',
        http_status: response.status,
        upstream_code: detail?.code || '',
        upstream_message: String(upstreamMessage || ''),
        upstream_body: body.slice(0, 1200),
      });
    }
    const payload = await readBody(response, readOptions);
    base.response_bytes = payload.bytes;
    const parsed = parseOutput(payload.body, contentType);
    // 模型可能回显输入秘密；原始输出保存前也执行精确脱敏。
    response_text = redact(parsed.text);
    parsed.returned_model = redact(parsed.returned_model);
    parsed.upstream_event_type = redact(parsed.upstream_event_type);
    base.usage = parsed.usage;
    base.returned_model = parsed.returned_model;
    base.upstream_event_type = parsed.upstream_event_type;
    const svg = extractSVG(response_text);
    if (!svg) {
      const hasRoot = /(?:<|&lt;)svg(?=\s|\/?>|&gt;)/i.test(response_text);
      throw diagnosticError(
        hasRoot ? '输出包含 SVG 开始标签，但无法提取完整文档' :
          response_text.trim() ? '上游只返回文字，没有生成 SVG' : '上游完成响应，但没有返回正文',
        {
          failure_kind: hasRoot ? 'local_svg_validation' : response_text.trim() ? 'upstream_no_svg' : 'upstream_empty_output',
          upstream_message: hasRoot ? 'SVG 标签可能不完整或未配对，可查看原始响应核对' :
            response_text.trim() ? '已保存模型原文；请查看原始响应了解未生成图形的原因' : '完成事件和文本片段中均未找到正文',
        }
      );
    }
    return { ...base, ...parsed, text: undefined, response_text, svg, status: 'success', duration_ms: Date.now() - started };
  } catch (error) {
    // 只以本次请求自身的取消原因认定本地超时，保留上游同名报错的真实来源。
    if (deadline.signal.aborted) error = diagnosticError(deadline.signal.reason.message, {
      ...error, ...deadline.signal.reason,
    });
    if (error.partial_body) {
      let partial;
      try {
        const parsed = parseOutput(error.partial_body, contentType);
        partial = { partial_text: parsed.text, usage: parsed.usage, returned_model: parsed.returned_model,
          upstream_event_type: parsed.upstream_event_type };
      } catch (parsedError) { partial = parsedError; }
      // 保留读取器的失败类别；局部正文只补充证据，不能把本地超时改写成上游失败。
      for (const name of ['partial_text', 'usage', 'returned_model', 'upstream_event_type', 'upstream_code', 'upstream_message']) {
        if (error[name] === undefined || error[name] === null || error[name] === '') error[name] = partial[name];
      }
    }
    const message = error?.message || '测试失败';
    if (error?.partial_text && !response_text) response_text = redact(error.partial_text);
    return {
      ...base, response_text, svg: '', status: 'failed',
      error: redact(message),
      failure_kind: error.failure_kind || 'local_probe_error',
      timeout_kind: error.timeout_kind || '',
      http_status: error.http_status || base.http_status || 0,
      response_bytes: error.response_bytes || base.response_bytes || 0,
      usage: error.usage ?? base.usage ?? null,
      returned_model: redact(error.returned_model ?? base.returned_model),
      upstream_event_type: redact(error.upstream_event_type || base.upstream_event_type).slice(0, 120),
      upstream_code: redact(error.upstream_code).slice(0, 120),
      upstream_message: redact(error.upstream_message).slice(0, 1200),
      upstream_body: redact(error.upstream_body).slice(0, 1200),
      duration_ms: Date.now() - started
    };
  } finally {
    deadline.dispose();
  }
}
