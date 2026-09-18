const USER_ID_PATTERN = /^[1-9][0-9]{0,18}$/;
const SENSITIVE_KEYS = new Set([
  'api_key',
  'password',
  'access_key_secret',
  'token',
  'secret',
  'service_token',
  'wechat_token',
]);
const MAX_STRING_LENGTH = 4000;
const MAX_DEPTH = 8;

export function normalizeMonitorUserId(value: unknown): string {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value) || value <= 0) throw new Error('user id must be a positive integer');
    return String(value);
  }
  if (typeof value !== 'string') throw new Error('user id must be a positive integer');
  const normalized = value.trim();
  if (!USER_ID_PATTERN.test(normalized)) throw new Error('user id must be a positive integer');
  return normalized;
}

export function sanitizeMonitorConfig(value: unknown, depth = 0): unknown {
  if (depth > MAX_DEPTH) return null;
  if (typeof value === 'string') return value.slice(0, MAX_STRING_LENGTH);
  if (value === null || typeof value === 'number' || typeof value === 'boolean') return value;
  if (Array.isArray(value)) return value.slice(0, 200).map((item) => sanitizeMonitorConfig(item, depth + 1));
  if (typeof value !== 'object') return null;

  const output: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value)) {
    if (SENSITIVE_KEYS.has(key.toLowerCase())) continue;
    output[key] = sanitizeMonitorConfig(item, depth + 1);
  }
  return output;
}
