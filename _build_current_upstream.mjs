import fs from "node:fs/promises";
import { SpreadsheetFile, Workbook } from "@oai/artifact-tool";

const outputDir = "outputs";
await fs.mkdir(outputDir, { recursive: true });
const rows = JSON.parse(await fs.readFile("_model_results.json", "utf8"));
const workbook = Workbook.create();
const sheet = workbook.worksheets.add("当前上游");
sheet.showGridLines = false;

sheet.getRange("A1:D1").merge();
sheet.getRange("A1").values = [["当前上游模型列表"]];
sheet.getRange("A2:D2").values = [["标题", "链接", "key", "模型列表"]];
const values = rows.map(r => [r.title, r.link, r.key, r.models]);
sheet.getRangeByIndexes(2, 0, values.length, 4).values = values;

sheet.getRange("A1:D1").format = {
  font: { name: "Arial", size: 14, bold: true, color: "#1F2937" },
  verticalAlignment: "center",
};
sheet.getRange("A1:D1").format.rowHeight = 28;
sheet.getRange("A2:D2").format = {
  fill: "#1F4E78",
  font: { name: "Arial", size: 10, bold: true, color: "#FFFFFF" },
  horizontalAlignment: "center",
  verticalAlignment: "center",
  wrapText: true,
  borders: { preset: "outside", style: "thin", color: "#9CA3AF" },
};
sheet.getRange("A3:D311").format = {
  font: { name: "Arial", size: 10, color: "#111827" },
  verticalAlignment: "center",
  wrapText: true,
};
sheet.getRange("A3:C311").format.horizontalAlignment = "left";
sheet.getRange("D3:D311").format.horizontalAlignment = "left";
sheet.getRange("A3:D311").format.borders = {
  insideHorizontal: { style: "thin", color: "#E5E7EB" },
  bottom: { style: "thin", color: "#E5E7EB" },
};
sheet.getRange("A:A").format.columnWidth = 16;
sheet.getRange("B:B").format.columnWidth = 28;
sheet.getRange("C:C").format.columnWidth = 52;
sheet.getRange("C3:C311").format.wrapText = false;
sheet.getRange("D:D").format.columnWidth = 70;
sheet.getRange("A2:D311").format.rowHeight = 18;
rows.forEach((r, i) => {
  const lineCount = String(r.models ?? "").split("\n").length;
  const height = Math.min(360, Math.max(20, 16 + lineCount * 14));
  sheet.getRange(`A${i + 3}:D${i + 3}`).format.rowHeight = height;
});
sheet.freezePanes.freezeRows(2);
const table = sheet.tables.add("A2:D311", true, "CurrentUpstreamModels");
table.style = "TableStyleMedium2";
table.showFilterButton = true;
table.showBandedRows = true;

workbook.recalculate();
const inspect = await workbook.inspect({ kind: "table", range: "当前上游!A1:D8", include: "values,formulas", tableMaxRows: 8, tableMaxCols: 4 });
console.log(inspect.ndjson);
const errors = await workbook.inspect({ kind: "match", searchTerm: "#REF!|#DIV/0!|#VALUE!|#NAME\\?|#N/A|#NUM!|#NULL!|#SPILL!|#CALC!", options: { useRegex: true, maxResults: 50 }, summary: "final formula error scan" });
console.log(errors.ndjson);
const preview = await workbook.render({ sheetName: "当前上游", range: "A1:D12", scale: 1, format: "png" });
await fs.writeFile(`${outputDir}/当前上游_preview.png`, new Uint8Array(await preview.arrayBuffer()));
const xlsx = await SpreadsheetFile.exportXlsx(workbook);
await xlsx.save(`${outputDir}/当前上游.xlsx`);
console.log(`EXPORTED ${outputDir}/当前上游.xlsx`);


