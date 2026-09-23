import { createHmac } from 'node:crypto';
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { consumeSsoHandoff, createSsoHandoff, verifySub2ApiTicket } from './sso';

const SECRET = 'test-sso-secret-with-at-least-32-characters-123';

function ticket(payload: Record<string, unknown>): string {
  const encoded = Buffer.from(JSON.stringify(payload), 'utf8').toString('base64url');
  const mac = createHmac('sha256', SECRET).update(encoded).digest('base64url');
  return `${encoded}.${mac}`;
}

describe('Sub2API SSO ticket', () => {
  it('accepts a valid ticket and sanitizes next', () => {
    process.env.SUB2API_SSO_SECRET = SECRET;
    const result = verifySub2ApiTicket(ticket({
      iss: 'sub2api',
      aud: 'yibiao-bidmonitor',
      sub: 'external-7',
      email: 'user@example.test',
      displayName: 'User',
      iat: 1000,
      exp: 1060,
      jti: 'valid-1',
      next: '//evil.example.test',
    }), 1020);
    assert.equal(result?.user.sub, 'external-7');
    assert.equal(result?.next, '/');
  });

  it('rejects wrong audience, expired tickets, and replayed jti', () => {
    process.env.SUB2API_SSO_SECRET = SECRET;
    const base = { iss: 'sub2api', sub: 'external-8', iat: 1000, exp: 1060, jti: 'replay-1', next: '/' };
    assert.equal(verifySub2ApiTicket(ticket({ ...base, aud: 'other' }), 1020), null);
    assert.equal(verifySub2ApiTicket(ticket({ ...base, aud: 'yibiao-bidmonitor', exp: 1010 }), 1020), null);
    assert.ok(verifySub2ApiTicket(ticket({ ...base, aud: 'yibiao-bidmonitor' }), 1020));
    assert.equal(verifySub2ApiTicket(ticket({ ...base, aud: 'yibiao-bidmonitor' }), 1020), null);
  });

  it('consumes a handoff exactly once', () => {
    const code = createSsoHandoff({ sub: 'external-9', displayName: 'User' }, 60, 2000);
    assert.equal(consumeSsoHandoff(code, 2020)?.sub, 'external-9');
    assert.equal(consumeSsoHandoff(code, 2020), null);
  });
});
