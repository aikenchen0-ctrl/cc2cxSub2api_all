import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("front end should expose batch count input with default 3", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(html, /id="generateCount"[^>]*type="number"[^>]*value="3"/);
  assert.match(html, /id="batchResultGrid"/);
  assert.match(html, /app\.js\?v=20260522-wechat-loading-feedback/);
  assert.match(js, /const generateCountInput = document\.querySelector\("#generateCount"\)/);
  assert.match(js, /function getGenerateCount\(\)/);
  assert.match(js, /DEFAULT_GENERATE_COUNT = 3/);
});

test("generate button should submit requested jobs and render each completed result immediately", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.doesNotMatch(js, /Promise\.all\(\s*Array\.from\(\{ length: generateCount \}/);
  assert.match(js, /Promise\.allSettled\(\s*Array\.from\(\{ length: generateCount \}/);
  assert.match(js, /formData\.append\("async", "true"\)/);
  assert.match(js, /pollGenerateJob\(/);
  assert.match(js, /apiPath\(`generate\/jobs\/\$\{encodeURIComponent\(jobId\)\}`\)/);
  assert.match(js, /const GENERATE_TIMEOUT_MS = 900000/);
  assert.match(js, /const generateTimeoutMs = GENERATE_TIMEOUT_MS \* generateCount/);
  assert.match(js, /appendGeneratedResult\(result, index, generateCount\)/);
  assert.match(js, /selectedGeneratedImageIndex/);
  assert.match(js, /generated-result-card selected/);
  assert.match(js, /selectGeneratedResult\(index\)/);
  assert.match(js, /getSelectedGeneratedResult\(\)/);
});

test("generate button should show queue status for each requested image", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /function updateGeneratedResultStatus\(index, status/);
  assert.match(js, /renderGeneratedResultCards\(totalCount\)/);
  assert.match(js, /updateGeneratedResultStatus\(index, "queued"/);
  assert.match(js, /updateGeneratedResultStatus\(index, data\.status/);
  assert.match(js, /updateGeneratedResultStatus\(index, "failed"/);
  assert.match(js, /生成中/);
  assert.match(js, /排队中/);
  assert.match(js, /生成失败/);
});

test("generate button should not read pipeline from a null response body", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /renderPipeline\(data\?\.pipeline\)/);
  assert.doesNotMatch(js, /renderPipeline\(data\.pipeline\)/);
  assert.match(js, /if \(!data\?\.imageDataUrl\)/);
});

test("WeChat material upload should use the selected generated result", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /const selectedResult = getSelectedGeneratedResult\(\)/);
  assert.match(js, /await getGeneratedMediaFile\(filename, selectedResult\)/);
  assert.match(html, /上传选中生成图到素材库/);
  assert.doesNotMatch(js, /const mediaFile = await getGeneratedMediaFile\(filename\);/);
});
