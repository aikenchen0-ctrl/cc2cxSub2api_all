import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";
import { pathToFileURL } from "node:url";
import ts from "typescript";

const require = createRequire(import.meta.url);
const nextServerUrl = pathToFileURL(require.resolve("next/server.js")).href;

async function transpiledModule(sourceUrl, replacements = []) {
  let source = await readFile(sourceUrl, "utf8");
  for (const [specifier, replacement] of replacements) {
    source = source.replaceAll(`from "${specifier}"`, `from ${JSON.stringify(replacement)}`);
  }
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
    fileName: sourceUrl.pathname,
  });
  return `data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`;
}

const authUrl = await transpiledModule(new URL("../app/auth-sso.ts", import.meta.url));
const routeUrl = await transpiledModule(new URL("../app/api/ai/route.ts", import.meta.url), [
  ["next/server", nextServerUrl],
  ["../../auth-sso", authUrl],
]);
const { createSession, SESSION_COOKIE } = await import(authUrl);
const { POST } = await import(routeUrl);
const { NextRequest } = await import(nextServerUrl);

const envNames = [
  "SUB2API_SSO_SECRET",
  "SUB2API_APP_CREDENTIAL",
  "SUB2API_RELAY_BASE_URL",
  "SUB2API_RELAY_MODEL",
  "LINK",
];
const subject = "fixture-user-42";
const credential = "fixture-aiexcel-app-credential";
const plan = {
  title: "新增销售数据",
  summary: "在当前工作表追加一条数据",
  operations: [{ type: "insert_rows", sheet: "销售", startRow: 3, values: [["苹果", 10, 5]] }],
  warnings: [],
};

function routeTest(name, run) {
  test(name, { concurrency: false, timeout: 5000 }, async (t) => {
    const originalEnv = new Map(envNames.map((name) => [name, process.env[name]]));
    const originalFetch = globalThis.fetch;
    t.after(() => {
      for (const [name, value] of originalEnv) {
        if (value === undefined) delete process.env[name];
        else process.env[name] = value;
      }
      globalThis.fetch = originalFetch;
    });
    process.env.SUB2API_SSO_SECRET = "fixture-aiexcel-sso-secret-only-for-route-tests-2026";
    process.env.SUB2API_APP_CREDENTIAL = ` ${credential} `;
    process.env.SUB2API_RELAY_BASE_URL = "http://relay.test/v1";
    delete process.env.SUB2API_RELAY_MODEL;
    delete process.env.LINK;
    await run(t);
  });
}

function request({ session = createSession({ sub: subject }), body = {} } = {}) {
  return new NextRequest("http://aiexcel.test/api/ai", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(session ? { Cookie: `${SESSION_COOKIE}=${session}` } : {}),
      "X-Sub2API-On-Behalf-Of": "untrusted-browser-user",
    },
    body: JSON.stringify({
      activeSheet: "销售",
      schema: { sheets: [{ name: "销售", rowCount: 2, columnCount: 3 }] },
      request: "新增一条苹果销售数据",
      userId: "untrusted-body-user",
      ...body,
    }),
  });
}

function successResponse() {
  return Response.json({ choices: [{ message: { content: JSON.stringify(plan) } }] });
}

function rejectUnexpectedFetch() {
  let calls = 0;
  globalThis.fetch = async () => {
    calls += 1;
    throw new Error("No upstream request should occur");
  };
  return () => assert.equal(calls, 0);
}

routeTest("rejects a missing Session without contacting the upstream", async () => {
  const assertNoFetch = rejectUnexpectedFetch();
  const response = await POST(request({ session: "" }));
  assert.equal(response.status, 401);
  assert.deepEqual(await response.json(), { error: "请先通过 Sub2API 登录" });
  assertNoFetch();
});

routeTest("rejects a missing relay address without contacting the upstream", async () => {
  delete process.env.SUB2API_RELAY_BASE_URL;
  const assertNoFetch = rejectUnexpectedFetch();
  const response = await POST(request());
  assert.equal(response.status, 503);
  assert.deepEqual(await response.json(), { error: "服务端尚未配置 Sub2API 中转地址" });
  assertNoFetch();
});

routeTest("rejects a missing app credential without contacting the upstream", async () => {
  process.env.SUB2API_APP_CREDENTIAL = "   ";
  const assertNoFetch = rejectUnexpectedFetch();
  const response = await POST(request());
  assert.equal(response.status, 503);
  assert.deepEqual(await response.json(), { error: "服务端尚未配置 SUB2API_APP_CREDENTIAL" });
  assertNoFetch();
});

routeTest("defaults to gpt-6-astra and binds normalized relay requests to the Session user", async () => {
  const urls = [
    { relay: undefined, link: "localhost:18080////", expected: "http://localhost:18080/v1/chat/completions" },
    { relay: "https://relay.test/v1///", link: "ignored.test", expected: "https://relay.test/v1/chat/completions" },
  ];
  for (const { relay, link, expected } of urls) {
    if (relay === undefined) delete process.env.SUB2API_RELAY_BASE_URL;
    else process.env.SUB2API_RELAY_BASE_URL = relay;
    process.env.LINK = link;
    let calls = 0;
    globalThis.fetch = async (url, init) => {
      calls += 1;
      assert.equal(url, expected);
      assert.equal(init.method, "POST");
      assert.deepEqual(Object.fromEntries(new Headers(init.headers)), {
        authorization: `Bearer ${credential}`,
        "content-type": "application/json",
        "x-sub2api-on-behalf-of": subject,
        "x-sub2api-satellite": "aiexcel",
      });
      const payload = JSON.parse(init.body);
      assert.equal(payload.model, "gpt-6-astra");
      assert.equal(payload.reasoning_effort, "low");
      assert.deepEqual(payload.response_format, { type: "json_object" });
      assert.deepEqual(JSON.parse(payload.messages.at(-1).content), {
        userId: subject,
        activeSheet: "销售",
        workbook: { sheets: [{ name: "销售", rowCount: 2, columnCount: 3 }] },
        request: "新增一条苹果销售数据",
      });
      return successResponse();
    };
    const response = await POST(request());
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), plan);
    assert.equal(calls, 1);
  }
});

routeTest("uses a trimmed server-side model override", async () => {
  process.env.SUB2API_RELAY_MODEL = " gpt-5.6-sol ";
  let payload;
  globalThis.fetch = async (_url, init) => {
    payload = JSON.parse(init.body);
    return successResponse();
  };
  const response = await POST(request({ body: { model: "untrusted-browser-model" } }));
  assert.equal(response.status, 200);
  assert.equal(payload.model, "gpt-5.6-sol");
  assert.equal(payload.reasoning_effort, undefined);
});

routeTest("retains the upstream JSON error and HTTP 404 status", async () => {
  globalThis.fetch = async () => Response.json({ error: { message: "Upstream error: 404" } }, { status: 404 });
  const response = await POST(request());
  assert.equal(response.status, 404);
  assert.deepEqual(await response.json(), { error: "Upstream error: 404" });
});

routeTest("handles a non-JSON upstream 404 without exposing a JSON parse error", async () => {
  globalThis.fetch = async () => new Response("<html>Not Found</html>", {
    status: 404,
    headers: { "Content-Type": "text/html" },
  });
  const response = await POST(request());
  assert.equal(response.status, 404);
  assert.deepEqual(await response.json(), { error: "AI 上游请求失败（404）" });
});

routeTest("duplicate requests keep the original user's lock until its fetch completes", async () => {
  let release;
  let started;
  let calls = 0;
  const fetchStarted = new Promise((resolve) => { started = resolve; });
  globalThis.fetch = () => {
    calls += 1;
    started();
    return new Promise((resolve) => { release = resolve; });
  };
  const original = POST(request());
  await fetchStarted;
  let originalResponse;
  try {
    const duplicate = await POST(request());
    assert.equal(duplicate.status, 429);
    assert.deepEqual(await duplicate.json(), { error: "当前用户已有 AI 任务执行中，请稍后再试" });
    const anotherDuplicate = await POST(request());
    assert.equal(anotherDuplicate.status, 429);
    assert.equal(calls, 1);
  } finally {
    release(successResponse());
    originalResponse = await original;
  }
  assert.equal(originalResponse.status, 200);
  assert.deepEqual(await originalResponse.json(), plan);
  globalThis.fetch = async () => {
    calls += 1;
    return successResponse();
  };
  const next = await POST(request());
  assert.equal(next.status, 200);
  assert.equal(calls, 2);
});
