import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto';

export interface SsoUser {
  sub: string;
  email?: string;
  username?: string;
  displayName?: string;
  avatarUrl?: string;
}

export interface SsoTicketResult {
  user: SsoUser;
  next: string;
}

interface HandoffRecord {
  user: SsoUser;
  expiresAt: number;
}

const replayed = new Map<string, number>();
const handoffs = new Map<string, HandoffRecord>();

function secret(): string | null {
  const value = process.env.SUB2API_SSO_SECRET?.trim() || '';
  return value.length >= 32 ? value : null;
}

function safeNext(value: unknown): string {
  if (typeof value !== 'string' || value.length > 2048) return '/';
  const next = value.trim();
  if (!next.startsWith('/') || next.startsWith('//') || next.includes('\\') || /[\r\n]/.test(next)) return '/';
  return next;
}

function cleanUser(payload: Record<string, unknown>): SsoUser | null {
  const sub = typeof payload.sub === 'string' ? payload.sub.trim() : '';
  if (!sub || sub.length > 256) return null;
  const user: SsoUser = { sub };
  for (const key of ['email', 'username', 'displayName', 'avatarUrl'] as const) {
    const value = payload[key];
    if (typeof value === 'string' && value.length <= 512) user[key] = value;
  }
  return user;
}

function clearExpired(map: Map<string, number>, now: number): void {
  for (const [key, expiry] of map) if (expiry <= now) map.delete(key);
}

function clearExpiredHandoffs(now: number): void {
  for (const [key, record] of handoffs) if (record.expiresAt <= now) handoffs.delete(key);
}

export function verifySub2ApiTicket(raw: string, now = Math.floor(Date.now() / 1000)): SsoTicketResult | null {
  const key = secret();
  if (!key || typeof raw !== 'string' || raw.length > 8192) return null;
  const [encoded, mac] = raw.split('.');
  if (!encoded || !mac) return null;
  const expected = createHmac('sha256', key).update(encoded).digest('base64url');
  if (mac.length !== expected.length || !timingSafeEqual(Buffer.from(mac), Buffer.from(expected))) return null;

  let payload: Record<string, unknown>;
  try {
    const parsed = JSON.parse(Buffer.from(encoded, 'base64url').toString('utf8'));
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
    payload = parsed as Record<string, unknown>;
  } catch {
    return null;
  }

  const issuer = payload.iss;
  const audience = payload.aud;
  const issuedAt = Number(payload.iat);
  const expiresAt = Number(payload.exp);
  const jti = typeof payload.jti === 'string' ? payload.jti.trim() : '';
  const user = cleanUser(payload);
  if (
    issuer !== 'sub2api' ||
    audience !== 'yibiao-bidmonitor' ||
    !user ||
    !jti ||
    !Number.isFinite(issuedAt) ||
    !Number.isFinite(expiresAt) ||
    expiresAt <= now ||
    issuedAt > now + 30 ||
    expiresAt - issuedAt > 120
  ) return null;

  clearExpired(replayed, now);
  if (replayed.has(jti)) return null;
  replayed.set(jti, expiresAt);
  return { user, next: safeNext(payload.next) };
}

export function createSsoHandoff(user: SsoUser, ttlSeconds = 60, now = Math.floor(Date.now() / 1000)): string {
  clearExpiredHandoffs(now);
  const code = randomBytes(32).toString('base64url');
  handoffs.set(code, { user, expiresAt: now + Math.max(1, Math.min(ttlSeconds, 120)) });
  return code;
}

export function consumeSsoHandoff(code: string, now = Math.floor(Date.now() / 1000)): SsoUser | null {
  if (!code || code.length > 256) return null;
  for (const [key, record] of handoffs) if (record.expiresAt <= now) handoffs.delete(key);
  const record = handoffs.get(code);
  if (!record || record.expiresAt <= now) return null;
  handoffs.delete(code);
  return record.user;
}
