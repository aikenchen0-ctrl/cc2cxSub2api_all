import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { SESSION_COOKIE, SESSION_TTL_MS, secureCookie, sessionRefreshAfterMs, sessionSecret } from './config.ts';

export interface OccupiedSession {
  userId: string;
  email: string;
  issuedAt: number;
}

function sign(payload: string): string {
  return createHmac('sha256', sessionSecret()).update(payload).digest('base64url');
}

export function encodeSession(session: OccupiedSession): string {
  const payload = Buffer.from(JSON.stringify(session), 'utf8').toString('base64url');
  return `${payload}.${sign(payload)}`;
}

export function decodeSession(token: string): OccupiedSession | null {
  const [payload, signature] = token.split('.');
  if (!payload || !signature) return null;
  const expected = sign(payload);
  const left = Buffer.from(signature);
  const right = Buffer.from(expected);
  if (left.length !== right.length || !timingSafeEqual(left, right)) return null;
  try {
    const parsed = JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')) as OccupiedSession;
    if (!parsed?.userId || typeof parsed.issuedAt !== 'number') return null;
    if (Date.now() - parsed.issuedAt > SESSION_TTL_MS) return null;
    return parsed;
  } catch {
    return null;
  }
}

export function readCookie(req: IncomingMessage, name = SESSION_COOKIE): string {
  const header = req.headers.cookie;
  if (!header) return '';
  for (const part of header.split(';')) {
    const [rawName, ...rest] = part.trim().split('=');
    if (rawName === name) return decodeURIComponent(rest.join('='));
  }
  return '';
}

export function sessionFromRequest(req: IncomingMessage): OccupiedSession | null {
  const token = readCookie(req);
  return token ? decodeSession(token) : null;
}

function cookieAttrs(maxAge: number): string {
  const secure = secureCookie() ? '; Secure' : '';
  return `Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}${secure}`;
}

export function setSessionCookie(res: ServerResponse, session: OccupiedSession): void {
  const token = encodeSession(session);
  res.setHeader('Set-Cookie', `${SESSION_COOKIE}=${encodeURIComponent(token)}; ${cookieAttrs(Math.floor(SESSION_TTL_MS / 1000))}`);
}

export function clearSessionCookie(res: ServerResponse): void {
  res.setHeader('Set-Cookie', `${SESSION_COOKIE}=; ${cookieAttrs(0)}`);
}

export function newSession(userId: string, email: string): OccupiedSession {
  return { userId, email, issuedAt: Date.now() };
}

export function sessionNeedsRefresh(session: OccupiedSession, now = Date.now()): boolean {
  return now - session.issuedAt >= sessionRefreshAfterMs();
}

export function maybeRefreshSessionCookie(res: ServerResponse, session: OccupiedSession): OccupiedSession {
  if (!sessionNeedsRefresh(session)) return session;
  const next = newSession(session.userId, session.email);
  setSessionCookie(res, next);
  return next;
}

export function randomId(bytes = 16): string {
  return randomBytes(bytes).toString('hex');
}
