import { createHmac, timingSafeEqual } from 'node:crypto';

interface AicutTicket {
  iss?: string;
  aud?: string;
  sub: string;
  email?: string;
  iat: number;
  exp: number;
  jti: string;
  next?: string;
  rk?: string;
}

const consumed = new Map<string, number>();

function secret(): string {
  return (process.env.SUB2API_SSO_SECRET ?? '').trim();
}

function safeNext(value: string | undefined): string {
  if (!value || value.length > 2048 || !value.startsWith('/') || value.startsWith('//') || value.includes('\\') || /[\r\n]/.test(value)) return '/';
  return value;
}

function consume(jti: string, exp: number): boolean {
  const now = Date.now();
  for (const [key, expires] of consumed) if (expires <= now) consumed.delete(key);
  if (consumed.has(jti)) return false;
  consumed.set(jti, exp * 1000);
  return true;
}

export interface VerifiedAicutSSO {
  userId: string;
  email: string;
  relayKey: string;
  next: string;
}

export function verifyAicutSSOTicket(raw: string, now = Math.floor(Date.now() / 1000)): VerifiedAicutSSO {
  const shared = secret();
  if (shared.length < 32) throw new Error('OpenChatCut SSO is not configured');
  const parts = raw.split('.');
  const encoded = parts[0];
  const signature = parts[1];
  if (parts.length !== 2 || !encoded || !signature || raw.length > 16_384 || encoded.length > 12_000) throw new Error('invalid SSO ticket');
  const expected = createHmac('sha256', shared).update(encoded).digest('base64url');
  const left = Buffer.from(signature);
  const right = Buffer.from(expected);
  if (left.length !== right.length || !timingSafeEqual(left, right)) throw new Error('invalid SSO ticket signature');
  let ticket: AicutTicket;
  try { ticket = JSON.parse(Buffer.from(encoded, 'base64url').toString('utf8')) as AicutTicket; } catch { throw new Error('invalid SSO ticket payload'); }
  if (ticket.iss !== 'sub2api' || ticket.aud !== 'openchatcut'
    || !/^[0-9]+$/.test(ticket.sub) || ticket.sub.length > 32
    || !/^[a-f0-9-]{16,128}$/i.test(ticket.jti)) {
    throw new Error('invalid SSO ticket claims');
  }
  if (!Number.isSafeInteger(ticket.iat) || !Number.isSafeInteger(ticket.exp) || ticket.exp <= now || ticket.iat > now + 30 || ticket.exp - ticket.iat > 180) throw new Error('expired SSO ticket');
  if (!consume(ticket.jti, ticket.exp)) throw new Error('SSO ticket already used');
  return { userId: ticket.sub, email: ticket.email ?? '', relayKey: '', next: safeNext(ticket.next) };
}
