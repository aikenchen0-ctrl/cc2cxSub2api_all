const trailing = /[)\]},，。；;]+$/u;
const secretNames = /^(?:api[-_]?key|key|token|access[-_]?token|secret|password|authorization)$/i;
// 聊天复制常携带零宽边界字符，仅清理行首行尾，不改写密钥或地址内部字符。
function trimClipboardBoundary(value) {
  return String(value || '').replace(/^[\s\u200B\u2060]+|[\s\u200B\u2060]+$/gu, '');
}

// 地址只接收明确的 HTTP 协议；拒绝含凭据的 URL，避免误把 SSH 口令用于测试。
export function cleanSourceURL(input) {
  const raw = trimClipboardBoundary(input).replace(trailing, '');
  if (!/^https?:\/\//i.test(raw)) throw new Error('请输入带 http:// 或 https:// 的接口地址');
  const url = new URL(raw);
  if (url.username || url.password) throw new Error('接口地址不能包含用户名或密码');
  for (const name of url.searchParams.keys()) {
    if (secretNames.test(name)) throw new Error('请将 URL 中的密钥移到独立的 Key 字段');
  }
  if (url.search || url.hash) throw new Error('接口地址不能包含查询参数或片段');
  return url.toString().replace(/\/+$/, '');
}

export function responsesEndpoint(input) {
  const url = new URL(cleanSourceURL(input));
  let path = url.pathname.replace(/\/+$/, '');
  if (/\/(?:chat\/completions|messages|models)$/i.test(path)) {
    path = path.replace(/\/(?:chat\/completions|messages|models)$/i, '/responses');
  } else if (!/\/responses$/i.test(path)) {
    path += /\/v1$/i.test(path) ? '/responses' : '/v1/responses';
  }
  url.pathname = path;
  return url.toString();
}

export function parseCredentials(text) {
  const original = String(text || '');
  if (original.length > 32_000) throw new Error('粘贴内容超过 32000 字符');
  const value = original.split(/\r?\n/).map(trimClipboardBoundary).join('\n');
  const urls = [...new Set((value.match(/https?:\/\/[^\s"'`<>，。；]+/giu) || [])
    .map(url => cleanSourceURL(url)))];
  if (urls.length !== 1) throw new Error(urls.length ? '检测到多个 URL，请每次粘贴一个上游' : '未检测到明确的 http(s) URL');
  // 同时支持 JSON、环境变量、curl Authorization，以及常见 sk- 密钥。
  const keys = new Set();
  for (const match of value.matchAll(/(?:["']?(?:api[_ -]?key|OPENAI_API_KEY|密钥|key|token)["']?\s*[:=：]\s*["'`]?|Bearer\s+)([A-Za-z0-9._~+\/=*\-]{8,})/gi)) {
    keys.add(match[1].replace(/[,;]+$/, ''));
  }
  for (const match of value.matchAll(/(?:^|[\s"'`=:])(sk-[A-Za-z0-9._~+\/=-]{8,})(?=$|[\s"'`,;<>])/gi)) keys.add(match[1]);
  if (keys.size !== 1) throw new Error(keys.size ? '检测到多个 Key，请每次粘贴一个上游' : '未检测到 API Key，请使用 api_key=... 或 Authorization: Bearer ...');
  const key = [...keys][0];
  if (/^(?:sk-)?(?:example|placeholder|redacted|your[-_]?key)/i.test(key) || key.includes('*')) {
    throw new Error('密钥是示例或已脱敏内容，请填写真实 API Key');
  }
  return { url: urls[0], key };
}
