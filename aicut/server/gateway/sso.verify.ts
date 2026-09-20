import assert from 'node:assert/strict';
import { createHmac, randomUUID } from 'node:crypto';
import { verifyAicutSSOTicket } from './sso.ts';

const secret = 's'.repeat(32);
process.env.SUB2API_SSO_SECRET = secret;

function ticket(overrides: Record<string, unknown> = {}): string {
  const now = Math.floor(Date.now() / 1000);
  const payload = {
    iss: 'sub2api', aud: 'openchatcut', sub: '42', email: 'user@example.com',
    iat: now, exp: now + 60, jti: randomUUID(), next: '/', ...overrides,
  };
  const encoded = Buffer.from(JSON.stringify(payload)).toString('base64url');
  const signature = createHmac('sha256', secret).update(encoded).digest('base64url');
  return `${encoded}.${signature}`;
}

const verified = verifyAicutSSOTicket(ticket());
assert.equal(verified.userId, '42');
assert.equal(verified.relayKey, '');
assert.equal(verified.next, '/');
assert.equal(verifyAicutSSOTicket(ticket({ rk: 'ignored', jti: randomUUID() })).relayKey, '');
assert.throws(() => verifyAicutSSOTicket(ticket({ aud: 'other' })), /claims/);
const replay = ticket();
verifyAicutSSOTicket(replay);
assert.throws(() => verifyAicutSSOTicket(replay), /already used/);

console.log('aicut SSO verify: signature, audience, identity-only ticket, replay protection hold');
