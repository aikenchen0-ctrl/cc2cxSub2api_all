import { createHmac } from 'node:crypto';
import Fastify from 'fastify';
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { authRoutes } from './auth';

const SECRET = 'test-sso-secret-with-at-least-32-characters-123';

function ticket(payload: Record<string, unknown>): string {
  const encoded = Buffer.from(JSON.stringify(payload), 'utf8').toString('base64url');
  const mac = createHmac('sha256', SECRET).update(encoded).digest('base64url');
  return `${encoded}.${mac}`;
}

function user() {
  return {
    id: 7,
    username: 'sso-user',
    displayName: 'SSO User',
    role: 'user',
    status: 'active',
    phone: null,
    department: null,
    modules: '["bid-monitor"]',
  };
}

describe('auth SSO routes', () => {
  it('exchanges a one-time handoff for a local JWT session', async () => {
    process.env.JWT_SECRET = 'test-jwt-secret-with-at-least-32-characters-123';
    process.env.SUB2API_SSO_SECRET = SECRET;
    const localUser = user();
    const now = Math.floor(Date.now() / 1000);
    const app = Fastify();
    app.decorate('prisma', {
      userIdentity: {
        findUnique: async () => ({ user: localUser }),
      },
    });
    await app.register(authRoutes, { prefix: '/api' });

    const callback = await app.inject({
      method: 'GET',
      url: `/api/auth/sso/callback?ticket=${encodeURIComponent(ticket({
        iss: 'sub2api', aud: 'yibiao-bidmonitor', sub: 'external-7', iat: now, exp: now + 60, jti: `route-${now}`, next: '/monitor',
      }))}`,
    });
    assert.equal(callback.statusCode, 302);
    const location = new URL(callback.headers.location || '/', 'http://localhost');
    const handoff = location.searchParams.get('sso_handoff');
    assert.ok(handoff);
    assert.equal(location.searchParams.get('redirect'), '/monitor');

    const exchange = await app.inject({
      method: 'POST',
      url: '/api/auth/sso/exchange',
      payload: { handoff },
    });
    assert.equal(exchange.statusCode, 200);
    const body = exchange.json();
    assert.equal(body.user.id, 7);
    assert.equal(typeof body.token, 'string');

    const replay = await app.inject({
      method: 'POST',
      url: '/api/auth/sso/exchange',
      payload: { handoff },
    });
    assert.equal(replay.statusCode, 401);
    await app.close();
  });
});
