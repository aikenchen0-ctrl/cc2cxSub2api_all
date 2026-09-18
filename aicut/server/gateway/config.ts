/** Gateway is off unless SUB2API_BASE is set. Local OCC stays single-user. */

export function gatewayEnabled(): boolean {
  return Boolean(sub2apiBase());
}

export function sub2apiBase(): string {
  return (process.env.SUB2API_BASE ?? '').trim().replace(/\/+$/, '');
}

export function sessionSecret(): string {
  const secret = (process.env.OCC_SESSION_SECRET ?? '').trim();
  if (secret) return secret;
  if (!gatewayEnabled()) return 'occ-dev-unconfigured';
  throw new Error('OCC_SESSION_SECRET is required when SUB2API_BASE is set');
}

export function occKeyNamePrefix(): string {
  return (process.env.OCC_SUB2API_KEY_NAME ?? 'occ').trim() || 'occ';
}

export const SESSION_COOKIE = 'occ_session';
export const SESSION_TTL_MS = 7 * 24 * 60 * 60 * 1000;

/** Sub2API self-serve sign-up. Off unless explicitly enabled. */
export function openRegisterEnabled(): boolean {
  return gatewayEnabled() && process.env.OCC_OPEN_REGISTER === '1';
}

/** Local OCC exposes MCP; a public gateway keeps it closed unless opted in. */
export function publicMcpEnabled(): boolean {
  if (!gatewayEnabled()) return true;
  return process.env.OCC_PUBLIC_MCP === '1';
}

export function maxExportsPerUser(): number {
  const raw = (process.env.OCC_MAX_EXPORTS_PER_USER ?? '').trim();
  if (!/^\d+$/.test(raw)) return 1;
  return Math.max(1, Math.min(4, Number(raw)));
}

export function maxQueuedExportsPerUser(): number {
  const raw = (process.env.OCC_MAX_QUEUED_EXPORTS_PER_USER ?? '').trim();
  if (!/^\d+$/.test(raw)) return 4;
  return Math.max(1, Math.min(20, Number(raw)));
}

export function maxTranscriptionsPerUser(): number {
  const raw = (process.env.OCC_MAX_TRANSCRIBES_PER_USER ?? '').trim();
  if (!/^\d+$/.test(raw)) return 1;
  return Math.max(1, Math.min(4, Number(raw)));
}

export function maxQueuedTranscriptionsPerUser(): number {
  const raw = (process.env.OCC_MAX_QUEUED_TRANSCRIBES_PER_USER ?? '').trim();
  if (!/^\d+$/.test(raw)) return 4;
  return Math.max(1, Math.min(20, Number(raw)));
}

/** Set OCC_SECURE_COOKIE=1 behind HTTPS. Local HTTP stays without Secure. */
export function secureCookie(): boolean {
  return process.env.OCC_SECURE_COOKIE === '1';
}

/** Per-tenant cumulative upload cap. Default 10 GiB. */
export function tenantUploadQuotaBytes(): number {
  const raw = (process.env.OCC_TENANT_UPLOAD_QUOTA_BYTES ?? '').trim();
  if (!/^\d+$/.test(raw)) return 10 * 1024 ** 3;
  return Math.max(1, Math.min(Number(raw), Number.MAX_SAFE_INTEGER));
}

/** Refresh the OCC session cookie after this much of SESSION_TTL_MS has elapsed. */
export function sessionRefreshAfterMs(): number {
  return Math.floor(SESSION_TTL_MS / 2);
}
