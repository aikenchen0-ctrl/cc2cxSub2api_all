import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("generate button should require a blank plate template before submitting", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /if \(!templateFile\) {[\s\S]*请先选择空白盘模板/);
  assert.ok(
    js.indexOf("if (!templateFile)") < js.indexOf("generateButton.disabled = true"),
    "template guard should run before the button is disabled and before submit setup"
  );
});

test("generate button should stop when blank plate preview is not created", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /const previewData = await previewMaskNow\(\{ silent: true \}\)/);
  assert.match(js, /if \(!previewData\?\.(debugOverlayDataUrl|collagePreviewDataUrl)[\s\S]*不能继续生成/);
  assert.ok(
    js.indexOf("const previewData = await previewMaskNow({ silent: true })") < js.indexOf("正在按稳定队列生成"),
    "preview guard should run before the final AI generation status"
  );
});
