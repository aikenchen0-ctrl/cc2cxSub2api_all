import test from "node:test";
import assert from "node:assert/strict";

import { buildQrEditPayload } from "../src/qr-payload.js";

test("buildQrEditPayload should create a fixed gpt-image request body", () => {
  const payload = buildQrEditPayload({
    images: [
      "data:image/png;base64,AAA",
      "data:image/png;base64,BBB"
    ],
    positivePrompt: "POSITIVE",
    negativePrompt: "NEGATIVE"
  });

  assert.equal(payload.model, "image2");
  assert.equal(payload.size, "1024x1024");
  assert.equal(payload.image.length, 2);
  assert.equal(payload.image[0], "data:image/png;base64,AAA");
  assert.equal(payload.image[1], "data:image/png;base64,BBB");
  assert.equal(payload.prompt, "POSITIVE");
  assert.equal(payload.negative_prompt, "NEGATIVE");
});
