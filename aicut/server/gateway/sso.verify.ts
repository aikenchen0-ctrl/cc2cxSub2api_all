import assert from 'node:assert/strict';
import { createCipheriv, createHash, createHmac, randomUUID } from 'node:crypto';
import { verifyAicutSSOTicket } from './sso.ts';

const secret = 's'.repeat(32);
process.env.SUB2API_SSO_SECRET = secret;

function ticket(overrides: Record<string, unknown> = {}): string {
  const now = Math.floor(Date.now() / 1000);
  const key = createHash('sha256').update(secret).digest();
  const iv = Buffer.alloc(12, 7);
  const cipher = createCipheriv('aes-256-gcm', key, iv);
  const encrypted = Buffer.concat([cipher.update('sk-super-test', 'utf8'), cipher.final()]);
  const rk = Buffer.concat([iv, encrypted, cipher.getAuthTag()]).toString('base64url');
  const payload = {
    iss: 'sub2api', aud: 'openchatcut', sub: '42', email: 'user@example.com',
    iat: now, exp: now + 60, jti: randomUUID(), next: '/', rk, ...overrides,
  };
  const encoded = Buffer.from(JSON.stringify(payload)).toString('base64url');
  const signature = createHmac('sha256', secret).update(encoded).digest('base64url');
  return `${encoded}.${signature}`;
}

const verified = verifyAicutSSOTicket(ticket());
assert.equal(verified.userId, '42');
assert.equal(verified.relayKey, 'sk-super-test');
assert.equal(verified.next, '/');
assert.throws(() => verifyAicutSSOTicket(ticket({ aud: 'other' })), /claims/);
const replay = ticket();
verifyAicutSSOTicket(replay);
assert.throws(() => verifyAicutSSOTicket(replay), /already used/);

console.log('aicut SSO verify: signature, audience, AES relay key, replay protection hold');
