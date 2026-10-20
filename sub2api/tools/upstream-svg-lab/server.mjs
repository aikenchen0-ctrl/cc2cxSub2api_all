import http from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomBytes, timingSafeEqual } from 'node:crypto';
import { Store } from './lib/store.mjs';
import { AdminClient, accountSource } from './lib/admin.mjs';
import { Scheduler } from './lib/scheduler.mjs';
import { parseCredentials } from './lib/credentials.mjs';
import { PROMPT, MODELS, REASONING_EFFORTS, MAX_RESPONSE_BYTES, DEFAULT_TIMEOUT_MS, DEFAULT_IDLE_TIMEOUT_MS, DEFAULT_MAX_OUTPUT_TOKENS } from './lib/probe.mjs';
import { analyzeBillingCSV, MAX_CSV_BYTES } from './lib/billing-audit.mjs';
import { PromptCatalog } from './lib/prompt-catalog.mjs';

const directory = path.dirname(fileURLToPath(import.meta.url));
const staticFiles = { '/': ['index.html', 'text/html; charset=utf-8'], '/app.js': ['app.js', 'text/javascript; charset=utf-8'], '/style.css': ['style.css', 'text/css; charset=utf-8'] };
const json = (res, status, data) => { res.writeHead(status, { 'content-type': 'application/json; charset=utf-8' }); res.end(JSON.stringify(data)); };
async function body(req, limit = 40000) {
  const chunks = [];
  let length = 0;
  for await (const chunk of req) {
    length += chunk.length;
    if (length > limit) throw new Error('请求内容超过限制');
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}');
}

async function rawBody(req, limit = MAX_CSV_BYTES + 64 * 1024) {
  const chunks = [];
  let length = 0;
  for await (const chunk of req) {
    length += chunk.length;
    if (length > limit) throw new Error('上传内容超过限制');
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

function parseMultipart(buffer, contentType) {
  const match = String(contentType || '').match(/boundary=(?:"([^"]+)"|([^;]+))/i);
  if (!match) throw new Error('上传表单缺少 multipart 边界');
  const boundary = Buffer.from(`--${match[1] || match[2]}`);
  const parts = [];
  let cursor = 0;
  while (cursor < buffer.length) {
    const start = buffer.indexOf(boundary, cursor);
    if (start < 0) break;
    const bodyStart = start + boundary.length;
    if (buffer.subarray(bodyStart, bodyStart + 2).equals(Buffer.from('--'))) break;
    const headerStart = buffer.indexOf(Buffer.from('\r\n'), bodyStart);
    if (headerStart < 0) break;
    const contentStart = headerStart + 2;
    const separator = Buffer.from('\r\n\r\n');
    const dataStart = buffer.indexOf(separator, contentStart);
    if (dataStart < 0) break;
    const headers = buffer.subarray(contentStart, dataStart).toString('utf8').split(/\r\n/);
    const nextBoundary = buffer.indexOf(Buffer.from(`\r\n${boundary.toString('utf8')}`), dataStart + separator.length);
    if (nextBoundary < 0) break;
    const value = buffer.subarray(dataStart + separator.length, nextBoundary);
    const headerMap = {};
    for (const line of headers) {
      const index = line.indexOf(':');
      if (index > 0) headerMap[line.slice(0, index).trim().toLowerCase()] = line.slice(index + 1).trim();
    }
    const disposition = headerMap['content-disposition'] || '';
    const name = disposition.match(/(?:^|;\s*)name="([^"]*)"/i)?.[1] || '';
    const filename = disposition.match(/(?:^|;\s*)filename="([^"]*)"/i)?.[1] || '';
    parts.push({ name, filename, type: headerMap['content-type'] || '', value });
    cursor = nextBoundary + 2;
  }
  return parts;
}

export async function createLab({ dataDir, admin, timeoutMs = DEFAULT_TIMEOUT_MS, idleTimeoutMs = DEFAULT_IDLE_TIMEOUT_MS, maxOutputTokens = DEFAULT_MAX_OUTPUT_TOKENS, reasoningEffort = 'low', startScheduler = true, publicOrigin = '', basePath = '', accessToken = '', maxParallelJobs = 6 }) {
  const store = await new Store(dataDir).init();
  if (store.state.connection) {
    admin.baseURL = store.state.connection.base_url;
    admin.key = store.open(store.state.connection.secret);
  }
  const scheduler = new Scheduler({ store, admin, timeoutMs, idleTimeoutMs, maxOutputTokens, reasoningEffort, maxParallelJobs });
  const promptCatalog = new PromptCatalog(store);
  const csrf = randomBytes(24).toString('hex');
  const prefix = basePath.replace(/\/$/, '');
  const publicHost = publicOrigin ? new URL(publicOrigin).host : '';
  if (publicOrigin && accessToken.length < 3) throw new Error('公开部署管理口令至少需要 3 个字符');
  function authorized(req) {
    if (!accessToken) return true;
    const supplied = Buffer.from(String(req.headers['x-lab-access'] || ''));
    const expected = Buffer.from(accessToken);
    return supplied.length === expected.length && timingSafeEqual(supplied, expected);
  }
  const server = http.createServer(async (req, res) => {
    res.setHeader('x-content-type-options', 'nosniff');
    res.setHeader('cache-control', 'no-store');
    res.setHeader('referrer-policy', 'no-referrer');
    res.setHeader('content-security-policy', "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'");
    const authority = req.headers.host || '';
    const host = authority.split(':')[0];
    if (!['127.0.0.1', 'localhost'].includes(host) && authority !== publicHost) return json(res, 403, { error: '访问域名不匹配' });
    const expectedOrigin = publicOrigin && authority === publicHost ? publicOrigin : `http://${authority}`;
    if (req.headers.origin && req.headers.origin !== expectedOrigin) return json(res, 403, { error: '跨站请求已拒绝' });
    try {
      const url = new URL(req.url, `http://${authority}`);
      if (prefix && url.pathname === prefix) { res.writeHead(302, { location: `${prefix}/` }); return res.end(); }
      if (prefix) {
        if (!url.pathname.startsWith(`${prefix}/`)) return json(res, 404, { error: '接口不存在' });
        url.pathname = url.pathname.slice(prefix.length);
      }
      const authenticated = authorized(req);
      const publicRoute = req.method === 'GET' && (staticFiles[url.pathname] || url.pathname === '/api/config' || url.pathname === '/api/results' || /^\/api\/results\/[a-f\d-]+\/artifact$/.test(url.pathname));
      if (!publicRoute && !authenticated) return json(res, 401, { error: '请输入正确的管理口令' });
      if (!['GET', 'HEAD'].includes(req.method)) {
        const supplied = Buffer.from(String(req.headers['x-lab-token'] || ''));
        const expected = Buffer.from(csrf);
        if (supplied.length !== expected.length || !timingSafeEqual(supplied, expected)) {
          return json(res, 403, { code: 'CSRF_EXPIRED', error: '页面会话已过期，请刷新后重试' });
        }
      }
      if (req.method === 'GET' && staticFiles[url.pathname]) {
        const [file, type] = staticFiles[url.pathname];
        res.setHeader('content-type', type);
        return res.end(await fs.readFile(path.join(directory, 'public', file)));
      }
      if (req.method === 'GET' && url.pathname === '/api/config') return json(res, 200, {
        prompt: PROMPT, models: MODELS, reasoning_effort: scheduler.reasoningEffort, reasoning_efforts: REASONING_EFFORTS,
        prompts: promptCatalog.list(), prompt_id: 'ak47', max_parallel_jobs: scheduler.maxParallelJobs,
        model_execution: 'parallel',
        response_limit_bytes: MAX_RESPONSE_BYTES,
        timeout_ms: timeoutMs, idle_timeout_ms: idleTimeoutMs, max_output_tokens: scheduler.maxOutputTokens,
        management_required: !!accessToken, authenticated,
        ...(authenticated ? { csrf_token: csrf } : {})
      });
      if (req.method === 'GET' && url.pathname === '/api/settings') return json(res, 200, {
        max_parallel_jobs: scheduler.maxParallelJobs,
        active_count: scheduler.activeCount,
        active_model_count: scheduler.activeModelCount,
        queue_count: scheduler.queueCount
      });
      if (req.method === 'POST' && url.pathname === '/api/prompts') {
        const created = await promptCatalog.create(await body(req, 256000));
        return json(res, 201, created);
      }
      const promptMatch = url.pathname.match(/^\/api\/prompts\/([^/]+)$/);
      if (promptMatch && req.method === 'PATCH') {
        const updated = await promptCatalog.update(decodeURIComponent(promptMatch[1]), await body(req, 256000));
        return json(res, 200, updated);
      }
      if ((req.method === 'POST' || req.method === 'PATCH') && url.pathname === '/api/settings') {
        const input = await body(req);
        const value = Number(input.max_parallel_jobs);
        if (!Number.isInteger(value) || value < 1 || value > 16) throw new Error('并行任务数须为 1 至 16 的整数');
        await store.change(state => {
          state.settings = { ...(state.settings || {}), max_parallel_jobs: value };
        });
        scheduler.setMaxParallelJobs(value);
        return json(res, 200, {
          max_parallel_jobs: scheduler.maxParallelJobs,
          active_count: scheduler.activeCount,
          active_model_count: scheduler.activeModelCount,
          queue_count: scheduler.queueCount
        });
      }
      if (req.method === 'GET' && url.pathname === '/api/connection') return json(res, 200, {
        base_url: admin.baseURL || '', key_configured: !!admin.key
      });
      if (req.method === 'POST' && url.pathname === '/api/connection') {
        if (scheduler.active) return json(res, 409, { error: '请等待当前测试完成后修改连接' });
        const input = await body(req);
        if (typeof input.admin_key !== 'string' || input.admin_key.length < 8 || input.admin_key.length > 4096) throw new Error('管理员密钥无效');
        const candidate = new AdminClient({ baseURL: input.base_url, key: input.admin_key.trim() });
        await candidate.get('/accounts?page=1&page_size=1&lite=true');
        await store.change(state => { state.connection = { base_url: candidate.baseURL, secret: store.seal(candidate.key) }; });
        admin.baseURL = candidate.baseURL;
        admin.key = candidate.key;
        return json(res, 200, { base_url: admin.baseURL, key_configured: true });
      }
      if (req.method === 'GET' && url.pathname === '/api/accounts') {
        const accounts = await admin.list();
        return json(res, 200, { items: accounts.map(account => {
          const source = accountSource(account);
          return { id: account.id, name: account.name, source_url: source.source_url || '', eligible: source.eligible, reason: source.reason };
        }) });
      }
      if (req.method === 'GET' && url.pathname === '/api/jobs') return json(res, 200, {
        items: store.state.jobs.map(job => ({ ...store.publicJob(job), queued: scheduler.isQueued(job.id) })),
        active_count: scheduler.activeCount, active_model_count: scheduler.activeModelCount,
        max_parallel_jobs: scheduler.maxParallelJobs, queue_count: scheduler.queueCount
      });
      if (req.method === 'GET' && url.pathname === '/api/sources') return json(res, 200, {
        // 来源索引只提供可重新获取所需的任务或账号引用，不返回任何明文密钥。
        items: scheduler.sourceList()
      });
      if (req.method === 'GET' && url.pathname === '/api/billing-audits') return json(res, 200, {
        items: store.state.billing_audits || []
      });
      const billingAudit = url.pathname.match(/^\/api\/billing-audits\/([a-f\d-]+)$/);
      if (req.method === 'GET' && billingAudit) {
        const item = (store.state.billing_audits || []).find(audit => audit.id === billingAudit[1]);
        if (!item) return json(res, 404, { error: '计费审计结果不存在' });
        return json(res, 200, item);
      }
      if (req.method === 'POST' && url.pathname === '/api/billing-audits') {
        const type = String(req.headers['content-type'] || '');
        let csv;
        let filename = 'usage.csv';
        if (/multipart\/form-data/i.test(type)) {
          const parts = parseMultipart(await rawBody(req), type);
          const file = parts.find(part => part.name === 'file' || part.name === 'csv') || parts.find(part => part.filename);
          if (!file) throw new Error('上传表单中没有 CSV 文件');
          csv = file.value;
          filename = file.filename || filename;
        } else {
          const input = JSON.parse((await rawBody(req)).toString('utf8') || '{}');
          filename = input.filename || filename;
          if (typeof input.csv === 'string') csv = Buffer.from(input.csv, 'utf8');
          else if (typeof input.content === 'string') csv = Buffer.from(input.content, 'utf8');
          else if (typeof input.csv_base64 === 'string') csv = Buffer.from(input.csv_base64, 'base64');
          else throw new Error('请上传 CSV 文件或提供 csv 文本');
        }
        const audit = analyzeBillingCSV(csv, { filename });
        await store.saveBillingAudit(audit);
        return json(res, 201, audit);
      }
      if (req.method === 'GET' && url.pathname === '/api/results') return json(res, 200, { items: [...scheduler.getLiveResults(), ...store.state.results].map(result => {
        if (authenticated) return result;
        const { account_id, account_name, job_id, job_name, ...safe } = result;
        return safe;
      }) });
      if (req.method === 'POST' && url.pathname === '/api/parse') {
        const parsed = parseCredentials((await body(req)).text);
        return json(res, 200, { url: parsed.url, key_detected: true });
      }
      if (req.method === 'POST' && url.pathname === '/api/jobs/batch') {
        const input = await body(req);
        return json(res, 201, await scheduler.createBatch(input));
      }
      if (req.method === 'POST' && url.pathname === '/api/jobs') return json(res, 201, await scheduler.create(await body(req)));
      const jobMatch = url.pathname.match(/^\/api\/jobs\/([a-f\d-]+)(\/run)?$/);
      if (jobMatch) {
        if (req.method === 'PATCH' && !jobMatch[2]) return json(res, 200, await scheduler.update(jobMatch[1], await body(req)));
        if (req.method === 'POST' && jobMatch[2]) {
          if (!store.state.jobs.some(job => job.id === jobMatch[1])) return json(res, 404, { error: '计划不存在' });
          const status = scheduler.queueRun(jobMatch[1], true);
          return json(res, 202, { message: status.status === 'queued' ? '本轮测试已进入等待队列' : '本轮测试已开始', ...status });
        }
      }
      const rawResponse = url.pathname.match(/^\/api\/results\/([a-f\d-]+)\/response$/);
      if (req.method === 'GET' && rawResponse) {
        const result = store.state.results.find(item => item.id === rawResponse[1]);
        if (!result) return json(res, 404, { error: '结果不存在或仍在运行中' });
        const data = await store.artifact(result.id);
        const responseText = typeof data.response_text === 'string' ? data.response_text : '';
        const diagnosticFields = [
          'status', 'failure_kind', 'error', 'upstream_code', 'upstream_message', 'upstream_event_type',
          'http_status', 'response_bytes', 'timeout_ms', 'idle_timeout_ms', 'timeout_kind',
          'headers_ms', 'first_byte_ms', 'last_byte_ms', 'duration_ms', 'source_url', 'request_url',
          'model', 'returned_model', 'reasoning_effort', 'started_at', 'max_output_tokens', 'usage'
        ];
        const diagnostics = Object.fromEntries(diagnosticFields
          .filter(field => result[field] !== undefined).map(field => [field, result[field]]));
        // 历史失败保留原判定；查看时根据已保存正文补充可直接核实的说明。
        if (!data.svg && responseText.trim() && !/(?:<|&lt;)svg(?=\s|\/?>|&gt;)/i.test(responseText)) {
          diagnostics.analysis_hint = '这条响应只包含文字，没有 SVG 根标签；不是完整 SVG 被提取器截断。';
        } else if (!responseText.trim()) {
          diagnostics.analysis_hint = result.has_response_text === undefined
            ? '旧记录未保存正文或上游未输出正文，无法从这条历史记录还原错误事件；新测试将保留错误详情和已接收正文。'
            : '本次没有收到可见正文；请结合错误事件和读取字节数判断原因。';
        }
        return json(res, 200, {
          id: result.id, response_text: responseText, has_response_text: Boolean(responseText.trim()), diagnostics
        });
      }
      const artifact = url.pathname.match(/^\/api\/results\/([a-f\d-]+)\/artifact$/);
      if (req.method === 'GET' && artifact) {
        const data = await store.artifact(artifact[1]);
        if (!data.svg) return json(res, 404, { error: '此结果没有完整 SVG' });
        res.setHeader('content-type', 'image/svg+xml; charset=utf-8');
        res.setHeader('content-security-policy', "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; frame-ancestors 'self'");
        if (url.searchParams.has('download')) res.setHeader('content-disposition', `attachment; filename="result-${artifact[1]}.svg"`);
        return res.end(data.svg);
      }
      return json(res, 404, { error: '接口不存在' });
    } catch (error) {
      return json(res, error.status === 409 ? 409 : 400, { error: error.message || '请求失败', ...(error.code ? { code: error.code } : {}) });
    }
  });
  if (startScheduler) scheduler.start();
  server.on('close', () => scheduler.stop());
  return { server, scheduler, store };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const admin = new AdminClient({ baseURL: process.env.SUB2API_BASE_URL, key: process.env.SUB2API_ADMIN_API_KEY });
  const lab = await createLab({
    admin, dataDir: process.env.LAB_DATA_DIR || path.join(directory, 'data'),
    timeoutMs: Math.max(10, Math.min(1200, Number(process.env.LAB_TIMEOUT_SECONDS || DEFAULT_TIMEOUT_MS / 1000))) * 1000,
    idleTimeoutMs: Math.max(10, Math.min(1200, Number(process.env.LAB_IDLE_TIMEOUT_SECONDS || DEFAULT_IDLE_TIMEOUT_MS / 1000))) * 1000,
    maxOutputTokens: Number(process.env.LAB_MAX_OUTPUT_TOKENS || DEFAULT_MAX_OUTPUT_TOKENS),
    reasoningEffort: process.env.LAB_REASONING_EFFORT || 'low',
    publicOrigin: process.env.LAB_PUBLIC_ORIGIN || '',
    basePath: process.env.LAB_BASE_PATH || '',
    accessToken: process.env.LAB_ACCESS_TOKEN || '',
    maxParallelJobs: Number(process.env.LAB_MAX_PARALLEL_JOBS || 6)
  });
  const port = Number(process.env.LAB_PORT || 9876);
  lab.server.listen(port, '127.0.0.1', () => console.log(`上游 SVG 测试台已启动：http://127.0.0.1:${port}`));
  for (const signal of ['SIGTERM', 'SIGINT']) process.once(signal, () => {
    lab.scheduler.stop();
    lab.server.close();
    const timer = setInterval(() => { if (!lab.scheduler.active) { clearInterval(timer); process.exit(0); } }, 250);
  });
}
