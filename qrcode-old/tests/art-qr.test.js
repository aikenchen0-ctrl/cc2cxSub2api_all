import test from "node:test";
import assert from "node:assert/strict";
import crypto from "node:crypto";
import sharp from "sharp";

import { stylizeQrImage } from "../src/art-qr.js";
import { APP_CONFIG } from "../src/config.js";

async function createQrBuffer() {
  return sharp({
    create: {
      width: 32,
      height: 32,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  }).png().toBuffer();
}

test("stylizeQrImage should submit the configured porcelain art QR payload", async () => {
  const imageBuffer = await createQrBuffer();
  const responseImage = await createQrBuffer();
  let submitBody = null;

  await stylizeQrImage({
    imageBuffer,
    qrTrim: { enabled: false },
    fetchImpl: async (url, options = {}) => {
      if (url.endsWith(APP_CONFIG.artQr.generateEndpoint)) {
        submitBody = JSON.parse(options.body);
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ code: 0, data: { img_uuid: "task-1" } })
        };
      }

      if (url.includes(APP_CONFIG.artQr.detailEndpoint)) {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({
            code: 0,
            data: {
              status: 1,
              urls: ["https://example.com/result.png"],
              duration: 1,
              cost: 1
            }
          })
        };
      }

      if (url === "https://example.com/result.png") {
        return {
          ok: true,
          headers: new Headers({ "content-type": "image/png" }),
          arrayBuffer: async () => responseImage.buffer.slice(
            responseImage.byteOffset,
            responseImage.byteOffset + responseImage.byteLength
          )
        };
      }

      throw new Error(`unexpected url: ${url}`);
    }
  });

  assert.equal(submitBody.model, "67");
  assert.equal(submitBody.prompt, APP_CONFIG.artQr.defaultPrompt);
  assert.equal(submitBody.negative_prompt, APP_CONFIG.artQr.negativePrompt);
  assert.equal(Object.hasOwn(submitBody, "qr_content"), false);
  assert.equal(submitBody.callback_url, APP_CONFIG.artQr.callbackUrl);
  assert.equal(typeof submitBody.qr_image, "string");
});

test("stylizeQrImage should retry transient non JSON submit timeouts", async () => {
  const imageBuffer = await createQrBuffer();
  const responseImage = await createQrBuffer();
  let submitCount = 0;

  const result = await stylizeQrImage({
    imageBuffer,
    qrTrim: { enabled: false },
    submitRetryDelayMs: 0,
    fetchImpl: async (url) => {
      if (url.endsWith(APP_CONFIG.artQr.generateEndpoint)) {
        submitCount += 1;
        if (submitCount === 1) {
          return {
            ok: false,
            status: 504,
            text: async () => "Invoking task timed out after 10 seconds"
          };
        }

        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ code: 0, data: { img_uuid: "task-retry" } })
        };
      }

      if (url.includes(APP_CONFIG.artQr.detailEndpoint)) {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({
            code: 0,
            data: {
              status: 1,
              urls: ["https://example.com/retry-result.png"],
              duration: 1,
              cost: 1
            }
          })
        };
      }

      if (url === "https://example.com/retry-result.png") {
        return {
          ok: true,
          headers: new Headers({ "content-type": "image/png" }),
          arrayBuffer: async () => responseImage.buffer.slice(
            responseImage.byteOffset,
            responseImage.byteOffset + responseImage.byteLength
          )
        };
      }

      throw new Error(`unexpected url: ${url}`);
    }
  });

  assert.equal(submitCount, 2);
  assert.equal(result.task.imgUuid, "task-retry");
});

test("stylizeQrImage should use the cropped QR image as qr_image", async () => {
  const imageBuffer = await sharp({
    create: {
      width: 32,
      height: 32,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  })
    .composite([
      {
        input: Buffer.from(`
          <svg width="32" height="32" xmlns="http://www.w3.org/2000/svg">
            <rect x="8" y="8" width="16" height="16" fill="black" />
          </svg>
        `),
        left: 0,
        top: 0
      }
    ])
    .png()
    .toBuffer();
  const responseImage = await createQrBuffer();
  let submittedQrImage = null;

  await stylizeQrImage({
    imageBuffer,
    qrTrim: {
      enabled: true,
      mode: "manual",
      x: 8,
      y: 8,
      width: 16,
      height: 16
    },
    fetchImpl: async (url, options = {}) => {
      if (url.endsWith(APP_CONFIG.artQr.generateEndpoint)) {
        submittedQrImage = JSON.parse(options.body).qr_image;
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ code: 0, data: { img_uuid: "task-2" } })
        };
      }

      if (url.includes(APP_CONFIG.artQr.detailEndpoint)) {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({
            code: 0,
            data: {
              status: 1,
              urls: ["https://example.com/result.png"],
              duration: 1,
              cost: 1
            }
          })
        };
      }

      if (url === "https://example.com/result.png") {
        return {
          ok: true,
          headers: new Headers({ "content-type": "image/png" }),
          arrayBuffer: async () => responseImage.buffer.slice(
            responseImage.byteOffset,
            responseImage.byteOffset + responseImage.byteLength
          )
        };
      }

      throw new Error(`unexpected url: ${url}`);
    }
  });

  assert.equal(submittedQrImage.startsWith("data:"), false);
  const metadata = await sharp(Buffer.from(submittedQrImage, "base64")).metadata();
  assert.equal(metadata.format, "jpeg");
  assert.equal(metadata.width, 16);
  assert.equal(metadata.height, 16);
});

test("stylizeQrImage should submit a different qr_image for each uploaded QR image", async () => {
  const createInput = async (x) => sharp({
    create: {
      width: 64,
      height: 64,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  })
    .composite([
      {
        input: Buffer.from(`
          <svg width="64" height="64" xmlns="http://www.w3.org/2000/svg">
            <rect x="${x}" y="8" width="12" height="12" fill="black" />
            <rect x="40" y="40" width="12" height="12" fill="black" />
          </svg>
        `),
        left: 0,
        top: 0
      }
    ])
    .png()
    .toBuffer();
  const responseImage = await createQrBuffer();
  const submitted = [];

  const fetchImpl = async (url, options = {}) => {
    if (url.endsWith(APP_CONFIG.artQr.generateEndpoint)) {
      submitted.push(JSON.parse(options.body).qr_image);
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ code: 0, data: { img_uuid: `task-${submitted.length}` } })
      };
    }

    if (url.includes(APP_CONFIG.artQr.detailEndpoint)) {
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          code: 0,
          data: {
            status: 1,
            urls: ["https://example.com/result.png"],
            duration: 1,
            cost: 1
          }
        })
      };
    }

    if (url === "https://example.com/result.png") {
      return {
        ok: true,
        headers: new Headers({ "content-type": "image/png" }),
        arrayBuffer: async () => responseImage.buffer.slice(
          responseImage.byteOffset,
          responseImage.byteOffset + responseImage.byteLength
        )
      };
    }

    throw new Error(`unexpected url: ${url}`);
  };

  const first = await stylizeQrImage({
    imageBuffer: await createInput(8),
    qrTrim: { enabled: false },
    fetchImpl
  });
  const second = await stylizeQrImage({
    imageBuffer: await createInput(24),
    qrTrim: { enabled: false },
    fetchImpl
  });

  assert.notEqual(submitted[0], submitted[1]);
  assert.notEqual(first.input.qrImageSha256, second.input.qrImageSha256);
  assert.equal(
    first.input.qrImageSha256,
    crypto.createHash("sha256").update(Buffer.from(submitted[0], "base64")).digest("hex")
  );
});
