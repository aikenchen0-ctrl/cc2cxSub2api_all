import test from "node:test";
import assert from "node:assert/strict";

import { buildResponsesPayload } from "../src/payload.js";

test("buildResponsesPayload should assemble fixed instructions, chat text, and image inputs", () => {
  const payload = buildResponsesPayload({
    chatText: "请整理这个对话",
    imageDataUrls: [
      "data:image/png;base64,AAA",
      "data:image/jpeg;base64,BBB"
    ]
  });

  assert.equal(payload.model, "gpt-4.1-mini");
  assert.equal(Array.isArray(payload.input), true);
  assert.equal(payload.input.length, 1);
  assert.equal(payload.input[0].role, "user");
  assert.equal(payload.input[0].content[0].type, "input_text");
  assert.equal(payload.input[0].content[1].type, "input_image");
  assert.equal(payload.input[0].content[2].type, "input_image");
  assert.match(payload.input[0].content[0].text, /整理为可执行脚本/);
  assert.match(payload.input[0].content[0].text, /请整理这个对话/);
});
