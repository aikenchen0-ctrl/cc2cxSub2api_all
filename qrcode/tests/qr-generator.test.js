import test from "node:test";
import assert from "node:assert/strict";
import sharp from "sharp";

import { generateQrArtwork } from "../src/qr-generator.js";

async function createTestImageBuffer() {
  return sharp({
    create: {
      width: 8,
      height: 8,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  })
    .png()
    .toBuffer();
}

async function createTemplateBuffer() {
  const svg = `
    <svg width="2048" height="1600" xmlns="http://www.w3.org/2000/svg">
      <rect width="2048" height="1600" fill="white"/>
      <circle cx="1003" cy="1130" r="520" fill="white" stroke="#c8a55a" stroke-width="12"/>
    </svg>
  `;
  return sharp(Buffer.from(svg)).png().toBuffer();
}

test("generateQrArtwork should expose endpoint and root cause when fetch fails", async () => {
  const imageBuffer = await createTestImageBuffer();

  await assert.rejects(
    () =>
      generateQrArtwork({
        imageBuffer,
        mimeType: "image/png",
        fetchImpl: async () => {
          const error = new TypeError("fetch failed");
          error.cause = {
            code: "UND_ERR_CONNECT_TIMEOUT",
            message: "Connect Timeout Error"
          };
          throw error;
        }
    }),
    (error) => {
      assert.match(error.message, /接口提交失败/);
      assert.match(error.message, /UND_ERR_CONNECT_TIMEOUT/);
      assert.equal(error.pipeline.submitAttempts.length, 1);
      return true;
    }
  );
});

test("generateQrArtwork artistic mode should send a centered full QR canvas", async () => {
  const imageBuffer = await sharp({
    create: {
      width: 320,
      height: 320,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  })
    .composite([
      {
        input: Buffer.from('<svg width="320" height="320"><rect x="80" y="80" width="160" height="160" fill="black"/></svg>')
      }
    ])
    .png()
    .toBuffer();
  const responseBuffer = await sharp({
    create: {
      width: 64,
      height: 64,
      channels: 3,
      background: { r: 28, g: 116, b: 132 }
    }
  }).png().toBuffer();
  let submittedImage;
  let submittedMask;
  let submittedPrompt;
  let submittedInputFidelity;

  const result = await generateQrArtwork({
    imageBuffer,
    mimeType: "image/png",
    artisticQr: true,
    apiKey: "test-api-key",
    fetchImpl: async (url, options = {}) => {
      if (!url.includes("/v1/images/edits")) {
        throw new Error(`unexpected url: ${url}`);
      }

      submittedImage = Buffer.from(await options.body.get("image").arrayBuffer());
      submittedMask = Buffer.from(await options.body.get("mask").arrayBuffer());
      submittedPrompt = options.body.get("prompt");
      submittedInputFidelity = options.body.get("input_fidelity");
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          data: [{ b64_json: responseBuffer.toString("base64") }]
        })
      };
    }
  });

  const imageMeta = await sharp(submittedImage).metadata();
  const maskMeta = await sharp(submittedMask).metadata();
  assert.deepEqual(
    { width: imageMeta.width, height: imageMeta.height },
    { width: 1024, height: 1024 }
  );
  assert.deepEqual(
    { width: maskMeta.width, height: maskMeta.height },
    { width: 1024, height: 1024 }
  );
  assert.match(submittedPrompt, /entire square QR image/i);
  assert.doesNotMatch(submittedPrompt, /embedded inside another image/i);
  assert.equal(submittedInputFidelity, "high");
  assert.equal(result.pipeline.artisticQr, true);
  assert.equal(result.pipeline.uploadContextMode, "artistic-qr-canvas");
  assert.equal(result.pipeline.editMode, "artistic-qr-edits");

  const outputBuffer = Buffer.from(result.imageDataUrl.split(",")[1], "base64");
  const { data: outputPixels, info: outputInfo } = await sharp(outputBuffer)
    .raw()
    .toBuffer({ resolveWithObject: true });
  const corner = outputPixels.slice(0, outputInfo.channels);
  const centerOffset = ((outputInfo.height / 2) * outputInfo.width + outputInfo.width / 2) * outputInfo.channels;
  const center = outputPixels.slice(centerOffset, centerOffset + outputInfo.channels);
  assert.ok(corner[0] > 240 && corner[1] > 240 && corner[2] > 240);
  assert.ok(center[0] < 100 && center[2] > 30);
});

test("generateQrArtwork should submit gpt-image-2 edit task and return edited image", async () => {
  const imageBuffer = await createTestImageBuffer();
  const templateBuffer = await createTemplateBuffer();
  const referenceBuffer = await createTestImageBuffer();
  const requests = [];
  const pngBytes = Buffer.from([137, 80, 78, 71]);

  const result = await generateQrArtwork({
    imageBuffer,
    mimeType: "image/png",
    templateImage: { buffer: templateBuffer, mimeType: "image/png" },
    referenceImages: [{ buffer: referenceBuffer, mimeType: "image/png" }],
    positivePrompt: "POSITIVE",
    negativePrompt: "NEGATIVE",
    apiKey: "test-api-key",
    fetchImpl: async (url, options = {}) => {
      requests.push({ url, options });

      if (url.includes("/v1/images/edits")) {
        assert.equal(options.method, "POST");
        assert.equal(url, "http://localhost:18080/v1/images/edits");
        assert.equal(
          options.headers.Authorization,
          "Bearer test-api-key"
        );
        assert.ok(options.body instanceof FormData);
        assert.equal(options.body.get("model"), "gpt-image-2");
        assert.equal(options.body.get("prompt"), "POSITIVE");
        assert.equal(options.body.get("negative_prompt"), "NEGATIVE");
        assert.ok(options.body.get("image") instanceof Blob);
        assert.ok(options.body.get("mask") instanceof Blob);
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({
            data: [{ b64_json: pngBytes.toString("base64"), revised_prompt: "server revised prompt" }]
          })
        };
      }

      throw new Error(`unexpected url: ${url}`);
    }
  });

  assert.equal(requests.length, 1);
  assert.match(requests[0].url, /\/v1\/images\/edits$/);
  assert.equal(requests[0].options.method, "POST");
  assert.equal(result.revisedPrompt, "server revised prompt");
  assert.equal(result.imageDataUrl, `data:image/png;base64,${pngBytes.toString("base64")}`);
  assert.equal(result.pipeline.templateUsed, true);
  assert.equal(result.pipeline.referenceImageCount, 1);
  assert.equal(result.pipeline.requestImageCount, 1);
  assert.equal(result.pipeline.uploadContextMode, "single-mask-preview");
  assert.equal(result.pipeline.positivePrompt, "POSITIVE");
  assert.equal(result.pipeline.negativePrompt, "NEGATIVE");
  assert.match(result.pipeline.squarePreviewDataUrl, /^data:image\/png;base64,/);
  assert.match(result.pipeline.squarePreviewUploadDataUrl, /^data:image\/png;base64,/);
  assert.match(result.pipeline.editMaskDataUrl, /^data:image\/png;base64,/);
  assert.equal(result.pipeline.editMode, "image2-edits-mask");
  assert.match(result.pipeline.debugOverlayDataUrl, /^data:image\/png;base64,/);
  assert.match(result.pipeline.collagePreviewDataUrl, /^data:image\/png;base64,/);
});
