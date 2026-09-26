import { currentTenant } from './tenant-context.ts';
import { gatewayEnabled, sub2apiBase } from './config.ts';

export function satelliteCredential(): string {
  return (process.env.SUB2API_APP_CREDENTIAL ?? '').trim();
}

export function satelliteOrigin(): string {
  return sub2apiBase().replace(/\/v1\/?$/i, '');
}

export function satelliteV1Base(): string {
  const origin = satelliteOrigin();
  return origin ? `${origin}/v1` : '';
}

export function satellitePurchaseUrl(): string | null {
  const raw = (process.env.LINK ?? '').trim();
  if (!raw) return null;
  try {
    const url = new URL(/^https?:\/\//i.test(raw) ? raw : `http://${raw}`);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || url.username || url.password) return null;
    url.pathname = '/purchase';
    url.search = '';
    url.hash = '';
    return url.toString().replace(/\/$/, '');
  } catch {
    return null;
  }
}

export function satelliteHeadersForUser(userId?: string): Record<string, string> | null {
  const credential = satelliteCredential();
  const subject = (userId ?? currentTenant()?.userId ?? '').trim();
  if (!gatewayEnabled() || !credential || !subject) return null;
  return {
    Authorization: `Bearer ${credential}`,
    'X-Sub2API-On-Behalf-Of': subject,
    'X-Sub2API-Satellite': 'aicut',
  };
}

export function satelliteHeaders(): Record<string, string> | null {
  return satelliteHeadersForUser(currentTenant()?.userId);
}

export function satelliteReady(): boolean {
  return Boolean(satelliteHeaders() && satelliteV1Base());
}

export function satelliteImageModel(local: string): string {
  switch (local) {
    case 'grok-imagine':
      return 'grok-imagine-image-1.5';
    case 'nano-banana':
      return 'gemini-3.1-flash-image';
    case 'gpt-image-2':
      return 'gpt-image-2';
    default:
      return 'gpt-image-2';
  }
}

export function satelliteVideoModel(local: string): string {
  switch (local) {
    case 'seedance2':
    case 'byteplus':
      return 'seedance-2.0';
    case 'kling':
      return 'kling-v3';
    case 'grok-imagine-video':
    case 'hailuo':
    case 'ofox':
      return 'grok-imagine-video-1.5';
    default:
      return 'grok-imagine-video-1.5';
  }
}

export function satelliteCapsEnabled(): boolean {
  return Boolean(satelliteCredential() && (process.env.SUB2API_BASE || process.env.LINK));
}
