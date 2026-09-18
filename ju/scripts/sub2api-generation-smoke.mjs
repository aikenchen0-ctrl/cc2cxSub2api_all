// Opt-in paid smoke against an isolated ju database. Checkpoints contain IDs,
// never credentials. Re-running resumes existing tasks instead of resubmitting.
import assert from "node:assert/strict";
import { createHmac, randomUUID } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";

const base = process.env.SUB2API_SMOKE_BASE_URL;
const secret = process.env.SUB2API_SSO_SECRET;
const inspectOnly = process.argv.includes("--status");
assert(base && secret?.length >= 32, "Set the isolated base URL and SSO secret");
if (!inspectOnly) assert.equal(process.env.SUB2API_SMOKE_PAID, "1", "Explicitly enable the paid smoke with SUB2API_SMOKE_PAID=1");
const checkpoint = new URL("../.local/sub2api-generation-smoke.json", import.meta.url);
let state;
try { state = JSON.parse(await readFile(checkpoint, "utf8")); }
catch (error) { if (error.code !== "ENOENT") throw error; }
state ??= { base, subject: `generation-smoke-${randomUUID()}`, tasks: {} };
assert.equal(state.base, base, "Checkpoint belongs to another deployment");
async function save() {
  await mkdir(new URL("../.local/", import.meta.url), { recursive: true });
  await writeFile(checkpoint, `${JSON.stringify(state, null, 2)}\n`);
}
await save();
const now = Math.floor(Date.now() / 1000);
const payload = Buffer.from(JSON.stringify({ iss: "sub2api", aud: "ju", sub: state.subject, iat: now, exp: now + 120, jti: randomUUID(), next: "/projects" })).toString("base64url");
const ticket = `${payload}.${createHmac("sha256", secret).update(payload).digest("base64url")}`;
const callback = await fetch(new URL(`/api/auth/sso/callback?ticket=${ticket}`, base), { redirect: "manual", signal: AbortSignal.timeout(20_000) });
assert.equal(callback.headers.get("location"), "/auth/sso", "SSO receiver login failed");
const handoff = callback.headers.get("set-cookie")?.split(";")[0];
assert(handoff, "SSO handoff missing");
const exchange = await fetch(new URL("/api/auth/sso/exchange", base), { method: "POST", redirect: "manual", signal: AbortSignal.timeout(20_000), headers: { Cookie: handoff, Origin: new URL(base).origin, "X-Ju-SSO": "1" } });
assert.equal(exchange.status, 200);
const cookie = exchange.headers.getSetCookie().find((value) => !value.startsWith("ju_sso_handoff="))?.split(";")[0];
assert(cookie, "SSO session missing");
async function request(path, init = {}) {
  return fetch(new URL(path, base), {
    ...init, redirect: "manual", signal: AbortSignal.timeout(30_000),
    headers: { Cookie: cookie, Origin: new URL(base).origin, ...init.headers },
  });
}
async function json(path, init) {
  const response = await request(path, init);
  assert(response.ok, `${path} HTTP ${response.status}`);
  const body = await response.json();
  assert.equal(body.code, 0, `${path} business code ${body.code}`);
  return body.data;
}
function post(body) { return { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }; }
if (inspectOnly) {
  for (const [mode, saved] of Object.entries(state.tasks)) {
    if (!saved.id) continue;
    const task = await json(`/api/tasks/${saved.id}`);
    const error = (task.error || "").replace(/https?:\/\/\S+/g, "[URL]").replace(/(?:Bearer\s+|sk-)[\w.-]+/g, "[credential]");
    console.log(JSON.stringify({ mode, id: task.id, status: task.status, stage: task.stage, providerRequestId: task.providerRequestId, error }));
  }
  process.exit(0);
}
const catalog = await json("/api/model-catalog");
const channel = catalog.channels.find((item) => item.id === "sub2api-relay");
const models = { text: "gpt-5.5", video: "minimax_h3_b99_001" };
for (const [mode, id] of Object.entries(models)) {
  assert(channel?.models.some((item) => item.modelKey === id && item.capability === mode && item.available), `Unavailable ${mode} model`);
}
if (!state.projectId) {
  state.projectId = (await json("/api/projects", post({ name: "Sub2API creation acceptance", type: "video", aspectRatio: "16:9" }))).project.id;
  await save();
}
for (const mode of ["text", "video"]) {
  if (!state.tasks[mode]) {
    const prompt = mode === "text"
      ? "Write one short Chinese sentence describing a quiet city street after rain. No heading."
      : "A quiet empty city street after rain, soft daylight reflected on the pavement, slow cinematic camera movement, no people, no text.";
    // A durable local marker prevents accidentally repeating a POST whose
    // response was lost. Resolve such a marker in the task center manually.
    state.tasks[mode] = { submitting: true, operationId: randomUUID() };
    await save();
    const task = await json("/api/tasks", post({
      projectId: state.projectId, type: `canvas_${mode}`, operation: mode === "video" ? "text_to_video" : "text",
      prompt, model: `sub2api-relay::${models[mode]}`,
      input: {
        mode, prompt, config: { channelId: "sub2api-relay", model: models[mode], ...(mode === "video" ? { videoSeconds: "5", vquality: "736p", size: "16:9" } : {}) },
        ...(mode === "text" ? { textOptions: { stream: true, thinking: false } } : {}),
        metadata: { source: "sub2api-paid-smoke", clientOperationId: state.tasks[mode].operationId },
      },
    }));
    state.tasks[mode] = { id: task.id };
    await save();
    console.log(`CREATED ${mode} task ${task.id}`);
  }
  assert(state.tasks[mode].id, `${mode} submission outcome unknown; inspect the task center, do not resubmit`);
  let task;
  let lastStatus;
  const deadline = Date.now() + (mode === "video" ? 20 * 60_000 : 3 * 60_000);
  while (Date.now() < deadline) {
    task = await json(`/api/tasks/${state.tasks[mode].id}`);
    if (task.status !== lastStatus) { console.log(`STATUS ${mode} ${task.status}`); lastStatus = task.status; }
    if (["succeeded", "failed", "cancelled"].includes(task.status)) break;
    await new Promise((resolve) => setTimeout(resolve, 5000));
  }
  state.tasks[mode].status = task.status;
  state.tasks[mode].providerRequestId = task.providerRequestId;
  await save();
  assert.equal(task.status, "succeeded", `${mode} task ${task.id} did not succeed; inspect server task diagnostics`);
  const result = JSON.parse(task.resultJson);
  if (mode === "text") {
    assert(result.text?.trim(), "Text result is empty");
    console.log(`PASS real text generation (${result.text.length} characters)`);
  } else {
    const key = result.video?.storageKey;
    assert(key?.startsWith("resource:"), "Video was not persisted as a resource");
    const resourceId = key.slice("resource:".length);
    const response = await request(`/api/resources/${encodeURIComponent(resourceId)}/file?proxy=1`, { headers: { Range: "bytes=0-31" } });
    assert.equal(response.status, 206, "Video Range request failed");
    assert(response.headers.get("content-type")?.startsWith("video/"));
    assert.equal((await response.arrayBuffer()).byteLength, 32);
    state.tasks[mode].resourceId = resourceId;
    await save();
    console.log(`PASS real video generation and persisted Range playback (${resourceId})`);
  }
}
console.log("Paid generation smoke complete. Browser sidebar/issuer still requires separate verification.");
