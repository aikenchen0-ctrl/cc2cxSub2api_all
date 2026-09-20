import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Plugin } from 'vite';
import { gatewayEnabled } from './config.ts';
import { audit } from './audit.ts';
import { requestPath, sendJson } from './http.ts';
import { isProtectedGatewayPath } from './public-path.ts';
import { clientIp } from './rate-limit.ts';
import { consumeApiAttempt } from './api-limit.ts';
import {
  clearSessionCookie,
  maybeRefreshSessionCookie,
  newSession,
  sessionFromRequest,
  setSessionCookie,
} from './session.ts';
import { listModels } from './sub2api-client.ts';
import { runWithTenant } from './tenant-context.ts';
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

function handleLocalAuthDisabled(_req: IncomingMessage, res: ServerResponse): void {
  sendJson(res, 403, {
    error: '请从 Sub2API 左侧菜单进入此应用',
    code: 'SSO_REQUIRED',
  });
}

async function handleConfig(_req: IncomingMessage, res: ServerResponse): Promise<void> {
  // Password login and self-serve registration are intentionally unavailable
  // in satellite mode. Identity is established only by the Sub2API SSO flow.
  sendJson(res, 200, { openRegister: false });
}

async function handleSSOCallback(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? '/', 'http://localhost');
  const raw = url.searchParams.get('ticket') ?? '';
  const ticket = verifyAicutSSOTicket(raw);
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
  const credential = (process.env.SUB2API_APP_CREDENTIAL ?? '').trim();
  if (!credential) {
    sendJson(res, 503, { error: 'Sub2API satellite credential is unavailable' });
    return;
  }
  const models = await listModels(session.userId);
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
            if ((pathname === '/api/auth/login' || pathname === '/api/auth/register') && req.method === 'POST') {
              handleLocalAuthDisabled(req, res);
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
            const status = pathname === '/api/auth/models' ? 502 : 401;
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
          runWithTenant({
            userId: live.userId,
            email: live.email,
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
