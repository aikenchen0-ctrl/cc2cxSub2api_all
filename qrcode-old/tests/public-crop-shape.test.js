import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("front end should allow square or circular placement selection", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(html, /name="maskShape" value="square"/);
  assert.match(html, /name="maskShape" value="circle"/);
  assert.match(js, /formData\.append\("maskShape", getMaskShape\(\)\)/);
  assert.match(css, /\.shape-option/);
  assert.match(css, /\.image-frame > \.empty-text/);
  assert.match(css, /\.preview-block > \.empty-text/);
  assert.match(css, /\.preview-block > \.empty-text[^{]*{[\s\S]*position: static/);
  const baseEmptyTextRule = css.match(/\.empty-text\s*{(?<body>[^}]*)}/)?.groups?.body || "";
  assert.doesNotMatch(baseEmptyTextRule, /position:\s*absolute/);
});

test("front end should use the requested default mask placement", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(html, /id="maskX"[^>]*value="250"/);
  assert.match(html, /id="maskY"[^>]*value="270"/);
  assert.match(html, /id="maskSize"[^>]*value="950"/);
  assert.match(js, /DEFAULT_MASK_X = 250/);
  assert.match(js, /DEFAULT_MASK_Y = 270/);
  assert.match(js, /DEFAULT_MASK_SIZE = 950/);
});

test("crop canvas should preserve source image aspect while mapping crop coordinates", async () => {
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(js, /let cropDrawBox = \{ x: 0, y: 0, width: 0, height: 0 \}/);
  assert.match(js, /function getCropDisplayBox\(imageWidth, imageHeight, maxWidth, maxHeight\)/);
  assert.match(js, /function canvasRectToImageRect\(rect\)/);
  assert.match(js, /cropContext\.drawImage\(\s*cropImage,\s*0,\s*0,\s*cropImage\.width,\s*cropImage\.height,\s*cropDrawBox\.x,\s*cropDrawBox\.y,\s*cropDrawBox\.width,\s*cropDrawBox\.height\s*\)/);
  assert.doesNotMatch(js, /cropContext\.drawImage\(cropImage, 0, 0, parseFloat\(cropCanvas\.style\.width\), parseFloat\(cropCanvas\.style\.height\)\)/);
});
