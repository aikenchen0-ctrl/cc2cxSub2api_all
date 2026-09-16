import fs from "node:fs/promises";

import { analyzeChatLog } from "./analyze-chat.js";

export async function analyzeChatFile({ chatFilePath, callModel }) {
  const chatLog = await fs.readFile(chatFilePath, "utf8");

  return analyzeChatLog({
    chatLog,
    callModel
  });
}
