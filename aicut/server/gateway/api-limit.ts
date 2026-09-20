import type { IncomingMessage } from 'node:http';
import { gatewayEnabled } from './config.ts';
import { currentTenant } from './tenant-context.ts';
import { clientIp } from './rate-limit.ts';

const hits = new Map<string, number[]>();

export function apiRateLimitMax(): number {
  const raw = (process.env.OCC_API_RATE_MAX ?? '').trim();
  if (!/^\d+$/.test(raw)) return 120;
  return Math.max(1, Math.min(2000, Number(raw)));
}

export function apiRateLimitWindowMs(): number {
  const raw = (process.env.OCC_API_RATE_WINDOW_MS ?? '').trim();
  if (!/^\d+$/.test(raw)) return 60_000;
  return Math.max(1_000, Math.min(10 * 60_000, Number(raw)));
}

export function consumeApiAttempt(req: IncomingMessage, now = Date.now()): boolean {
  if (!gatewayEnabled()) return true;
  const key = currentTenant()?.userId || clientIp(req);
  const windowMs = apiRateLimitWindowMs();
  const max = apiRateLimitMax();
  const recent = (hits.get(key) ?? []).filter((stamp) => now - stamp < windowMs);
  if (recent.length >= max) {
    hits.set(key, recent);
    return false;
  }
  recent.push(now);
  hits.set(key, recent);
  return true;
}

export function resetApiRateLimitForTests(): void {
  hits.clear();
}
