import { cleanSourceURL } from './credentials.mjs';

export function accountSource(account) {
  if (!account || account.platform !== 'openai' || account.type !== 'apikey') return { eligible: false, reason: '仅支持 OpenAI API Key 上游，OAuth 等账号暂不测试' };
  if (account.status !== 'active') return { eligible: false, reason: '账号不是启用状态' };
  if (account.proxy_id) return { eligible: false, reason: '此账号依赖代理；独立服务尚未配置同等代理路径' };
  if (account.parent_account_id || account.credential_account_id || account.extra?.credential_account_id) {
    return { eligible: false, reason: '影子或共享凭据账号不能作为独立上游' };
  }
  try {
    const url = cleanSourceURL(account.credentials?.base_url);
    if (account.credentials_status?.has_api_key !== true) {
      return { eligible: false, source_url: url, reason: '账号未配置 API Key' };
    }
    return { eligible: true, url, source_url: url, name: account.name || '', reason: '' };
  } catch {
    return { eligible: false, reason: '账号没有明确且有效的上游 base_url' };
  }
}

export class AdminClient {
  constructor({ baseURL, key, fetchImpl = fetch }) {
    this.baseURL = baseURL ? cleanSourceURL(baseURL).replace(/\/api\/v1$/, '') : '';
    this.key = key;
    this.fetch = fetchImpl;
  }
  async get(path) {
    if (!this.baseURL || !this.key) throw new Error('请在服务环境变量配置 Sub2API 地址及管理员密钥');
    const response = await this.fetch(`${this.baseURL}/api/v1/admin${path}`, {
      redirect: 'error', headers: { 'x-api-key': this.key }, signal: AbortSignal.timeout(20_000)
    });
    if (!response.ok) throw new Error(`Sub2API 管理员 API 返回 HTTP ${response.status}`);
    const body = await response.json();
    if (body.code !== undefined && body.code !== 0) throw new Error('Sub2API 管理员 API 返回业务错误');
    return body.data ?? body;
  }
  async account(id) {
    if (!Number.isSafeInteger(id) || id < 1) throw new Error('账号编号无效');
    return this.get(`/accounts/${id}`);
  }
  async source(id) {
    const account = await this.account(id);
    const checked = accountSource(account);
    if (!checked.eligible) throw new Error(`账号 #${id}：${checked.reason}`);

    const exported = await this.get(`/accounts/data?ids=${encodeURIComponent(id)}&include_proxies=false`);
    const accounts = Array.isArray(exported?.accounts) ? exported.accounts : [];
    if (accounts.length !== 1) throw new Error(`账号 #${id} 导出结果数量不匹配`);
    const record = accounts[0];
    const exportedURL = cleanSourceURL(record.credentials?.base_url);
    const exportedKey = record.credentials?.api_key;
    if (
      record.name !== account.name ||
      record.platform !== account.platform ||
      record.type !== account.type ||
      exportedURL !== checked.source_url
    ) {
      throw new Error(`账号 #${id} 导出结果与账号详情不一致`);
    }
    if (record.proxy_key || !exportedKey || typeof exportedKey !== 'string' || exportedKey.includes('*')) {
      throw new Error(`账号 #${id} 不是可直接连接的独立 API Key 上游`);
    }
    return {
      name: account.name || '',
      url: checked.source_url,
      source_url: checked.source_url,
      key: exportedKey,
    };
  }
  async list() {
    const output = [];
    for (let page = 1; page <= 1000; page++) {
      const data = await this.get(`/accounts?page=${page}&page_size=100&platform=openai&lite=false`);
      const items = Array.isArray(data) ? data : data.items || [];
      output.push(...items);
      if (Array.isArray(data) || !items.length || (data.pages && page >= data.pages) || (Number.isFinite(data.total) && output.length >= data.total) || items.length < 100) return output;
    }
    throw new Error('账号分页超过安全上限，请缩小筛选范围');
  }
}
