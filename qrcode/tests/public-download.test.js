import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("generated image panel should expose a direct download control", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(html, /id="downloadOutputImage"/);
  assert.match(html, /download="generated-qrcode\.png"/);
  assert.match(js, /const outputDownloadLink = document\.querySelector\("#downloadOutputImage"\)/);
  assert.match(js, /updateImageDownloadLink\(outputDownloadLink, result\.imageDataUrl, result\.filename/);
  assert.match(css, /\.download-image-button/);
  assert.match(css, /overflow-wrap: anywhere/);
  assert.match(css, /min-width: 0/);
  assert.match(css, /body[^{]*{[\s\S]*overflow-x: hidden/);
  assert.match(css, /\.image-frame[^{]*{[\s\S]*aspect-ratio: 1 \/ 1/);
  assert.match(css, /\.download-image-button[^{]*{[\s\S]*bottom: 12px/);
  assert.match(css, /\.download-image-button[^{]*{[\s\S]*background: rgba\(18, 52, 60, 0\.88\)/);
  assert.match(css, /\.download-icon[^{]*{[\s\S]*border-bottom: 2px solid #fff/);
  assert.match(css, /\.image-frame img:not\(\[src\]\)/);
  assert.match(css, /\.image-frame img\[src=""\]/);
  assert.match(css, /\.image-frame > \.empty-text[^{]*{[\s\S]*inset: 0/);
  assert.match(css, /\.image-frame > \.empty-text[^{]*{[\s\S]*place-items: center/);
  assert.match(css, /\.empty-text\[hidden\][^{]*{[\s\S]*display: none !important/);
  assert.match(css, /@media \(max-width: 420px\)/);
});
