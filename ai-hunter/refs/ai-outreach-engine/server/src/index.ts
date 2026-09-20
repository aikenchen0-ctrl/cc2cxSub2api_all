import { Hono } from "hono";
import { cors } from "hono/cors";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { activeSenderAddresses, env, capabilities } from "./env";
import { getSettings, sendWindowLabel } from "./settings";
import { api } from "./routes/api";
import { unsubscribe } from "./routes/unsubscribe";
import { preflight } from "./compliance/guards";
import { startRunner } from "./pipeline/runner";

const app = new Hono();

app.use("/api/*", cors());
app.route("/api", api);
app.route("/", unsubscribe);

// Serve the built UI when it exists, so `bun start` is a single process.
const webDist = resolve(import.meta.dir, "../../web/dist");
if (existsSync(webDist)) {
  app.get("/*", async (c) => {
    const path = new URL(c.req.url).pathname;
    const candidate = Bun.file(resolve(webDist, `.${path}`));
    if (path !== "/" && (await candidate.exists())) return new Response(candidate);
    return new Response(Bun.file(resolve(webDist, "index.html")), {
      headers: { "Content-Type": "text/html" },
    });
  });
}

startRunner();

const caps = capabilities();
const blockers = preflight();
const settings = getSettings();

console.log(`
  outreach-engine  http://localhost:${env.port}

  llm       ${caps.llm ? `ready (openai: ${env.llm.reasoningModel})` : "NOT CONFIGURED - set OPENAI_API_KEY"}
  crawler   firecrawl${caps.crawler ? "" : " (no key, falling back to plain fetch)"} · max ${settings.crawlMaxPages} pages
  search    ${caps.search ? "ready (serper)" : "NOT CONFIGURED - set SERPER_API_KEY"}
  contacts  scrape + pattern + verify (no paid finders)
  verify    ${caps.verify ? "reoon" : "NOT CONFIGURED - set REOON_API_KEY"}
  sender    ${env.sender.provider}${caps.sendingLive ? ` as ${activeSenderAddresses().fromEmail}` : " (nothing will actually be sent)"}
  cap       ${settings.dailySendCap}/day, ${sendWindowLabel(settings)}
${blockers.length ? `\n  before sending for real:\n${blockers.map((b) => `    - ${b}`).join("\n")}\n` : ""}`);

export default { port: env.port, fetch: app.fetch, idleTimeout: 60 };
