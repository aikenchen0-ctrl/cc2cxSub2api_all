import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { analyzeChatLog } from "../src/analyze-chat.js";

test("analyzeChatLog should return extracted summary and model output", async () => {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "chatlog-script-"));
  const imagePath = path.join(tempDir, "mood.png");

  await fs.writeFile(imagePath, Buffer.from("image-bits"));

  const result = await analyzeChatLog({
    chatLog: `用户\n请整理这段对话\n${imagePath}\n<image name=[Image #2]>\n</image>`,
    callModel: async (payload) => {
      return {
        outputText: "最终脚本内容",
        payload
      };
    }
  });

  assert.equal(result.imageCount, 1);
  assert.deepEqual(result.placeholders, ["Image #2"]);
  assert.match(result.cleanedText, /请整理这段对话/);
  assert.equal(result.aiResult, "最终脚本内容");
  assert.equal(result.requestPayload.input[0].content.length, 2);
});
