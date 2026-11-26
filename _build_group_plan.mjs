import fs from "node:fs/promises";
import { FileBlob, SpreadsheetFile, Workbook } from "@oai/artifact-tool";

const source = await FileBlob.load("outputs/当前上游.xlsx");
const sourceWb = await SpreadsheetFile.importXlsx(source);
const sourceSheet = sourceWb.worksheets.getItem("当前上游");
const sourceValues = sourceSheet.getRange("A3:D311").values;

const groups = [
  // The repository's routing rules default unverified model names to an
  // OpenAI-compatible profile. Only a model explicitly carrying "grok" is
  // assigned to the Grok profile. These are logical model groups, not claims
  // that the upstream natively implements that vendor protocol.
  { name: "OpenAI 文本", type: "openai", use: "GPT / Codex 文本对话", basis: "模型名前缀匹配；按 OpenAI-compatible 默认规则", re: /^(gpt-(?!image)|codex|o[1345](?:[-.]|$))/i },
  { name: "Claude 文本", type: "openai", use: "Claude 模型文本对话", basis: "模型名前缀匹配；未验证 /v1/messages，先按 OpenAI-compatible", re: /^claude/i },
  { name: "Gemini 文本", type: "openai", use: "Gemini 模型文本与多模态对话", basis: "模型名前缀匹配；未验证原生 Gemini 协议，先按 OpenAI-compatible", re: /^gemini/i },
  { name: "Grok 文本", type: "grok", use: "Grok 文本对话", basis: "模型名明确含 grok，按仓库规则使用 Grok profile", re: /^grok(?!.*image)/i },
  { name: "DeepSeek", type: "openai", use: "DeepSeek 模型文本对话", basis: "模型名前缀匹配；未验证原生端点，先按 OpenAI-compatible", re: /^deepseek/i },
  { name: "智谱 GLM", type: "openai", use: "GLM 模型文本对话", basis: "模型名前缀匹配；未验证原生端点，先按 OpenAI-compatible", re: /^(glm|chatglm)/i },
  { name: "Kimi", type: "openai", use: "Kimi 模型文本对话", basis: "模型名前缀匹配；未验证原生端点，先按 OpenAI-compatible", re: /^kimi/i },
  { name: "MiniMax", type: "openai", use: "MiniMax 模型文本对话", basis: "模型名前缀匹配；未验证原生端点，先按 OpenAI-compatible", re: /^minimax/i },
  { name: "国内综合", type: "openai", use: "Qwen / 豆包 / 阶跃 / Mimo 等兼容模型", basis: "模型名前缀匹配；按 OpenAI-compatible 默认规则", re: /^(qwen|doubao|step|mimo|longcat|hunyuan|spark|yi[-_]|intern|nemotron|dots|composer|sensenova|agnes|t\d|hy\d|firefly)/i },
  { name: "图片兼容", type: "openai", use: "图片生成 / 图片编辑（OpenAI-compatible）", basis: "媒体模型名匹配；真实图片端点和 profile 仍需实测", re: /(image|seedream|nano[- ]?banana|banana|flux|sd(?:[-.]|\d)|firefly|recraft|imagine)/i },
  { name: "视频兼容", type: "openai", use: "视频生成（OpenAI-compatible）", basis: "媒体模型名匹配；真实视频端点和异步协议仍需实测", re: /(seedance|veo|runway|kling|sora|wan(?:\d|[-.]|$)|vidu|video|muse[-_]?video|hailuo|pika|hyvideo|luma)/i },
  { name: "音频兼容", type: "openai", use: "语音 / 音频（OpenAI-compatible）", basis: "音频模型名匹配；真实音频端点仍需实测", re: /(audio|tts|whisper|speech|realtime)/i },
];

const parsed = sourceValues.map((r) => ({
  title: String(r[0] ?? ""), link: String(r[1] ?? ""), key: String(r[2] ?? ""), modelsText: String(r[3] ?? ""),
})).filter(r => r.title && r.key);
const modelRows = parsed.map(r => ({ ...r, models: r.modelsText.split(/\r?\n/).map(s => s.trim()).filter(Boolean) }));
const isError = r => r.modelsText.startsWith("请求失败") || r.modelsText.startsWith("请求异常") || r.modelsText.startsWith("请求超时");
const isEmpty = r => r.models.length === 0 || r.modelsText.startsWith("响应成功但未找到模型字段") || r.modelsText.startsWith("响应成功但不是 JSON");

const assignments = [];
for (const r of modelRows) {
  if (isError(r) || isEmpty(r)) continue;
  for (const g of groups) {
    const matched = r.models.filter(m => g.re.test(m));
    if (matched.length) assignments.push({ group: g.name, type: g.type, use: g.use, title: r.title, link: r.link, key: r.key, models: matched.join("\n"), modelCount: matched.length });
  }
}

const summary = groups.map(g => {
  const a = assignments.filter(x => x.group === g.name).sort((x,y) => y.modelCount - x.modelCount || x.title.localeCompare(y.title));
  const primary = a[0];
  const backups = a.slice(1, 4);
  const uniqueModels = [...new Set(a.flatMap(x => x.models.split(/\r?\n/)))];
  return {
    group: g.name, type: g.type, use: g.use, basis: g.basis,
    status: a.length ? "建议启用" : "暂无可用 key",
    primaryTitle: primary?.title ?? "", primaryLink: primary?.link ?? "", primaryKey: primary?.key ?? "",
    backupKeys: backups.map(x => `${x.title} | ${x.key}`).join("\n"),
    keyCount: a.length, modelCount: uniqueModels.length,
    models: uniqueModels.join("\n"), note: a.length ? "同一 key 可同时加入其他匹配分组；按模型白名单限制范围。" : "当前采集结果没有可用模型，不建议创建或启用。",
  };
});

const unavailable = modelRows.filter(isError || isEmpty).map(r => ({
  title: r.title, link: r.link, key: r.key,
  status: isError(r) ? "请求失败/异常" : "HTTP 200 但无模型",
  detail: r.modelsText,
}));

const wb = Workbook.create();
const s1 = wb.worksheets.add("分组汇总");
const s2 = wb.worksheets.add("Key分配明细");
const s3 = wb.worksheets.add("不可分配Key");
for (const s of [s1,s2,s3]) { s.showGridLines = false; }
const font = { name: "Arial", size: 10, color: "#111827" };
const header = { fill: "#1F4E78", font: { name: "Arial", size: 10, bold: true, color: "#FFFFFF" }, horizontalAlignment: "center", verticalAlignment: "center", wrapText: true };
const title = { font: { name: "Arial", size: 14, bold: true, color: "#1F2937" }, verticalAlignment: "center" };

s1.getRange("A1:K1").merge(); s1.getRange("A1").values = [["当前上游中转站分组建议（只读反馈，不修改数据库）"]]; s1.getRange("A1:K1").format = title; s1.getRange("A1:K1").format.rowHeight = 28;
s1.getRange("A2:K2").values = [["建议分组","建议平台类型","用途","类型依据","状态","主推荐上游","主推荐链接","主推荐 key","备选 key（最多 3 个）","可用 key 数","模型数"]]; s1.getRange("A2:K2").format = header;
s1.getRangeByIndexes(2,0,summary.length,11).values = summary.map(x=>[x.group,x.type,x.use,x.basis,x.status,x.primaryTitle,x.primaryLink,x.primaryKey,x.backupKeys,x.keyCount,x.modelCount]);
s1.getRange("A3:K14").format = { font, verticalAlignment:"center", wrapText:true, borders:{insideHorizontal:{style:"thin",color:"#E5E7EB"},bottom:{style:"thin",color:"#E5E7EB"}}};
s1.getRange("A:A").format.columnWidth=18; s1.getRange("B:B").format.columnWidth=16; s1.getRange("C:C").format.columnWidth=30; s1.getRange("D:D").format.columnWidth=45; s1.getRange("E:E").format.columnWidth=14; s1.getRange("F:F").format.columnWidth=16; s1.getRange("G:G").format.columnWidth=28; s1.getRange("H:H").format.columnWidth=44; s1.getRange("I:I").format.columnWidth=50; s1.getRange("J:K").format.columnWidth=12; s1.getRange("A2:K14").format.rowHeight=22;
summary.forEach((x,i)=>{ const h=Math.min(320,Math.max(22,20+String(x.backupKeys).split("\n").length*16)); s1.getRange(`A${i+3}:K${i+3}`).format.rowHeight=h; });
s1.freezePanes.freezeRows(2); const t1=s1.tables.add("A2:K14",true,"GroupPlanSummary"); t1.style="TableStyleMedium2";

s2.getRange("A1:I1").merge(); s2.getRange("A1").values = [["Key 分配明细（同一 key 可出现于多个分组）"]]; s2.getRange("A1:I1").format=title; s2.getRange("A1:I1").format.rowHeight=28;
s2.getRange("A2:J2").values=[["分组","建议平台类型","用途","类型依据","上游标题","链接","key","匹配模型","匹配模型数","建议"]]; s2.getRange("A2:J2").format=header;
s2.getRangeByIndexes(2,0,assignments.length,10).values=assignments.map(x=>[x.group,x.type,x.use,groups.find(g=>g.name===x.group)?.basis ?? "",x.title,x.link,x.key,x.models,x.modelCount,"加入该分组并设置模型白名单"]);
s2.getRangeByIndexes(2,0,assignments.length,10).format={font,verticalAlignment:"center",wrapText:true,borders:{insideHorizontal:{style:"thin",color:"#E5E7EB"},bottom:{style:"thin",color:"#E5E7EB"}}};
s2.getRange("A:A").format.columnWidth=18; s2.getRange("B:B").format.columnWidth=16; s2.getRange("C:C").format.columnWidth=30; s2.getRange("D:D").format.columnWidth=45; s2.getRange("E:E").format.columnWidth=16; s2.getRange("F:F").format.columnWidth=28; s2.getRange("G:G").format.columnWidth=44; s2.getRange("H:H").format.columnWidth=48; s2.getRange("I:I").format.columnWidth=12; s2.getRange("J:J").format.columnWidth=28; s2.getRange(`A2:J${assignments.length+2}`).format.rowHeight=22;
assignments.forEach((x,i)=>{ const h=Math.min(260,Math.max(22,20+String(x.models).split("\n").length*14)); s2.getRange(`A${i+3}:J${i+3}`).format.rowHeight=h; });
s2.freezePanes.freezeRows(2); const t2=s2.tables.add(`A2:J${assignments.length+2}`,true,"KeyGroupAssignments"); t2.style="TableStyleMedium2";

s3.getRange("A1:E1").merge(); s3.getRange("A1").values = [["不可分配 Key（采集失败、超时或无模型）"]]; s3.getRange("A1:E1").format=title; s3.getRange("A1:E1").format.rowHeight=28;
s3.getRange("A2:E2").values=[["上游标题","链接","key","状态","详情"]]; s3.getRange("A2:E2").format=header;
s3.getRangeByIndexes(2,0,unavailable.length,5).values=unavailable.map(x=>[x.title,x.link,x.key,x.status,x.detail]);
s3.getRangeByIndexes(2,0,unavailable.length,5).format={font,verticalAlignment:"center",wrapText:true,borders:{insideHorizontal:{style:"thin",color:"#E5E7EB"},bottom:{style:"thin",color:"#E5E7EB"}}};
s3.getRange("A:A").format.columnWidth=16; s3.getRange("B:B").format.columnWidth=28; s3.getRange("C:C").format.columnWidth=44; s3.getRange("D:D").format.columnWidth=18; s3.getRange("E:E").format.columnWidth=60; s3.getRange(`A2:E${unavailable.length+2}`).format.rowHeight=24;
s3.freezePanes.freezeRows(2); const t3=s3.tables.add(`A2:E${unavailable.length+2}`,true,"UnavailableKeys"); t3.style="TableStyleMedium2";

wb.recalculate();
for (const [name,range] of [["分组汇总","A1:K14"],["Key分配明细",`A1:J${Math.min(assignments.length+2,12)}`],["不可分配Key",`A1:E${Math.min(unavailable.length+2,12)}`]]) { const p=await wb.render({sheetName:name,range,scale:1,format:"png"}); await fs.writeFile(`outputs/${name}_preview.png`,new Uint8Array(await p.arrayBuffer())); }
const xlsx=await SpreadsheetFile.exportXlsx(wb); await xlsx.save("outputs/当前上游分组建议.xlsx");
console.log(JSON.stringify({parsed:parsed.length,assignments:assignments.length,unavailable:unavailable.length,summary:summary.map(x=>({group:x.group,keys:x.keyCount,models:x.modelCount,primary:x.primaryTitle}))}));
