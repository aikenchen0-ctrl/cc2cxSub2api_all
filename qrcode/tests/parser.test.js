import test from "node:test";
import assert from "node:assert/strict";

import {
  extractImageCandidates,
  extractConversationText
} from "../src/parser.js";

test("extractImageCandidates should collect local paths, image urls, placeholders, and generated image paths", () => {
  const chatLog = `
用户
请处理图片
C:\\Users\\Administrator\\Desktop\\cover.png
https://example.com/asset/photo.jpg
<image name=[Image #1]>
</image>
Generated images are saved to C:\\Users\\Administrator\\.codex\\generated_images\\abc\\_image_id_.png by default.
  `.trim();

  const result = extractImageCandidates(chatLog);

  assert.deepEqual(result.localPaths, [
    "C:\\Users\\Administrator\\Desktop\\cover.png",
    "C:\\Users\\Administrator\\.codex\\generated_images\\abc\\_image_id_.png"
  ]);
  assert.deepEqual(result.remoteUrls, [
    "https://example.com/asset/photo.jpg"
  ]);
  assert.deepEqual(result.placeholders, ["Image #1"]);
});

test("extractImageCandidates should collect POSIX local image paths", () => {
  const chatLog = `
用户
请处理这张 Linux 临时目录图片
/tmp/chatlog-script-a1b2c3/mood.png
  `.trim();

  const result = extractImageCandidates(chatLog);

  assert.deepEqual(result.localPaths, [
    "/tmp/chatlog-script-a1b2c3/mood.png"
  ]);
});

test("extractConversationText should remove low-value image wrappers and preserve meaningful text", () => {
  const chatLog = `
用户
把这张图做成脚本
<image name=[Image #2]>
</image>
模型
请补充材质描述
  `.trim();

  const result = extractConversationText(chatLog);

  assert.equal(result.includes("把这张图做成脚本"), true);
  assert.equal(result.includes("请补充材质描述"), true);
  assert.equal(result.includes("<image"), false);
  assert.equal(result.includes("</image>"), false);
});
