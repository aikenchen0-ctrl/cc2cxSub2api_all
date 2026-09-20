import { sub2apiBase } from './config.ts';

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

export async function listModels(subject: string): Promise<string[]> {
  const credential = (process.env.SUB2API_APP_CREDENTIAL ?? '').trim();
  const userId = subject.trim();
  if (!credential) throw new Error('Sub2API satellite credential is unavailable');
  if (!userId) throw new Error('当前会话未提供 Sub2API 用户身份');
  const headers: Record<string, string> = {
    Authorization: `Bearer ${credential}`,
    'X-Sub2API-On-Behalf-Of': userId,
    'X-Sub2API-Satellite': 'aicut',
  };
  const { status, body } = await sub2apiFetch('/v1/models', {
    method: 'GET',
    headers,
  });
  if (status >= 400) throw new Error(`Sub2API /v1/models failed (HTTP ${status})`);
  const root = asRecord(body);
  const list = Array.isArray(root?.data) ? root.data : [];
  return list.flatMap((item) => {
    const id = pickString(asRecord(item), 'id');
    return id ? [id] : [];
  });
}
