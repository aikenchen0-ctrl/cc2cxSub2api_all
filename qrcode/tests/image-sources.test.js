import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { collectImageDataUrls } from "../src/image-sources.js";

test("collectImageDataUrls should load accessible local images and ignore missing ones", async () => {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "chatlog-script-"));
  const imagePath = path.join(tempDir, "sample.png");

  await fs.writeFile(imagePath, Buffer.from("fake-image"));

  const result = await collectImageDataUrls({
    localPaths: [imagePath, path.join(tempDir, "missing.png")],
    remoteUrls: [],
    maxImages: 5
  });

  assert.equal(result.images.length, 1);
  assert.equal(result.images[0].source, imagePath);
  assert.equal(result.images[0].dataUrl.startsWith("data:image/png;base64,"), true);
  assert.equal(result.skipped.length, 1);
});

test("collectImageDataUrls should respect the maxImages limit", async () => {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "chatlog-script-"));
  const firstPath = path.join(tempDir, "one.jpg");
  const secondPath = path.join(tempDir, "two.jpg");

  await fs.writeFile(firstPath, Buffer.from("first"));
  await fs.writeFile(secondPath, Buffer.from("second"));

  const result = await collectImageDataUrls({
    localPaths: [firstPath, secondPath],
    remoteUrls: [],
    maxImages: 1
  });

  assert.equal(result.images.length, 1);
  assert.equal(result.images[0].source, firstPath);
  assert.equal(result.skipped.length, 1);
});
