import { sub2apiBase } from './config.ts';

export interface Sub2ApiLoginResult {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email: string;
}

export interface Sub2ApiUserKey {
  id: string;
  name: string;
  key?: string;
}

interface JsonObject {
  [key: string]: unknown;
}

function asRecord(value: unknown): JsonObject | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : null;
}

function pickString(record: JsonObject | null, ...keys: string[]): string {
  if (!record) return '';
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

async function readJson(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return null;
  const type = response.headers.get('content-type') ?? '';
  if (!type.includes('json')) {
    if (response.status === 404) return null;
    throw new Error(`Sub2API returned non-JSON (HTTP ${response.status})`);
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    throw new Error(`Sub2API returned non-JSON (HTTP ${response.status})`);
  }
}

async function sub2apiFetch(path: string, init: RequestInit): Promise<{ status: number; body: unknown }> {
  const base = sub2apiBase();
  if (!base) throw new Error('SUB2API_BASE is not configured');
  const response = await fetch(`${base}${path}`, {
    ...init,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
  });
  return { status: response.status, body: await readJson(response) };
}

function unwrapData(body: unknown): JsonObject | null {
  const root = asRecord(body);
  if (!root) return null;
  return asRecord(root.data) ?? asRecord(root.result) ?? root;
}

export async function registerWithPassword(
  email: string,
  password: string,
  name?: string,
): Promise<Sub2ApiLoginResult> {
  const payload: Record<string, string> = { email, password };
  if (name) payload.name = name;
  const { status, body } = await sub2apiFetch('/api/v1/auth/register', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
  if (status === 409) throw new Error('该邮箱已注册');
  if (status === 400) throw new Error('注册信息不合法');
  if (status >= 400) throw new Error(`Sub2API 注册失败（HTTP ${status}）`);
  const data = unwrapData(body);
  const user = asRecord(data?.user) ?? asRecord(data?.account) ?? data;
  const accessToken = pickString(data, 'access_token', 'accessToken', 'token');
  const refreshToken = pickString(data, 'refresh_token', 'refreshToken');
  const userId = pickString(user, 'id', 'user_id', 'userId') || pickString(data, 'id', 'user_id', 'userId');
  const userEmail = pickString(user, 'email') || email;
  if (accessToken && userId) {
    return { accessToken, refreshToken, userId, email: userEmail };
  }
  return loginWithPassword(email, password);
}

export async function loginWithPassword(email: string, password: string): Promise<Sub2ApiLoginResult> {
  const { status, body } = await sub2apiFetch('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
  if (status === 401 || status === 403) throw new Error('邮箱或密码错误');
  if (status >= 400) throw new Error(`Sub2API 登录失败（HTTP ${status}）`);
  const data = unwrapData(body);
  const user = asRecord(data?.user) ?? asRecord(data?.account) ?? data;
  const accessToken = pickString(data, 'access_token', 'accessToken', 'token');
  const refreshToken = pickString(data, 'refresh_token', 'refreshToken');
  const userId = pickString(user, 'id', 'user_id', 'userId') || pickString(data, 'id', 'user_id', 'userId');
  const userEmail = pickString(user, 'email') || email;
  if (!accessToken || !userId) throw new Error('Sub2API 登录响应缺少 access_token 或用户 id');
  return { accessToken, refreshToken, userId, email: userEmail };
}

export async function fetchMe(accessToken: string): Promise<{ userId: string; email: string }> {
  const { status, body } = await sub2apiFetch('/api/v1/auth/me', {
    method: 'GET',
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  if (status >= 400) throw new Error(`Sub2API /auth/me 失败（HTTP ${status}）`);
  const data = unwrapData(body);
  const user = asRecord(data?.user) ?? data;
  const userId = pickString(user, 'id', 'user_id', 'userId');
  const email = pickString(user, 'email');
  if (!userId) throw new Error('Sub2API /auth/me 缺少用户 id');
  return { userId, email };
}

export async function refreshAccessToken(refreshToken: string): Promise<Sub2ApiLoginResult> {
  const { status, body } = await sub2apiFetch('/api/v1/auth/refresh', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: refreshToken, refreshToken }),
  });
  if (status >= 400) throw new Error('登录已过期，请重新登录');
  const data = unwrapData(body);
  const user = asRecord(data?.user) ?? data;
  const accessToken = pickString(data, 'access_token', 'accessToken', 'token');
  const nextRefresh = pickString(data, 'refresh_token', 'refreshToken') || refreshToken;
  const userId = pickString(user, 'id', 'user_id', 'userId') || pickString(data, 'id');
  const email = pickString(user, 'email');
  if (!accessToken || !userId) throw new Error('Sub2API 刷新会话失败');
  return { accessToken, refreshToken: nextRefresh, userId, email };
}

export async function listKeys(accessToken: string): Promise<Sub2ApiUserKey[]> {
  const { status, body } = await sub2apiFetch('/api/v1/keys', {
    method: 'GET',
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  if (status === 404) return [];
  if (status >= 400) throw new Error(`Sub2API 列出 Key 失败（HTTP ${status}）`);
  const root = asRecord(body);
  const list = Array.isArray(body)
    ? body
    : Array.isArray(root?.data)
      ? root.data
      : Array.isArray(asRecord(root?.data)?.items)
        ? (asRecord(root?.data)?.items as unknown[])
        : Array.isArray(root?.items)
          ? root.items
          : [];
  return list.flatMap((item) => {
    const record = asRecord(item);
    const id = pickString(record, 'id', 'key_id');
    const name = pickString(record, 'name', 'title', 'remark');
    const key = pickString(record, 'key', 'api_key', 'token');
    return id ? [{ id, name, key: key || undefined }] : [];
  });
}

export async function createUserKey(accessToken: string, name: string): Promise<string> {
  const { status, body } = await sub2apiFetch('/api/v1/keys', {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    body: JSON.stringify({ name }),
  });
  if (status >= 400) throw new Error(`Sub2API 签发 Key 失败（HTTP ${status}）`);
  const data = unwrapData(body);
  const key = pickString(data, 'key', 'api_key', 'token', 'secret');
  if (!key) throw new Error('Sub2API 签发 Key 时没有返回明文 sk');
  return key;
}

export async function listModels(userApiKey: string): Promise<string[]> {
  const { status, body } = await sub2apiFetch('/v1/models', {
    method: 'GET',
    headers: { Authorization: `Bearer ${userApiKey}` },
  });
  if (status >= 400) throw new Error(`Sub2API /v1/models 失败（HTTP ${status}）`);
  const root = asRecord(body);
  const list = Array.isArray(root?.data) ? root.data : [];
  return list.flatMap((item) => {
    const id = pickString(asRecord(item), 'id');
    return id ? [id] : [];
  });
}
