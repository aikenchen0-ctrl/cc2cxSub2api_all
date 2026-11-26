import fs from "node:fs/promises";
import { FileBlob, SpreadsheetFile } from "@oai/artifact-tool";

const source = await FileBlob.load("outputs/当前上游.xlsx");
const wb = await SpreadsheetFile.importXlsx(source);
const sheet = wb.worksheets.getItem("当前上游");
const values = sheet.getRange("A3:D311").values;

const rules = [
  ["OpenAI 文本", /^(gpt-(?!image)|codex|o[1345](?:[-.]|$))/i],
  ["Claude 文本", /^claude/i],
  ["Gemini 文本", /^gemini/i],
  ["Grok 文本", /^grok(?!.*image)/i],
  ["DeepSeek", /^deepseek/i],
  ["智谱 GLM", /^(glm|chatglm)/i],
  ["Kimi", /^kimi/i],
  ["MiniMax", /^minimax/i],
  ["国内综合", /^(qwen|doubao|step|mimo|longcat|hunyuan|spark|yi[-_]|intern|nemotron|dots|composer|sensenova|agnes|t\d|hy\d|firefly)/i],
  ["图片兼容", /(image|seedream|nano[- ]?banana|banana|flux|sd(?:[-.]|\d)|firefly|recraft|imagine)/i],
  ["视频兼容", /(seedance|veo|runway|kling|sora|wan(?:\d|[-.]|$)|vidu|video|muse[-_]?video|hailuo|pika|hyvideo|luma)/i],
  ["音频兼容", /(audio|tts|whisper|speech|realtime)/i],
];
const rows = values.map(r => ({site:String(r[0]??""), link:String(r[1]??""), key:String(r[2]??""), text:String(r[3]??"")})).filter(r=>r.site && r.key);
const unavailable = r => r.text.startsWith("请求失败") || r.text.startsWith("请求异常") || r.text.startsWith("请求超时") || r.text.startsWith("响应成功但未找到模型字段") || r.text.startsWith("响应成功但不是 JSON");
const blocks = [];
let fallbackCount = 0;
for (const r of rows) {
  const models = r.text.split(/\r?\n/).map(x=>x.trim()).filter(Boolean);
  let groups = [];
  if (unavailable(r)) {
    groups = ["待复核"]; 
  } else {
    groups = rules.filter(([,re]) => models.some(m => re.test(m))).map(([name])=>name);
    if (!groups.length) { groups=["通用文本"]; fallbackCount++; }
  }
  blocks.push(`${r.site}\n${r.key}\n${groups.join("、")}`);
}
await fs.writeFile("分组.txt", blocks.join("\n\n\n\n"), "utf8");
console.log(JSON.stringify({blocks:blocks.length,fallbackCount,unavailable:rows.filter(unavailable).length,groups:[...new Set(blocks.flatMap(b=>b.split("\n")[2].split("、")))]}));
