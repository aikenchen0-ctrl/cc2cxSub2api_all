import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { analyzeChatFile } from "../src/analyze-chat-file.js";

test("analyzeChatFile should read the txt file and return the same analysis shape", async () => {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "chatlog-file-"));
  const imagePath = path.join(tempDir, "scene.png");
  const chatFilePath = path.join(tempDir, "chat.txt");

  await fs.writeFile(imagePath, Buffer.from("image"));
  await fs.writeFile(
    chatFilePath,
    `用户\n请把这个聊天记录整理成脚本\n${imagePath}\n<image name=[Image #1]>\n</image>`
  );

  const result = await analyzeChatFile({
    chatFilePath,
    callModel: async () => ({ outputText: "整理后的脚本" })
  });

  assert.equal(result.aiResult, "整理后的脚本");
  assert.equal(result.imageCount, 1);
  assert.deepEqual(result.placeholders, ["Image #1"]);
  assert.match(result.cleanedText, /整理成脚本/);
});
