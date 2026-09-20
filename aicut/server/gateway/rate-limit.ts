import type { IncomingMessage } from 'node:http';
import { gatewayEnabled } from './config.ts';

const hits = new Map<string, number[]>();

export function authRateLimitMax(): number {
  const raw = (process.env.OCC_AUTH_RATE_MAX ?? '').trim();
  if (!/^\d+$/.test(raw)) return 10;
  return Math.max(1, Math.min(100, Number(raw)));
}

export function authRateLimitWindowMs(): number {
  const raw = (process.env.OCC_AUTH_RATE_WINDOW_MS ?? '').trim();
  if (!/^\d+$/.test(raw)) return 15 * 60_000;
  return Math.max(1_000, Math.min(60 * 60_000, Number(raw)));
}

export function clientIp(req: IncomingMessage): string {
  if (process.env.OCC_TRUST_PROXY === '1') {
    const forwarded = req.headers['x-forwarded-for'];
    const first = typeof forwarded === 'string' ? forwarded.split(',')[0]?.trim() : '';
    if (first) return first;
  }
  return req.socket.remoteAddress ?? 'unknown';
}

export function consumeAuthAttempt(ip: string, now = Date.now()): boolean {
  if (!gatewayEnabled()) return true;
  const windowMs = authRateLimitWindowMs();
  const max = authRateLimitMax();
  const recent = (hits.get(ip) ?? []).filter((stamp) => now - stamp < windowMs);
  if (recent.length >= max) {
    hits.set(ip, recent);
    return false;
  }
  recent.push(now);
  hits.set(ip, recent);
  return true;
}

export function resetAuthRateLimitForTests(): void {
  hits.clear();
}
