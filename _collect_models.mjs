import fs from "node:fs/promises";
import { setTimeout as sleep } from "node:timers/promises";

const input = await fs.readFile("当前上游.txt", "utf8");
const groups = input.split(/\r?\n\s*\r?\n/).map(g => g.split(/\r?\n/).map(x => x.trim()).filter(Boolean)).filter(g => g.length >= 3);
const tasks = [];
for (const g of groups) {
  const title = g[0];
  const link = g[1];
  for (const key of g.slice(2)) tasks.push({ title, link, key });
}
function modelsUrl(link) {
  const u = new URL(link);
  const p = u.pathname.replace(/\/+$/, "");
  if (p.endsWith("/v1")) return `${u.origin}${p}/models`;
  return `${u.origin}/v1/models`;
}
function bodySnippet(s) { return s.replace(/\s+/g, " ").slice(0, 240); }
const rows = [];
let lastStart = 0;
for (let i = 0; i < tasks.length; i++) {
  const task = tasks[i];
  const now = Date.now();
  const wait = Math.max(0, 1000 - (now - lastStart));
  if (wait) await sleep(wait);
  lastStart = Date.now();
  let modelList = "";
  let status = 0;
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15000);
    const response = await fetch(modelsUrl(task.link), {
      method: "GET",
      headers: { "Authorization": `Bearer ${task.key}`, "Accept": "application/json" },
      signal: controller.signal,
    });
    clearTimeout(timeout);
    status = response.status;
    const text = await response.text();
    if (response.ok) {
      try {
        const parsed = JSON.parse(text);
        const arr = Array.isArray(parsed?.data) ? parsed.data : (Array.isArray(parsed?.models) ? parsed.models : []);
        const ids = arr.map(x => typeof x === "string" ? x : x?.id).filter(Boolean);
        modelList = ids.length ? ids.join("\n") : (text ? `响应成功但未找到模型字段：${bodySnippet(text)}` : "响应成功但为空");
      } catch {
        modelList = `响应成功但不是 JSON：${bodySnippet(text)}`;
      }
    } else {
      modelList = `请求失败 HTTP ${status}：${bodySnippet(text)}`;
    }
  } catch (e) {
    modelList = e?.name === "AbortError" ? "请求超时（15秒）" : `请求异常：${e?.message ?? String(e)}`;
  }
  rows.push({ title: task.title, link: task.link, key: task.key, models: modelList, status });
  process.stderr.write(`[${i + 1}/${tasks.length}] ${task.title} HTTP ${status || "ERR"}\n`);
}
await fs.writeFile("_model_results.json", JSON.stringify(rows), "utf8");
console.log(`DONE ${rows.length}`);
