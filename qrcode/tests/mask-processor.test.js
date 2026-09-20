import test from "node:test";
import assert from "node:assert/strict";
import sharp from "sharp";

import { buildSquareQrPlatePreview, computeFixedMaskBox } from "../src/mask-processor.js";

async function pixelAlpha(buffer, width, x, y) {
  const { data } = await sharp(buffer).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  return data[(y * width + x) * 4 + 3];
}

test("buildSquareQrPlatePreview should create a circular edit mask when requested", async () => {
  const templateBuffer = await sharp({
    create: {
      width: 120,
      height: 120,
      channels: 4,
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    }
  }).png().toBuffer();
  const qrBuffer = await sharp({
    create: {
      width: 40,
      height: 40,
      channels: 4,
      background: { r: 0, g: 0, b: 0, alpha: 1 }
    }
  }).png().toBuffer();

  const result = await buildSquareQrPlatePreview({
    templateBuffer,
    qrBuffer,
    placement: { x: 10, y: 10, size: 80, shape: "circle" },
    qrTrim: { enabled: false }
  });

  assert.equal(result.squareBox.source.shape, "circle");
  assert.equal(await pixelAlpha(result.editMaskBuffer, 120, 10, 10), 0);
  assert.equal(await pixelAlpha(result.editMaskBuffer, 120, 50, 50), 255);
});

test("computeFixedMaskBox should default to requested placement", () => {
  const box = computeFixedMaskBox({ width: 1400, height: 1400 });

  assert.equal(box.left, 250);
  assert.equal(box.top, 270);
  assert.equal(box.width, 950);
  assert.equal(box.height, 950);
  assert.equal(box.source.x, 250);
  assert.equal(box.source.y, 270);
  assert.equal(box.source.size, 950);
});
