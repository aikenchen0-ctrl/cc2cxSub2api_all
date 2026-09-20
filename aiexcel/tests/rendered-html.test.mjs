import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

async function render(path = "/") {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);

  return worker.fetch(
    new Request(`http://localhost${path}`, { headers: { accept: "text/html" } }),
    {
      ASSETS: {
        fetch: async () => new Response("Not found", { status: 404 }),
      },
    },
    {
      waitUntil() {},
      passThroughOnException() {},
    },
  );
}

test("server-renders the current AI Excel landing page", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>格知 .*AI Excel Copilot<\/title>/i);
  assert.match(html, /Excel/);
  assert.match(html, /选择文件/);
  assert.match(html, /原始文件不会上传/);
  assert.doesNotMatch(html, /Your site is taking shape|Building your site|codex-preview|react-loading-skeleton/i);
});

test("keeps the SSO-only AI route and server-side relay contract", async () => {
  const [page, route, auth] = await Promise.all([
    readFile(new URL("../app/page.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/api/ai/route.ts", import.meta.url), "utf8"),
    readFile(new URL("../app/auth-sso.ts", import.meta.url), "utf8"),
  ]);

  assert.match(page, /fetch\("\/api\/auth\/sso\/exchange"/);
  assert.match(page, /fetch\("\/api\/ai"/);
  assert.match(route, /readSession\(/);
  assert.match(route, /status: 401/);
  assert.match(route, /satelliteHeaders\(user\.sub\)/);
  assert.match(auth, /X-Sub2API-On-Behalf-Of/);
  assert.match(auth, /X-Sub2API-Satellite.*aiexcel/);
  assert.doesNotMatch(route, /SUB2API_RELAY_API_KEY|sk-super-|apiKey/);
});
