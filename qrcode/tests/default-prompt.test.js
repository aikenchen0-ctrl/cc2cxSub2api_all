import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import { APP_CONFIG } from "../src/config.js";

const REQUIRED_POSITIVE_PROMPT_SUFFIX = "把正方形的效果明显的边边角角也要融合到延展中融为一体。";

test("默认正向 Prompt 应包含正方形边角融合要求", async () => {
  const publicApp = await readFile(new URL("../public/app.js", import.meta.url), "utf8");

  assert.match(APP_CONFIG.defaultPositivePrompt, new RegExp(REQUIRED_POSITIVE_PROMPT_SUFFIX));
  assert.match(publicApp, new RegExp(REQUIRED_POSITIVE_PROMPT_SUFFIX));
});
