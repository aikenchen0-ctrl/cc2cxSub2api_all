import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Plugin } from 'vite';
import { gatewayEnabled, openRegisterEnabled } from './config.ts';
import { audit } from './audit.ts';
import { readJsonBody, requestPath, sendJson } from './http.ts';
import { provisionUserApiKey } from './key-provision.ts';
import { isProtectedGatewayPath } from './public-path.ts';
import { clientIp, consumeAuthAttempt } from './rate-limit.ts';
import { consumeApiAttempt } from './api-limit.ts';
import {
  clearSessionCookie,
  maybeRefreshSessionCookie,
  newSession,
  sessionFromRequest,
  setSessionCookie,
} from './session.ts';
import { listModels, loginWithPassword, registerWithPassword } from './sub2api-client.ts';
import { runWithTenant } from './tenant-context.ts';
import { loadVault, saveVault } from './vault.ts';
import { verifyAicutSSOTicket } from './sso.ts';

function isAuthPath(pathname: string): boolean {
  return pathname === '/api/auth/login'
    || pathname === '/api/auth/logout'
    || pathname === '/api/auth/me'
    || pathname === '/api/auth/models'
    || pathname === '/api/auth/register'
    || pathname === '/api/auth/config'
    || pathname === '/api/auth/sso/callback';
}

function rejectAuthRate(req: IncomingMessage, res: ServerResponse, action: string): boolean {
  const ip = clientIp(req);
  if (consumeAuthAttempt(ip)) return false;
  audit(action, { ip, ok: false, reason: 'rate_limited' });
  sendJson(res, 429, { error: '尝试过于频繁，请稍后再试' });
  return true;
}

async function handleLogin(req: IncomingMessage, res: ServerResponse): Promise<void> {
  if (rejectAuthRate(req, res, 'login')) return;
  const ip = clientIp(req);
  const body = await readJsonBody(req);
  const email = typeof body.email === 'string' ? body.email.trim() : '';
  const password = typeof body.password === 'string' ? body.password : '';
  if (!email || !password) {
    audit('login', { ip, ok: false, reason: 'missing_fields' });
    sendJson(res, 400, { error: '请输入邮箱和密码' });
    return;
  }
  try {
    const login = await loginWithPassword(email, password);
    await issueSession(res, login, email);
    audit('login', { ip, ok: true, userId: login.userId });
  } catch (error) {
    audit('login', { ip, ok: false, reason: 'rejected' });
    throw error;
  }
}

async function issueSession(
  res: ServerResponse,
  login: { userId: string; email: string; accessToken: string; refreshToken: string },
  email: string,
): Promise<void> {
  await provisionUserApiKey({
    userId: login.userId,
    email: login.email || email,
    accessToken: login.accessToken,
    refreshToken: login.refreshToken,
  });
  setSessionCookie(res, newSession(login.userId, login.email || email));
  sendJson(res, 200, { user: { id: login.userId, email: login.email || email } });
}

async function handleRegister(req: IncomingMessage, res: ServerResponse): Promise<void> {
  if (!openRegisterEnabled()) {
    sendJson(res, 403, { error: '未开放注册' });
    return;
  }
  if (rejectAuthRate(req, res, 'register')) return;
  const ip = clientIp(req);
  const body = await readJsonBody(req);
  const email = typeof body.email === 'string' ? body.email.trim() : '';
  const password = typeof body.password === 'string' ? body.password : '';
  const name = typeof body.name === 'string' ? body.name.trim() : '';
  if (!email || !password) {
    audit('register', { ip, ok: false, reason: 'missing_fields' });
    sendJson(res, 400, { error: '请输入邮箱和密码' });
    return;
  }
  if (password.length < 8) {
    audit('register', { ip, ok: false, reason: 'weak_password' });
    sendJson(res, 400, { error: '密码至少 8 位' });
    return;
  }
  try {
    const login = await registerWithPassword(email, password, name || undefined);
    await issueSession(res, login, email);
    audit('register', { ip, ok: true, userId: login.userId });
  } catch (error) {
    audit('register', { ip, ok: false, reason: 'rejected' });
    throw error;
  }
}

async function handleConfig(_req: IncomingMessage, res: ServerResponse): Promise<void> {
  sendJson(res, 200, { openRegister: openRegisterEnabled() });
}

async function handleSSOCallback(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? '/', 'http://localhost');
  const raw = url.searchParams.get('ticket') ?? '';
  const ticket = verifyAicutSSOTicket(raw);
  const existing = await loadVault(ticket.userId);
  await saveVault({
    userId: ticket.userId,
    email: ticket.email || existing?.email || '',
    refreshToken: existing?.refreshToken || '',
    userApiKey: ticket.relayKey,
  });
  setSessionCookie(res, newSession(ticket.userId, ticket.email));
  res.statusCode = 302;
  res.setHeader('Cache-Control', 'no-store');
  res.setHeader('Referrer-Policy', 'no-referrer');
  res.setHeader('Location', ticket.next);
  res.end();
}

async function handleMe(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const session = sessionFromRequest(req);
  if (!session) {
    sendJson(res, 401, { error: '未登录' });
    return;
  }
  sendJson(res, 200, { user: { id: session.userId, email: session.email } });
}

async function handleModels(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const session = sessionFromRequest(req);
  if (!session) {
    sendJson(res, 401, { error: '未登录' });
    return;
  }
  const vault = await loadVault(session.userId);
  if (!vault?.userApiKey) {
    sendJson(res, 409, { error: '尚未绑定模型额度' });
    return;
  }
  const models = await listModels(vault.userApiKey);
  sendJson(res, 200, { models });
}

export function gatewayPlugin(): Plugin {
  return {
    name: 'openchatcut-sub2api-gateway',
    configureServer(server) {
      server.middlewares.use(async (req: IncomingMessage, res: ServerResponse, next) => {
        const pathname = requestPath(req);
        if (gatewayEnabled() && isAuthPath(pathname)) {
          try {
            if (pathname === '/api/auth/sso/callback' && req.method === 'GET') {
              await handleSSOCallback(req, res);
              return;
            }
            if (pathname === '/api/auth/login' && req.method === 'POST') {
              await handleLogin(req, res);
              return;
            }
            if (pathname === '/api/auth/register' && req.method === 'POST') {
              await handleRegister(req, res);
              return;
            }
            if (pathname === '/api/auth/config' && (req.method === 'GET' || req.method === 'HEAD')) {
              await handleConfig(req, res);
              return;
            }
            if (pathname === '/api/auth/logout' && req.method === 'POST') {
              const session = sessionFromRequest(req);
              clearSessionCookie(res);
              audit('logout', { ip: clientIp(req), ok: true, userId: session?.userId });
              sendJson(res, 200, { ok: true });
              return;
            }
            if (pathname === '/api/auth/me' && (req.method === 'GET' || req.method === 'HEAD')) {
              await handleMe(req, res);
              return;
            }
            if (pathname === '/api/auth/models' && req.method === 'GET') {
              await handleModels(req, res);
              return;
            }
            sendJson(res, 405, { error: 'method not allowed' });
          } catch (error) {
            const message = error instanceof Error ? error.message : '请求失败';
            const status = pathname === '/api/auth/register' ? 400
              : pathname === '/api/auth/models' ? 502
              : 401;
            sendJson(res, status, { error: message });
          }
          return;
        }
        if (!gatewayEnabled()) {
          next();
          return;
        }
        const session = sessionFromRequest(req);
        if (!session) {
          if (isProtectedGatewayPath(pathname)) {
            sendJson(res, 401, { error: '未登录' });
            return;
          }
          next();
          return;
        }
        try {
          const live = maybeRefreshSessionCookie(res, session);
          const vault = await loadVault(live.userId);
          runWithTenant({
            userId: live.userId,
            email: live.email,
            userApiKey: vault?.userApiKey ?? '',
          }, () => {
            if (isProtectedGatewayPath(pathname) && !consumeApiAttempt(req)) {
              sendJson(res, 429, { error: '请求过于频繁，请稍后再试' });
              return;
            }
            next();
          });
        } catch (error) {
          sendJson(res, 500, { error: error instanceof Error ? error.message : 'gateway error' });
        }
      });
    },
  };
}

export function isGatewayAuthPath(pathname: string): boolean {
  return isAuthPath(pathname);
}
