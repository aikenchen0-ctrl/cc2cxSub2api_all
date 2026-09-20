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
