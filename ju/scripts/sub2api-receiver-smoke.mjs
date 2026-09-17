// Run only against an isolated ju deployment. Creates two test users and local
// project/resource records; never requests paid model generation or prints tickets.
import assert from "node:assert/strict";
import { createHmac, randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";

const base = process.env.SUB2API_SMOKE_BASE_URL;
const secret = process.env.SUB2API_SSO_SECRET;
assert(base && secret?.length >= 32, "Set SUB2API_SMOKE_BASE_URL and SUB2API_SSO_SECRET for an isolated deployment");
const request = (path, cookie, init = {}) => fetch(new URL(path, base), {
  ...init, redirect: "manual", signal: AbortSignal.timeout(20_000),
  headers: { Origin: new URL(base).origin, ...(cookie ? { Cookie: cookie } : {}), ...init.headers },
});
const ticket = (subject, overrides = {}) => {
  const now = Math.floor(Date.now() / 1000);
  const body = Buffer.from(JSON.stringify({ sub: subject, username: `smoke_${subject.slice(0, 8)}`, iat: now, exp: now + 120, jti: randomUUID(), next: "/projects", ...overrides })).toString("base64url");
  return `${body}.${createHmac("sha256", secret).update(body).digest("base64url")}`;
};
async function json(path, cookie, init) {
  const response = await request(path, cookie, init);
  assert(response.ok, `${path} HTTP ${response.status}`);
  const value = await response.json();
  assert.equal(value.code, 0, `${path}: ${value.msg}`);
  return value.data;
}
async function login(subject) {
  const raw = ticket(subject);
  const response = await request(`/api/auth/sso/callback?ticket=${encodeURIComponent(raw)}`);
  assert.equal(response.status, 302);
  assert.equal(response.headers.get("location"), "/projects");
  assert.equal(response.headers.get("cache-control"), "no-store");
  const referrerPolicies = response.headers.get("referrer-policy")?.split(",").map((value) => value.trim());
  assert(referrerPolicies?.length && referrerPolicies.every((value) => value === "no-referrer"), "callback must suppress referrers");
  const header = response.headers.get("set-cookie");
  assert(header?.includes("HttpOnly"), "session must be HttpOnly");
  const replay = await request(`/api/auth/sso/callback?ticket=${encodeURIComponent(raw)}`);
  assert.equal(replay.headers.get("location"), "/login?sso_error=1");
  return header.split(";")[0];
}

const firstSubject = randomUUID();
const first = await login(firstSubject);
const second = await login(randomUUID());
const session = await json("/api/auth/session", first);
const reused = await json("/api/auth/session", await login(firstSubject));
assert.equal(session.user.id, reused.user.id, "SSO must reuse the existing identity");
assert.notEqual(session.user.id, (await json("/api/auth/session", second)).user.id);
console.log("PASS SSO session, identity reuse, one-time replay, user separation");

for (const overrides of [{ exp: Math.floor(Date.now() / 1000) - 1 }, { exp: Math.floor(Date.now() / 1000) + 600 }]) {
  const invalid = await request(`/api/auth/sso/callback?ticket=${encodeURIComponent(ticket(firstSubject, overrides))}`);
  assert.equal(invalid.headers.get("location"), "/login?sso_error=1");
}
const unsafeNext = await request(`/api/auth/sso/callback?ticket=${encodeURIComponent(ticket(firstSubject, { next: "https://example.com" }))}`);
assert.equal(unsafeNext.headers.get("location"), "/projects");
console.log("PASS expired/overlong tickets and external redirect rejection");

const catalog = await json("/api/model-catalog", first);
assert.equal(catalog.source, "system", "Deployment must use the system model catalog");
const channel = catalog.channels.find((entry) => entry.id === "sub2api-relay");
assert(channel?.models.some((model) => model.capability === "text" && model.available), "no usable default text model");
const catalogJSON = JSON.stringify(catalog);
assert(!catalogJSON.includes('"apiKey"') && !catalogJSON.includes('"baseUrl"'), "catalog leaks execution credentials");
console.log(`PASS no-config model catalog (${channel.models.map((model) => model.capability).join(", ")})`);

const project = (await json("/api/projects", first, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: "SSO isolation smoke", type: "video", aspectRatio: "16:9" }) })).project;
assert(project?.id);
const forbiddenProject = await request(`/api/projects/${project.id}`, second);
assert([403, 404].includes(forbiddenProject.status), "cross-user project read must fail");

const form = new FormData();
const fixture = await readFile(new URL("../web/public/welcome/charge/sequence.mp4", import.meta.url));
form.set("file", new Blob([fixture], { type: "video/mp4" }), "smoke-fixture.mp4");
form.set("kind", "video");
const resource = (await json("/api/resources", first, { method: "POST", body: form })).resource;
assert(resource?.id);
const range = await request(`/api/resources/${resource.id}/file?proxy=1`, first, { headers: { Range: "bytes=0-31" } });
assert.equal(range.status, 206);
assert.equal(range.headers.get("content-range"), `bytes 0-31/${fixture.length}`);
assert.deepEqual(Buffer.from(await range.arrayBuffer()), fixture.subarray(0, 32));
const forbiddenResource = await request(`/api/resources/${resource.id}/file?proxy=1`, second);
assert([403, 404].includes(forbiddenResource.status), "cross-user resource read must fail");
console.log("PASS project/resource ownership and persisted MP4 Range playback");
console.log("Receiver smoke complete. This does not verify the Sub2API JWT issuer or paid upstream generation.");
