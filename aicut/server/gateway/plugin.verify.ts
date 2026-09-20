import assert from 'node:assert/strict';
import { encodeSession, decodeSession, newSession, sessionNeedsRefresh } from './session.ts';
import {
  gatewayEnabled,
  maxExportsPerUser,
  maxQueuedExportsPerUser,
  openRegisterEnabled,
  publicMcpEnabled,
  secureCookie,
  tenantUploadQuotaBytes,
} from './config.ts';
import { publicMcpAllowed } from './mcp-policy.ts';
import { r2UploadObjectKey } from './r2-key.ts';
import { tenantRoot } from './tenant.ts';
import { runWithTenant } from './tenant-context.ts';
import { assertUserExportQueue } from './export-queue.ts';
import { isAnonymousGatewayPath, isProtectedGatewayPath } from './public-path.ts';
import { consumeAuthAttempt, resetAuthRateLimitForTests } from './rate-limit.ts';
import { consumeApiAttempt, resetApiRateLimitForTests } from './api-limit.ts';
import { applyGatewaySecurityHeaders } from './security-headers.ts';
import { assertTenantUploadQuota, recordTenantUpload, resetUploadQuotaForTests, TenantUploadQuotaError } from './upload-quota.ts';
import type { IncomingMessage } from 'node:http';

assert.equal(gatewayEnabled(), false, 'gateway stays off without SUB2API_BASE');
assert.equal(openRegisterEnabled(), false, 'open register stays off without gateway');
assert.equal(publicMcpEnabled(), true, 'local OCC keeps MCP on');
assert.equal(publicMcpAllowed(), true);
assert.equal(maxExportsPerUser(), 1);
assert.equal(maxQueuedExportsPerUser(), 4);
assert.equal(r2UploadObjectKey('clip.mp4'), 'uploads/clip.mp4');
assert.equal(isProtectedGatewayPath('/api/project-store'), true);
assert.equal(isAnonymousGatewayPath('/api/auth/login'), true);
assert.equal(isProtectedGatewayPath('/api/auth/login'), false);
assert.equal(isProtectedGatewayPath('/src/App.tsx'), false);
assert.equal(consumeAuthAttempt('127.0.0.1'), true, 'rate limit idle when gateway is off');

const session = newSession('user-1', 'a@b.c');
const token = encodeSession(session);
const roundTrip = decodeSession(token);
assert.equal(roundTrip?.userId, 'user-1');
assert.equal(roundTrip?.email, 'a@b.c');
assert.equal(decodeSession('tampered.token'), null);

const root = tenantRoot('user/../../etc');
assert.match(root, /tenants[/\\]user_+etc$/);
assert.doesNotMatch(root, /[.][.]/);

process.env.SUB2API_BASE = 'https://sub2api.example';
process.env.OCC_SESSION_SECRET = 'verify-secret';
assert.equal(gatewayEnabled(), true);
assert.equal(openRegisterEnabled(), false);
assert.equal(publicMcpEnabled(), false);
assert.equal(publicMcpAllowed(), false);
assert.equal(isProtectedGatewayPath('/media/uploads/clip.mp4'), true);
assert.equal(isProtectedGatewayPath('/llm'), true);
assert.equal(isProtectedGatewayPath('/export/job'), true);
assert.equal(isProtectedGatewayPath('/api/auth/config'), false);
assert.throws(
  () => r2UploadObjectKey('clip.mp4'),
  /logged-in tenant/,
);
runWithTenant({ userId: 'user/../../etc', email: 'a@b.c' }, () => {
  const key = r2UploadObjectKey('clip.mp4');
  assert.match(key, /^uploads\/tenants\/user_+etc\/clip\.mp4$/);
  assert.doesNotMatch(key, /[.][.]/);
});

process.env.OCC_OPEN_REGISTER = '1';
process.env.OCC_PUBLIC_MCP = '1';
assert.equal(openRegisterEnabled(), true);
assert.equal(publicMcpEnabled(), true);
assert.equal(publicMcpAllowed(), true);
assert.equal(isAnonymousGatewayPath('/api/external-mcp/mcp'), true);

process.env.OCC_MAX_QUEUED_EXPORTS_PER_USER = '1';
assert.equal(maxQueuedExportsPerUser(), 1);
assert.doesNotThrow(() => assertUserExportQueue('user-a'));

process.env.OCC_AUTH_RATE_MAX = '2';
process.env.OCC_AUTH_RATE_WINDOW_MS = '60000';
resetAuthRateLimitForTests();
assert.equal(consumeAuthAttempt('10.0.0.8'), true);
assert.equal(consumeAuthAttempt('10.0.0.8'), true);
assert.equal(consumeAuthAttempt('10.0.0.8'), false, 'third login attempt is rate-limited');
assert.equal(consumeAuthAttempt('10.0.0.9'), true, 'other IPs keep their own bucket');

assert.equal(secureCookie(), false);
process.env.OCC_SECURE_COOKIE = '1';
assert.equal(secureCookie(), true);

process.env.OCC_API_RATE_MAX = '2';
process.env.OCC_API_RATE_WINDOW_MS = '60000';
resetApiRateLimitForTests();
const fakeReq = { socket: { remoteAddress: '10.1.2.3' }, headers: {} } as IncomingMessage;
runWithTenant({ userId: 'u1', email: 'a@b.c' }, () => {
  assert.equal(consumeApiAttempt(fakeReq), true);
  assert.equal(consumeApiAttempt(fakeReq), true);
  assert.equal(consumeApiAttempt(fakeReq), false, 'third API call is rate-limited');
});

assert.equal(sessionNeedsRefresh({ userId: 'u', email: 'a@b.c', issuedAt: Date.now() }), false);
assert.equal(sessionNeedsRefresh({ userId: 'u', email: 'a@b.c', issuedAt: Date.now() - (4 * 24 * 60 * 60 * 1000) }), true);

const headers: Record<string, string> = {};
applyGatewaySecurityHeaders({ setHeader(name, value) { headers[name] = value; } });
assert.equal(headers['X-Content-Type-Options'], 'nosniff');
assert.equal(headers['X-Frame-Options'], 'DENY');

process.env.OCC_TENANT_UPLOAD_QUOTA_BYTES = '100';
resetUploadQuotaForTests();
const quotaTenant = { userId: 'quota-user', email: 'a@b.c' };
await runWithTenant(quotaTenant, () => assertTenantUploadQuota(40));
await runWithTenant(quotaTenant, () => recordTenantUpload(40));
await runWithTenant(quotaTenant, () => assertTenantUploadQuota(60));
let quotaRejected = false;
try {
  await runWithTenant(quotaTenant, () => assertTenantUploadQuota(61));
} catch (error) {
  quotaRejected = error instanceof TenantUploadQuotaError;
}
assert.equal(quotaRejected, true, 'upload over quota is rejected');
assert.equal(tenantUploadQuotaBytes(), 100);

delete process.env.SUB2API_BASE;
delete process.env.OCC_SESSION_SECRET;
delete process.env.OCC_OPEN_REGISTER;
delete process.env.OCC_PUBLIC_MCP;
delete process.env.OCC_MAX_QUEUED_EXPORTS_PER_USER;
delete process.env.OCC_AUTH_RATE_MAX;
delete process.env.OCC_AUTH_RATE_WINDOW_MS;
delete process.env.OCC_SECURE_COOKIE;
delete process.env.OCC_API_RATE_MAX;
delete process.env.OCC_API_RATE_WINDOW_MS;
delete process.env.OCC_TENANT_UPLOAD_QUOTA_BYTES;

console.log('gateway.verify: session, tenant, R2 prefix, MCP, register, auth gate, rate limit, cookie, API limit, quota, headers hold');
