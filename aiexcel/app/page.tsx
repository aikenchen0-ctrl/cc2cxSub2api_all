"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import ExcelJS from "exceljs";
import * as XLSX from "xlsx";
import {
  ArrowLeft, ArrowUpDown, BarChart3, Bot, Check, Clock3, Download, FileSpreadsheet, Filter,
  History, LoaderCircle, Menu, Plus, Redo2, Search, Send,
  Settings, ShieldCheck, Sparkles, Undo2, UploadCloud, X, ZoomIn, ZoomOut
} from "lucide-react";

type ExcelValue = string | number | boolean | Date | null;
type GridCell = { value: ExcelValue; formula?: string; style?: Partial<ExcelJS.Style> };
type SheetView = { name: string; rows: GridCell[][]; rowCount: number; colCount: number; rowOffset: number; filterHeaderRow?: number; hiddenRows: number[] };
type Operation = {
  type: "add_formula" | "set_formula" | "style_range" | "conditional_format" | "create_analysis" | "beautify" | "find_replace"
    | "delete_rows" | "insert_rows" | "set_cells" | "delete_columns" | "add_column" | "fill_blank"
    | "enable_filter" | "sort_rows" | "filter_rows" | "clear_filter" | "highlight_rows" | "create_dashboard";
  sheet?: string; column?: string; header?: string; formula?: string; range?: string;
  style?: Record<string, unknown>; condition?: string; title?: string; sourceSheet?: string;
  find?: string; replace?: string;
  startRow?: number; count?: number; startColumn?: number;
  values?: ExcelValue[] | ExcelValue[][]; cells?: Array<{ cell: string; value: ExcelValue; formula?: string }>;
  columns?: string[]; headerRow?: number; startDataRow?: number; endRow?: number; value?: ExcelValue;
  order?: "asc" | "desc"; operator?: "equals" | "not_equals" | "contains" | "greater_than" | "less_than" | "between" | "not_blank" | "blank";
  filterValue?: ExcelValue; min?: number; max?: number;
  matchValue?: ExcelValue; backgroundColor?: string; fontColor?: string;
  groupByColumn?: string; valueColumn?: string; sheetName?: string;
};
type Plan = { title: string; summary: string; operations: Operation[]; warnings?: string[] };
type ChatMessage = { role: "user" | "assistant"; text: string; plan?: Plan; timestamp: number };
type HistoryEntry = { title: string; timestamp: number; buffer: ArrayBuffer };
type DashboardRow = { group: string; count: number; average: number; min: number; max: number };
type DashboardData = { groupLabel: string; valueLabel: string; sourceSheet: string; rows: DashboardRow[] };

const COLORS = { green: "167D5A", mint: "DCF3E9", ink: "172621", amber: "ECA72C", red: "E95D4E", blue: "3975C6" };

function columnLetter(n: number) {
  let s = "";
  while (n > 0) { n--; s = String.fromCharCode(65 + (n % 26)) + s; n = Math.floor(n / 26); }
  return s;
}

function columnNumber(reference: string) {
  const letters = reference.trim().toUpperCase().replace(/\$/g, "");
  if (!/^[A-Z]{1,3}$/.test(letters)) return 0;
  return [...letters].reduce((value, letter) => value * 26 + letter.charCodeAt(0) - 64, 0);
}

function displayCell(cell?: GridCell) {
  if (!cell) return "";
  if (cell.formula) return `=${cell.formula}`;
  if (cell.value instanceof Date) return cell.value.toLocaleDateString("zh-CN");
  if (typeof cell.value === "object" && cell.value !== null) return JSON.stringify(cell.value);
  return cell.value == null ? "" : String(cell.value);
}

function cellCss(cell?: GridCell): CSSProperties {
  const style = cell?.style;
  const fill = style?.fill;
  const font = style?.font;
  const fillColor = fill && "fgColor" in fill ? fill.fgColor?.argb : undefined;
  const fontColor = font?.color && "argb" in font.color ? font.color.argb : undefined;
  return {
    backgroundColor: fillColor ? `#${fillColor.slice(-6)}` : undefined,
    color: fontColor ? `#${fontColor.slice(-6)}` : undefined,
    fontWeight: font?.bold ? 700 : undefined
  };
}

function getWorkbookSchema(sheets: SheetView[]) {
  return sheets.map(s => ({
    name: s.name, rows: s.rowCount, columns: s.rows[0]?.map((c, i) => displayCell(c) || columnLetter(i + 1)) ?? [],
    headerCandidates: s.rows.slice(0, 10).map((row, index) => ({
      row: index + 1,
      values: row.map(displayCell)
    })),
    sample: s.rows.slice(0, 8).map(row => row.map(displayCell))
  }));
}

const PREVIEW_PAGE_SIZE = 500;

function workbookToViews(workbook: ExcelJS.Workbook, rowOffset = 0): SheetView[] {
  return workbook.worksheets.map(ws => {
    const rows: GridCell[][] = [];
    const hiddenRows: number[] = [];
    const maxCols = Math.min(Math.max(ws.columnCount, 1), 80);
    const firstRow = Math.min(Math.max(1, rowOffset + 1), Math.max(ws.rowCount, 1));
    const lastRow = Math.min(Math.max(ws.rowCount, 1), firstRow + PREVIEW_PAGE_SIZE - 1);
    for (let r = firstRow; r <= lastRow; r++) {
      if (ws.getRow(r).hidden) hiddenRows.push(r);
      const row: GridCell[] = [];
      for (let c = 1; c <= maxCols; c++) {
        const cell = ws.getCell(r, c);
        const raw = cell.value as ExcelJS.CellValue;
        if (raw && typeof raw === "object" && "formula" in raw) {
          row.push({ value: (raw.result as ExcelValue) ?? null, formula: String(raw.formula), style: cell.style });
        } else {
          row.push({ value: raw as ExcelValue, style: cell.style });
        }
      }
      rows.push(row);
    }
    const autoFilter = ws.autoFilter;
    let filterHeaderRow: number | undefined;
    if (typeof autoFilter === "string") {
      const match = autoFilter.match(/\$?[A-Z]+\$?(\d+)/i);
      filterHeaderRow = match ? Number(match[1]) : undefined;
    } else if (autoFilter && typeof autoFilter === "object" && "from" in autoFilter) {
      const from = autoFilter.from;
      if (typeof from === "string") {
        const match = from.match(/\$?[A-Z]+\$?(\d+)/i);
        filterHeaderRow = match ? Number(match[1]) : undefined;
      } else filterHeaderRow = from.row;
    }
    return { name: ws.name, rows, rowCount: ws.rowCount, colCount: ws.columnCount, rowOffset: firstRow - 1, filterHeaderRow, hiddenRows };
  });
}

function resolveFormula(formula: string, row: number) {
  return formula.replace(/^=/, "").replace(/\{row\}/gi, String(row)).replace(/([A-Z]+)2\b/g, `$1${row}`);
}

function findHeaderColumn(ws: ExcelJS.Worksheet, headerRow: number, name?: string) {
  const wanted = String(name || "").trim();
  let found = 0;
  ws.getRow(headerRow).eachCell({ includeEmpty: true }, (cell, col) => {
    if (String(cell.text || cell.value || "").trim() === wanted) found = col;
  });
  return found;
}

function comparable(value: ExcelJS.CellValue) {
  const raw = value && typeof value === "object" && "result" in value ? value.result : value;
  if (raw instanceof Date) return raw.getTime();
  if (typeof raw === "number") return raw;
  const text = String(raw ?? "").trim();
  const numeric = Number(text.replace(/[￥¥,$,\s]/g, ""));
  return text !== "" && Number.isFinite(numeric) ? numeric : text.toLocaleLowerCase("zh-CN");
}

function parseCellNumber(cell: ExcelJS.Cell) {
  const raw = cell.value;
  const resolved = raw && typeof raw === "object" && "result" in raw ? raw.result : raw;
  if (typeof resolved === "number" && Number.isFinite(resolved)) return resolved;
  const text = cell.text
    .replace(/[，,￥¥$€£\s]/g, "")
    .replace(/[—–至~～]/g, "-")
    .trim();
  if (!text || /^(无|暂无|未知|不详|面议|—|-)$/i.test(text)) return null;
  const matches = [...text.matchAll(/\d+(?:\.\d+)?/g)].map(match => Number(match[0])).filter(Number.isFinite);
  if (!matches.length) return null;
  let value = matches.length >= 2 && /[-]/.test(text) ? (matches[0] + matches[1]) / 2 : matches[0];
  if (/万/.test(text)) value *= 10000;
  if (/千/.test(text) && !/平方米|㎡/.test(text)) value *= 1000;
  return Number.isFinite(value) ? value : null;
}

function makeBarChart(labels: string[], values: number[], title: string) {
  const canvas = document.createElement("canvas");
  canvas.width = 1120; canvas.height = 560;
  const ctx = canvas.getContext("2d");
  if (!ctx) return "";
  ctx.fillStyle = "#FFFFFF"; ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#172621"; ctx.font = "bold 28px sans-serif"; ctx.fillText(title, 48, 48);
  ctx.fillStyle = "#718079"; ctx.font = "15px sans-serif"; ctx.fillText("由 AI表格 本地生成", 48, 76);
  const left = 76, top = 110, width = 990, height = 360;
  ctx.strokeStyle = "#DDE7E2"; ctx.lineWidth = 1;
  for (let i = 0; i <= 4; i++) {
    const y = top + height * i / 4; ctx.beginPath(); ctx.moveTo(left, y); ctx.lineTo(left + width, y); ctx.stroke();
  }
  const max = Math.max(...values, 1);
  const shown = labels.slice(0, 16);
  const gap = width / Math.max(shown.length, 1);
  shown.forEach((label, i) => {
    const value = values[i] || 0; const barWidth = Math.max(12, gap * .58);
    const barHeight = value / max * (height - 20); const x = left + gap * i + (gap - barWidth) / 2; const y = top + height - barHeight;
    ctx.fillStyle = "#167D5A"; ctx.fillRect(x, y, barWidth, barHeight);
    ctx.fillStyle = "#385047"; ctx.font = "12px sans-serif"; ctx.textAlign = "center"; ctx.fillText(Math.round(value).toLocaleString(), x + barWidth / 2, y - 7);
    ctx.save(); ctx.translate(x + barWidth / 2, top + height + 16); ctx.rotate(-Math.PI / 5); ctx.fillStyle = "#61716A"; ctx.fillText(label.slice(0, 8), 0, 0); ctx.restore();
  });
  ctx.textAlign = "left";
  return canvas.toDataURL("image/png");
}

function normalizeOperation(input: Operation & Record<string, unknown>): Operation {
  const aliases: Record<string, Operation["type"]> = {
    modify_cell: "set_cells", update_cell: "set_cells", update_cells: "set_cells", set_value: "set_cells",
    delete_row: "delete_rows", remove_row: "delete_rows", remove_rows: "delete_rows",
    insert_row: "insert_rows", add_row: "insert_rows", add_rows: "insert_rows",
    delete_column: "delete_columns", remove_column: "delete_columns", insert_column: "add_column",
fill_empty: "fill_blank", fill_blanks: "fill_blank", add_filter: "enable_filter",
    sort: "sort_rows", filter: "filter_rows", highlight: "highlight_rows",
    create_report: "create_dashboard", generate_report: "create_dashboard", create_dashboard_sheet: "create_dashboard",
    smart_analysis: "create_analysis", analyze: "create_analysis", data_analysis: "create_analysis",
    generate_analysis: "create_analysis", create_analysis_sheet: "create_analysis"
  };
  const nestedAction = input.action && typeof input.action === "object"
    ? input.action as Record<string, unknown>
    : {};
  const merged = { ...input, ...nestedAction } as Operation & Record<string, unknown>;
  const originalType = String(
    merged.type || (typeof input.action === "string" ? input.action : "") ||
    input.operation || input.operationType || input.action_type || input.op || input.tool || input.name || ""
  ).trim();
  const normalized = { ...merged, type: aliases[originalType] || originalType } as Operation;
  if (normalized.type === "set_cells" && !normalized.cells) {
    const cell = String(input.cell || input.address || input.range || "");
    if (cell) normalized.cells = [{ cell, value: (input.value ?? null) as ExcelValue, formula: input.formula as string | undefined }];
  }
  if (normalized.type === "delete_rows") {
    normalized.startRow ??= Number(input.row || input.rowIndex || 1);
    normalized.count ??= Number(input.rowCount || 1);
  }
  if (normalized.type === "insert_rows") {
    normalized.startRow ??= Number(input.row || input.rowIndex || 1);
    normalized.values ??= (input.rowValues || input.data || []) as ExcelValue[] | ExcelValue[][];
  }
  return normalized;
}

export default function Home() {
  const [workbook, setWorkbook] = useState<ExcelJS.Workbook | null>(null);
  const [sheets, setSheets] = useState<SheetView[]>([]);
  const [activeSheet, setActiveSheet] = useState("");
  const [fileName, setFileName] = useState("");
  const [loading, setLoading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [search, setSearch] = useState("");
  const [zoom, setZoom] = useState(100);
  const [selected, setSelected] = useState({ row: 1, col: 1 });
  const [prompt, setPrompt] = useState("");
  const [thinking, setThinking] = useState(false);
  const [pendingPlan, setPendingPlan] = useState<Plan | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [showHistory, setShowHistory] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [filterMenu, setFilterMenu] = useState<{ column: string; headerRow: number; x: number; y: number } | null>(null);
  const [dashboards, setDashboards] = useState<Record<string, DashboardData>>({});
  const [deferredSource, setDeferredSource] = useState<ArrayBuffer | null>(null);
  const [previewPage, setPreviewPage] = useState(0);
  const [toast, setToast] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("sso") !== "1") return;
    fetch("/api/auth/sso/exchange", { method: "POST", headers: { "x-aiexcel-sso": "1" }, credentials: "include", cache: "no-store" })
      .then(() => window.history.replaceState({}, "", "/"))
      .catch(() => undefined);
  }, []);

  const active = sheets.find(s => s.name === activeSheet);
  const filteredRows = useMemo(() => {
    if (!active) return [];
    const q = search.toLowerCase();
    return active.rows.filter((row, idx) => {
      if (active.hiddenRows.includes(active.rowOffset + idx + 1)) return false;
      if (!search.trim()) return true;
      return idx === 0 || row.some(c => displayCell(c).toLowerCase().includes(q));
    });
  }, [active, search]);
  const selectedCell = active?.rows[selected.row - active.rowOffset - 1]?.[selected.col - 1];
  const totalPreviewPages = Math.max(1, Math.ceil((active?.rowCount || 1) / PREVIEW_PAGE_SIZE));

  const notify = (text: string) => { setToast(text); window.setTimeout(() => setToast(""), 2600); };

  const goToPreviewPage = (page: number) => {
    if (!workbook || deferredSource) return;
    const next = Math.max(0, Math.min(totalPreviewPages - 1, page));
    setPreviewPage(next);
    setSheets(workbookToViews(workbook, next * PREVIEW_PAGE_SIZE));
    setSelected({ row: next * PREVIEW_PAGE_SIZE + 1, col: 1 });
  };

  const saveSnapshot = useCallback(async (title: string, wb: ExcelJS.Workbook) => {
    const buffer = await wb.xlsx.writeBuffer() as ArrayBuffer;
    setHistory(prev => {
      const next = [...prev.slice(0, historyIndex + 1), { title, timestamp: Date.now(), buffer }];
      return next.slice(-30);
    });
    setHistoryIndex(prev => Math.min(prev + 1, 29));
    try {
      localStorage.setItem("aiexcel-history-meta", JSON.stringify([
        ...history.slice(-9).map(h => ({ title: h.title, timestamp: h.timestamp })),
        { title, timestamp: Date.now() }
      ].slice(-10)));
    } catch {}
  }, [history, historyIndex]);

  const loadFile = useCallback(async (file: File) => {
    if (!/\.(xlsx|xls)$/i.test(file.name)) { notify("请选择 .xlsx 或 .xls 文件"); return; }
    setLoading(true);
    try {
      const data = await file.arrayBuffer();
      let wb = new ExcelJS.Workbook();
      let compatibilityMode = false;
      let largeFileMode = false;
      const fullRowCounts = new Map<string, number>();
      if (/\.xlsx$/i.test(file.name) && file.size >= 5 * 1024 * 1024) {
        const preview = XLSX.read(data, { type: "array", cellStyles: true, cellFormula: true, cellDates: true, dense: true, sheetRows: 501 });
        for (const name of preview.SheetNames) {
          const sheet = preview.Sheets[name];
          const reference = sheet["!fullref"] || sheet["!ref"];
          if (reference) fullRowCounts.set(name, XLSX.utils.decode_range(reference).e.r + 1);
        }
        const converted = XLSX.write(preview, { type: "array", bookType: "xlsx", bookSST: true });
        await wb.xlsx.load(converted);
        largeFileMode = true;
        setDeferredSource(data);
      } else if (/\.xls$/i.test(file.name)) {
        const legacy = XLSX.read(data, { type: "array", cellStyles: true, cellFormula: true, cellDates: true, bookVBA: true });
        const converted = XLSX.write(legacy, { type: "array", bookType: "xlsx", bookSST: true });
        await wb.xlsx.load(converted);
        compatibilityMode = true;
        setDeferredSource(null);
      } else {
        try {
          await wb.xlsx.load(data);
        } catch (excelJsError) {
          console.warn("ExcelJS direct load failed, retrying through compatibility conversion", excelJsError);
          const source = XLSX.read(data, { type: "array", cellStyles: true, cellFormula: true, cellDates: true, bookVBA: true });
          const converted = XLSX.write(source, { type: "array", bookType: "xlsx", bookSST: true });
          wb = new ExcelJS.Workbook();
          await wb.xlsx.load(converted);
          compatibilityMode = true;
        }
        setDeferredSource(null);
      }
      setWorkbook(wb); const views = workbookToViews(wb);
      if (largeFileMode) views.forEach(view => { view.rowCount = fullRowCounts.get(view.name) || view.rowCount; });
      setSheets(views);
      setPreviewPage(0);
      setActiveSheet(views[0]?.name ?? ""); setFileName(file.name);
      const buf = largeFileMode ? data : await wb.xlsx.writeBuffer() as ArrayBuffer;
      setHistory([{ title: "打开工作簿", timestamp: Date.now(), buffer: buf }]); setHistoryIndex(0);
      setMessages([{ role: "assistant", text: `${largeFileMode ? "已通过大文件快速模式完成预览" : compatibilityMode ? "已通过兼容模式完成本地解析" : "已完成本地解析"}：${views.length} 个工作表，${views.reduce((n, s) => n + s.rowCount, 0).toLocaleString()} 行数据。${largeFileMode ? "当前仅渲染前 500 行，确认修改时才会加载完整数据。" : ""}文件内容不会离开你的浏览器。你想先处理什么？`, timestamp: Date.now() }]);
      notify(largeFileMode ? "大文件快速预览已就绪" : compatibilityMode ? "已使用兼容模式打开工作簿" : "工作簿解析完成");
    } catch (error) {
      console.error(error);
      const detail = error instanceof Error ? error.message : "未知格式错误";
      setMessages([{ role: "assistant", text: `文件解析失败：${detail}`, timestamp: Date.now() }]);
      notify("文件解析失败，请检查文件是否加密");
    }
    finally { setLoading(false); }
  }, []);

  const restoreSnapshot = async (index: number) => {
    const entry = history[index]; if (!entry) return;
    const wb = new ExcelJS.Workbook(); await wb.xlsx.load(entry.buffer.slice(0));
    setWorkbook(wb); setPreviewPage(0); setSheets(workbookToViews(wb, 0)); setActiveSheet(prev => wb.getWorksheet(prev)?.name ?? wb.worksheets[0]?.name ?? "");
    setHistoryIndex(index); notify(index < historyIndex ? "已撤销上一步" : "已重做");
  };

  const executePlan = async (plan: Plan) => {
    if (!workbook) return;
    setLoading(true);
    try {
      let activeWorkbook = workbook;
      if (deferredSource) {
        notify("正在加载完整工作簿，百万行文件可能需要一些时间");
        activeWorkbook = new ExcelJS.Workbook();
        try {
          await activeWorkbook.xlsx.load(deferredSource.slice(0));
        } catch {
          const source = XLSX.read(deferredSource.slice(0), { type: "array", cellStyles: true, cellFormula: true, cellDates: true, bookVBA: true });
          const converted = XLSX.write(source, { type: "array", bookType: "xlsx", bookSST: true });
          await activeWorkbook.xlsx.load(converted);
        }
        setWorkbook(activeWorkbook);
        setDeferredSource(null);
      }
      let createdSheetName = "";
      if (!plan.operations.length) throw new Error("操作计划中没有可执行的修改");
      const supported = new Set<Operation["type"]>([
        "add_formula", "set_formula", "style_range", "conditional_format", "create_analysis", "beautify", "find_replace",
        "delete_rows", "insert_rows", "set_cells", "delete_columns", "add_column", "fill_blank", "enable_filter",
        "sort_rows", "filter_rows", "clear_filter", "highlight_rows", "create_dashboard"
      ]);
      let executedCount = 0;
      for (const rawOperation of plan.operations) {
        const op = normalizeOperation(rawOperation as Operation & Record<string, unknown>);
        if (!supported.has(op.type)) throw new Error(`暂不支持模型返回的操作类型：${String(op.type)}`);
        const ws = activeWorkbook.getWorksheet(op.sheet || activeSheet) || activeWorkbook.worksheets[0];
        if (!ws) continue;
        executedCount++;
        if (op.type === "add_formula") {
          let col = ws.columnCount + 1;
          const existing = ws.getRow(1).values as ExcelJS.CellValue[];
          const found = existing.findIndex(v => String(v ?? "").trim() === String(op.column || op.header));
          if (found > 0) col = found;
          ws.getCell(1, col).value = op.column || op.header || "计算结果";
          ws.getCell(1, col).font = { bold: true, color: { argb: "FFFFFFFF" } };
          ws.getCell(1, col).fill = { type: "pattern", pattern: "solid", fgColor: { argb: `FF${COLORS.green}` } };
          for (let r = 2; r <= ws.rowCount; r++) ws.getCell(r, col).value = { formula: resolveFormula(op.formula || "0", r) };
          ws.getColumn(col).width = Math.max(14, String(op.column || "").length * 2 + 4);
        }
        if (op.type === "delete_rows") {
          const start = Math.max(1, Number(op.startRow || 1));
          const count = Math.max(1, Number(op.count || 1));
          ws.spliceRows(start, count);
        }
        if (op.type === "insert_rows") {
          const start = Math.max(1, Number(op.startRow || ws.rowCount + 1));
          const rows = Array.isArray(op.values?.[0]) ? op.values as ExcelValue[][] : [Array.isArray(op.values) ? op.values as ExcelValue[] : []];
          ws.spliceRows(start, 0, ...rows);
        }
        if (op.type === "set_cells" && Array.isArray(op.cells)) {
          for (const change of op.cells) {
            if (!change?.cell) continue;
            ws.getCell(change.cell).value = change.formula
              ? { formula: resolveFormula(change.formula, ws.getCell(change.cell).row) }
              : change.value;
          }
        }
        if (op.type === "delete_columns") {
          const start = Math.max(1, Number(op.startColumn || 1));
          ws.spliceColumns(start, Math.max(1, Number(op.count || 1)));
        }
        if (op.type === "add_column") {
          const position = Math.max(1, Number(op.startColumn || ws.columnCount + 1));
          const values = Array.isArray(op.values) ? op.values as ExcelValue[] : [];
          ws.spliceColumns(position, 0, [op.header || op.column || "新列", ...values]);
        }
        if (op.type === "fill_blank") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          const startRow = Math.max(headerRow + 1, Number(op.startDataRow || headerRow + 1));
          const endRow = Math.min(ws.rowCount, Math.max(startRow, Number(op.endRow || ws.rowCount)));
          const wanted = new Set((op.columns || []).map(name => String(name).trim()));
          const targetColumns: number[] = [];
          ws.getRow(headerRow).eachCell({ includeEmpty: true }, (cell, colNumber) => {
            const header = String(cell.text || cell.value || "").trim();
            if (wanted.has(header)) targetColumns.push(colNumber);
          });
          for (const reference of wanted) {
            const col = columnNumber(reference);
            if (col > 0 && col <= Math.max(ws.columnCount, ws.getRow(headerRow).cellCount) && !targetColumns.includes(col)) {
              targetColumns.push(col);
            }
          }
          if (!targetColumns.length) throw new Error(`未在第 ${headerRow} 行找到目标列：${[...wanted].join("、")}`);
          for (let row = startRow; row <= endRow; row++) {
            for (const col of targetColumns) {
              const cell = ws.getCell(row, col);
              const value = cell.value;
              const isBlank = value === null || value === undefined || (typeof value === "string" && value.trim() === "");
              if (isBlank) cell.value = op.value ?? "无";
            }
          }
        }
        if (op.type === "enable_filter") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          ws.autoFilter = {
            from: { row: headerRow, column: 1 },
            to: { row: Math.max(headerRow, ws.rowCount), column: Math.max(1, ws.columnCount) }
          };
        }
        if (op.type === "sort_rows") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          const startRow = Math.max(headerRow + 1, Number(op.startDataRow || headerRow + 1));
          const endRow = Math.min(ws.rowCount, Math.max(startRow, Number(op.endRow || ws.rowCount)));
          const sortColumn = findHeaderColumn(ws, headerRow, op.column);
          if (!sortColumn) throw new Error(`未在第 ${headerRow} 行找到排序列：${op.column || ""}`);
          const snapshots = [];
          for (let rowNumber = startRow; rowNumber <= endRow; rowNumber++) {
            const row = ws.getRow(rowNumber);
            snapshots.push({
              height: row.height,
              values: Array.from({ length: ws.columnCount }, (_, i) => row.getCell(i + 1).value),
              styles: Array.from({ length: ws.columnCount }, (_, i) => row.getCell(i + 1).style),
              key: comparable(row.getCell(sortColumn).value)
            });
          }
          const direction = op.order === "desc" ? -1 : 1;
          snapshots.sort((a, b) => {
            if (a.key === b.key) return 0;
            if (a.key === "" || a.key === null) return 1;
            if (b.key === "" || b.key === null) return -1;
            return (a.key < b.key ? -1 : 1) * direction;
          });
          snapshots.forEach((snapshot, index) => {
            const row = ws.getRow(startRow + index);
            row.height = snapshot.height;
            row.hidden = false;
            snapshot.values.forEach((value, colIndex) => {
              const cell = row.getCell(colIndex + 1);
              cell.value = value;
              cell.style = snapshot.styles[colIndex];
            });
          });
          ws.autoFilter = {
            from: { row: headerRow, column: 1 },
            to: { row: Math.max(headerRow, ws.rowCount), column: Math.max(1, ws.columnCount) }
          };
        }
        if (op.type === "filter_rows") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          const startRow = Math.max(headerRow + 1, Number(op.startDataRow || headerRow + 1));
          const endRow = Math.min(ws.rowCount, Math.max(startRow, Number(op.endRow || ws.rowCount)));
          const filterColumn = findHeaderColumn(ws, headerRow, op.column);
          if (!filterColumn) throw new Error(`未在第 ${headerRow} 行找到筛选列：${op.column || ""}`);
          for (let rowNumber = startRow; rowNumber <= endRow; rowNumber++) {
            const raw = ws.getCell(rowNumber, filterColumn).value;
            const text = String(raw ?? "").trim();
            const number = Number(text.replace(/[￥¥,$,\s]/g, ""));
            const target = String(op.filterValue ?? "").trim();
            let match = true;
            if (op.operator === "equals") match = text === target;
            else if (op.operator === "not_equals") match = text !== target;
            else if (op.operator === "contains") match = text.includes(target);
            else if (op.operator === "greater_than") match = Number.isFinite(number) && number > Number(op.filterValue);
            else if (op.operator === "less_than") match = Number.isFinite(number) && number < Number(op.filterValue);
            else if (op.operator === "between") match = Number.isFinite(number) && number >= Number(op.min) && number <= Number(op.max);
            else if (op.operator === "blank") match = text === "";
            else if (op.operator === "not_blank") match = text !== "";
            ws.getRow(rowNumber).hidden = !match;
          }
          ws.autoFilter = {
            from: { row: headerRow, column: 1 },
            to: { row: Math.max(headerRow, ws.rowCount), column: Math.max(1, ws.columnCount) }
          };
        }
        if (op.type === "clear_filter") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          for (let rowNumber = headerRow + 1; rowNumber <= ws.rowCount; rowNumber++) ws.getRow(rowNumber).hidden = false;
          ws.autoFilter = {
            from: { row: headerRow, column: 1 },
            to: { row: Math.max(headerRow, ws.rowCount), column: Math.max(1, ws.columnCount) }
          };
        }
        if (op.type === "highlight_rows") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          const startRow = Math.max(headerRow + 1, Number(op.startDataRow || headerRow + 1));
          const endRow = Math.min(ws.rowCount, Math.max(startRow, Number(op.endRow || ws.rowCount)));
          const matchColumn = findHeaderColumn(ws, headerRow, op.column);
          if (!matchColumn) throw new Error(`未在第 ${headerRow} 行找到条件列：${op.column || ""}`);
          const background = String(op.backgroundColor || "C6EFCE").replace("#", "").toUpperCase();
          const foreground = String(op.fontColor || "006100").replace("#", "").toUpperCase();
          const expected = String(op.matchValue ?? op.filterValue ?? "").trim();
          if (!expected && !["blank", "not_blank"].includes(op.operator || "")) {
            throw new Error("整行标色缺少有效的匹配值");
          }
          for (let rowNumber = startRow; rowNumber <= endRow; rowNumber++) {
            const actual = ws.getCell(rowNumber, matchColumn).text.trim();
            const matches = op.operator === "contains" ? actual.includes(expected)
              : op.operator === "not_equals" ? actual !== expected
              : op.operator === "blank" ? actual === ""
              : op.operator === "not_blank" ? actual !== ""
              : actual === expected;
            if (!matches) continue;
            for (let col = 1; col <= ws.columnCount; col++) {
              const cell = ws.getCell(rowNumber, col);
              cell.style = {
                ...cell.style,
                fill: { type: "pattern", pattern: "solid", fgColor: { argb: `FF${background}` } },
                font: { ...cell.font, color: { argb: `FF${foreground}` } }
              };
            }
          }
          const escaped = expected.replace(/"/g, '""');
          const operator = op.operator === "not_equals" ? "<>" : "=";
          ws.addConditionalFormatting({
            ref: `A${startRow}:${columnLetter(Math.max(1, ws.columnCount))}${endRow}`,
            rules: [{
              type: "expression",
              formulae: [`$${columnLetter(matchColumn)}${startRow}${operator}"${escaped}"`],
              priority: 1,
              style: {
                fill: { type: "pattern", pattern: "solid", fgColor: { argb: `FF${background}` } },
                font: { color: { argb: `FF${foreground}` } }
              }
            }]
          });
        }
        if (op.type === "create_dashboard") {
          const headerRow = Math.max(1, Number(op.headerRow || 1));
          const startRow = Math.max(headerRow + 1, Number(op.startDataRow || headerRow + 1));
          const groupColumn = findHeaderColumn(ws, headerRow, op.groupByColumn);
          const valueColumn = findHeaderColumn(ws, headerRow, op.valueColumn);
          if (!groupColumn || !valueColumn) throw new Error(`未找到报表字段：${op.groupByColumn || "分组列"}、${op.valueColumn || "数值列"}`);
          const groups = new Map<string, number[]>();
          let inheritedGroup = "";
          for (let row = startRow; row <= ws.rowCount; row++) {
            const currentGroup = ws.getCell(row, groupColumn).text.trim();
            if (currentGroup) inheritedGroup = currentGroup;
            const group = currentGroup || inheritedGroup;
            const value = parseCellNumber(ws.getCell(row, valueColumn));
            if (!group || value === null) continue;
            const list = groups.get(group) || []; list.push(value); groups.set(group, list);
          }
          if (!groups.size) throw new Error("没有找到可用于生成报表的有效数值数据");
          const summary = [...groups.entries()].map(([group, values]) => ({
            group, count: values.length,
            average: values.reduce((sum, value) => sum + value, 0) / values.length,
            min: Math.min(...values), max: Math.max(...values)
          })).sort((a, b) => b.average - a.average);
          const reportName = (op.sheetName || op.title || `${op.valueColumn || "数据"}分析`).slice(0, 31);
          const existing = activeWorkbook.getWorksheet(reportName); if (existing) activeWorkbook.removeWorksheet(existing.id);
          const report = activeWorkbook.addWorksheet(reportName, { views: [{ state: "frozen", ySplit: 3 }] });
          report.mergeCells("A1:E1"); report.getCell("A1").value = `${op.groupByColumn} × ${op.valueColumn} 数据研究报表`;
          report.getCell("A1").font = { bold: true, size: 18, color: { argb: "FFFFFFFF" } };
          report.getCell("A1").fill = { type: "pattern", pattern: "solid", fgColor: { argb: `FF${COLORS.green}` } };
          report.getCell("A1").alignment = { vertical: "middle", horizontal: "left" }; report.getRow(1).height = 38;
          report.mergeCells("A2:E2"); report.getCell("A2").value = `数据源：${ws.name} · 共 ${summary.reduce((sum, item) => sum + item.count, 0)} 条有效记录 · 生成时间：${new Date().toLocaleString("zh-CN")}`;
          report.getCell("A2").font = { color: { argb: "FF68736F" }, italic: true };
          report.addRow([op.groupByColumn || "分组", "样本数", `平均${op.valueColumn}`, `最低${op.valueColumn}`, `最高${op.valueColumn}`]);
          summary.forEach(item => report.addRow([item.group, item.count, item.average, item.min, item.max]));
          report.getRow(3).eachCell(cell => {
            cell.font = { bold: true, color: { argb: "FFFFFFFF" } };
            cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FF2B5E4B" } };
            cell.alignment = { horizontal: "center" };
          });
          for (let row = 4; row <= summary.length + 3; row++) {
            report.getCell(row, 2).numFmt = "0";
            for (let col = 3; col <= 5; col++) report.getCell(row, col).numFmt = '¥#,##0.00';
            if (row % 2 === 0) report.getRow(row).eachCell(cell => {
              cell.style = { ...cell.style, fill: { type: "pattern", pattern: "solid", fgColor: { argb: "FFF1F7F4" } } };
            });
          }
          report.columns = [{ width: 18 }, { width: 12 }, { width: 18 }, { width: 18 }, { width: 18 }];
          const researchRow = summary.length + 6;
          report.mergeCells(researchRow, 1, researchRow, 5); report.getCell(researchRow, 1).value = "研究选项";
          report.getCell(researchRow, 1).font = { bold: true, size: 14, color: { argb: `FF${COLORS.green}` } };
          [
            ["区域价格对比", "比较各省份平均租金，识别高价与低价区域"],
            ["价格离散程度", "结合最低价和最高价判断各省份房源差异"],
            ["样本可信度", "优先关注样本数较多的省份，避免小样本偏差"],
            ["后续研究", "可继续按城市、房型、面积或交通条件进行交叉分析"]
          ].forEach((item, index) => {
            const row = researchRow + index + 1; report.getCell(row, 1).value = `0${index + 1}`; report.getCell(row, 2).value = item[0];
            report.mergeCells(row, 3, row, 5); report.getCell(row, 3).value = item[1];
            report.getCell(row, 1).font = { bold: true, color: { argb: `FF${COLORS.green}` } };
            report.getCell(row, 2).font = { bold: true };
          });
          const chart = makeBarChart(summary.map(item => item.group), summary.map(item => item.average), `各${op.groupByColumn}平均${op.valueColumn}`);
          if (chart) {
            const imageId = activeWorkbook.addImage({ base64: chart, extension: "png" });
            report.addImage(imageId, { tl: { col: 6, row: 0 }, ext: { width: 720, height: 360 } });
          }
          setDashboards(previous => ({ ...previous, [reportName]: {
            groupLabel: op.groupByColumn || "分类",
            valueLabel: op.valueColumn || "数值",
            sourceSheet: ws.name,
            rows: summary
          } }));
          createdSheetName = reportName;
        }
        if (op.type === "set_formula" && op.range) {
          const range = ws.getCell(op.range.split(":")[0]); const end = op.range.includes(":") ? ws.getCell(op.range.split(":")[1]) : range;
          for (let r = range.row; r <= end.row; r++) for (let c = range.col; c <= end.col; c++)
            ws.getCell(r, c).value = { formula: resolveFormula(op.formula || "0", r) };
        }
        if (op.type === "beautify") {
          ws.views = [{ state: "frozen", ySplit: 1 }];
          ws.getRow(1).height = 30;
          ws.getRow(1).eachCell(cell => {
            cell.font = { bold: true, color: { argb: "FFFFFFFF" }, size: 11 };
            cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: `FF${COLORS.green}` } };
            cell.alignment = { vertical: "middle", horizontal: "center" };
          });
          ws.columns.forEach(col => { col.width = Math.min(32, Math.max(12, ...(col.values || []).slice(1, 100).map(v => String(v ?? "").length + 3))); });
          for (let r = 2; r <= ws.rowCount; r++) ws.getRow(r).eachCell(cell => {
            cell.border = { bottom: { style: "hair", color: { argb: "FFE4EAE7" } } };
            if (r % 2 === 0) cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFF6FAF8" } };
          });
        }
        if (op.type === "style_range" && op.range) {
          const [a, b = a] = op.range.split(":").map(x => ws.getCell(x));
          for (let r = a.row; r <= b.row; r++) for (let c = a.col; c <= b.col; c++) {
            const cell = ws.getCell(r, c); const style = op.style || {};
            if (style.bold) cell.font = { ...cell.font, bold: true };
            if (style.backgroundColor) cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: `FF${String(style.backgroundColor).replace("#", "")}` } };
            if (style.numberFormat) cell.numFmt = String(style.numberFormat);
          }
        }
        if (op.type === "conditional_format" && op.range) {
          const condition = op.condition || ">0";
          ws.addConditionalFormatting({ ref: op.range, rules: [{
            type: "cellIs", operator: condition.includes("<") ? "lessThan" : "greaterThan",
            formulae: [condition.replace(/[<>=]/g, "") || "0"], priority: 1,
            style: { fill: { type: "pattern", pattern: "solid", bgColor: { argb: `FF${COLORS.mint}` } }, font: { color: { argb: `FF${COLORS.green}` } } }
          }] as ExcelJS.ConditionalFormattingRule[] });
        }
        if (op.type === "find_replace" && op.find) ws.eachRow(row => row.eachCell(cell => {
          if (typeof cell.value === "string" && cell.value.includes(op.find!)) cell.value = cell.value.replaceAll(op.find!, op.replace || "");
        }));
        if (op.type === "create_analysis") {
          const source = activeWorkbook.getWorksheet(op.sourceSheet || op.sheet || activeSheet) || ws;
          const name = op.title || "智能分析";
          const old = activeWorkbook.getWorksheet(name); if (old) activeWorkbook.removeWorksheet(old.id);
          const report = activeWorkbook.addWorksheet(name);
          report.addRow(["指标", "结果"]); report.addRow(["数据行数", Math.max(0, source.rowCount - 1)]);
          report.addRow(["字段数量", source.columnCount]); report.addRow(["来源工作表", source.name]);
          report.getRow(1).font = { bold: true, color: { argb: "FFFFFFFF" } };
          report.getRow(1).fill = { type: "pattern", pattern: "solid", fgColor: { argb: `FF${COLORS.green}` } };
          report.columns = [{ width: 24 }, { width: 20 }];
        }
      }
      if (!executedCount) throw new Error("没有对工作簿执行任何有效修改");
      setPreviewPage(0);
      setSheets(workbookToViews(activeWorkbook, 0));
      if (createdSheetName) setActiveSheet(createdSheetName);
      await saveSnapshot(plan.title, activeWorkbook);
      setPendingPlan(null); setMessages(prev => [...prev, { role: "assistant", text: `已执行「${plan.title}」。所有改动已记录，可随时撤销或导出。`, timestamp: Date.now() }]);
      notify("操作执行成功");
    } catch (error) {
      console.error(error);
      const detail = error instanceof Error ? error.message : "未知错误";
      setMessages(prev => [...prev, { role: "assistant", text: `执行失败：${detail}。工作簿未标记为成功修改。`, timestamp: Date.now() }]);
      notify(`执行失败：${detail}`);
    }
    finally { setLoading(false); }
  };

  const runColumnAction = (operation: Operation, title: string) => {
    setFilterMenu(null);
    executePlan({ title, summary: title, operations: [{ ...operation, sheet: activeSheet }] });
  };

  const askAI = async () => {
    const request = prompt.trim(); if (!request || !workbook) return;
    setPrompt(""); setThinking(true); setMessages(prev => [...prev, { role: "user", text: request, timestamp: Date.now() }]);
    try {
      const payload = { request, activeSheet, schema: getWorkbookSchema(sheets), history: messages.slice(-6).map(m => ({ role: m.role, content: m.text })) };
      const response = await fetch("/api/ai", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify(payload) });
      if (!response.ok) throw new Error((await response.json()).error || "AI 请求失败");
      const result = await response.json();
      const plan: Plan = result;
      if (!plan.title || !Array.isArray(plan.operations)) throw new Error("AI 返回的操作计划格式不正确");
      plan.operations = plan.operations.map(operation => {
        const normalized = normalizeOperation(operation as Operation & Record<string, unknown>);
        if (!normalized.type && /分析|统计|报表|dashboard/i.test(request)) {
          return {
            ...normalized,
            type: /报表|图表|dashboard/i.test(request) ? "create_dashboard" : "create_analysis",
            sheet: activeSheet,
            sourceSheet: activeSheet,
            title: normalized.title || "智能分析"
          };
        }
        return normalized;
      });
      setPendingPlan(plan);
      setMessages(prev => [...prev, { role: "assistant", text: plan.summary, plan, timestamp: Date.now() }]);
    } catch (error) {
      setMessages(prev => [...prev, { role: "assistant", text: `暂时无法连接 AI：${error instanceof Error ? error.message : "请稍后重试"}。你仍可继续预览和导出文件。`, timestamp: Date.now() }]);
    } finally { setThinking(false); }
  };

  const download = async () => {
    if (!workbook) return;
    const buffer = deferredSource || await workbook.xlsx.writeBuffer();
    const blob = new Blob([buffer], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" });
    const a = document.createElement("a"); a.href = URL.createObjectURL(blob);
    a.download = deferredSource ? fileName : `${fileName.replace(/\.(xlsx|xls)$/i, "") || "workbook"}-AI优化.xlsx`; a.click(); URL.revokeObjectURL(a.href); notify("导出已开始");
  };

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "z") { e.preventDefault(); restoreSnapshot(Math.max(0, historyIndex - 1)); }
      if ((e.metaKey || e.ctrlKey) && e.key === "y") { e.preventDefault(); restoreSnapshot(Math.min(history.length - 1, historyIndex + 1)); }
    };
    window.addEventListener("keydown", handler); return () => window.removeEventListener("keydown", handler);
  });

  if (!workbook) return (
    <main className="landing">
      <header className="landing-nav">
        <div className="brand"><span className="brand-mark"><img src="/project-icon.jpg" alt="AI表格" /></span><span>AI表格</span></div>
        <div className="nav-actions"><span className="local-badge"><ShieldCheck size={15}/> 文件仅在本地处理</span><button className="icon-btn" onClick={() => setShowSettings(true)} aria-label="设置"><Settings size={19}/></button></div>
      </header>
      <section className="hero">
        <div className="eyebrow"><Sparkles size={15}/> 用一句话，完成复杂表格操作</div>
        <h1>和 Excel 聊聊天，<br/><span>工作就完成了。</span></h1>
        <p>上传工作簿，用自然语言生成公式、关联数据、格式化报表与创建分析。<br/>原始文件不会上传，隐私始终留在你的设备。</p>
        <div className={`drop-zone ${dragging ? "dragging" : ""}`} onDragOver={e => { e.preventDefault(); setDragging(true); }} onDragLeave={() => setDragging(false)} onDrop={e => { e.preventDefault(); setDragging(false); const f = e.dataTransfer.files[0]; if (f) loadFile(f); }}>
          <input ref={inputRef} type="file" accept=".xlsx,.xls" onChange={e => e.target.files?.[0] && loadFile(e.target.files[0])}/>
          <div className="upload-art"><UploadCloud size={30}/></div>
          <h2>{loading ? "正在本地解析…" : "把 Excel 文件拖到这里"}</h2>
          <p>支持 .xlsx 与 .xls，最大建议 50 MB</p>
          <button className="primary-btn" disabled={loading} onClick={() => inputRef.current?.click()}>{loading ? <LoaderCircle className="spin" size={18}/> : <Plus size={18}/>} 选择文件</button>
        </div>
        <div className="trust-row"><span><Check/> 公式与样式保留</span><span><Check/> 跨表关联</span><span><Check/> 随时撤销</span><span><Check/> 一键导出</span></div>
      </section>
      <footer className="landing-footer">工作簿在浏览器解析 · AI 请求通过 Sub2API 服务端转发</footer>
      {showSettings && <SettingsModal onClose={() => setShowSettings(false)}/>}
      {toast && <div className="toast">{toast}</div>}
    </main>
  );

  return (
    <main className="workspace">
      <header className="topbar">
        <div className="top-left">
          <button className="icon-btn" onClick={() => { if (confirm("返回首页？未导出的修改将丢失。")) { setWorkbook(null); setSheets([]); setDeferredSource(null); } }}><ArrowLeft size={19}/></button>
          <div className="brand compact"><span className="brand-mark"><img src="/project-icon.jpg" alt="AI表格" /></span><span>AI表格</span></div><span className="divider"/>
          <div className="file-title"><strong>{fileName}</strong><small>{sheets.length} 个工作表 · 已在本地打开</small></div>
        </div>
        <div className="top-actions">
          <button className="tool-btn" disabled={historyIndex <= 0} onClick={() => restoreSnapshot(historyIndex - 1)}><Undo2/> 撤销</button>
          <button className="tool-btn only-icon" disabled={historyIndex >= history.length - 1} onClick={() => restoreSnapshot(historyIndex + 1)} aria-label="重做"><Redo2/></button>
          <button className="tool-btn" onClick={() => setShowHistory(true)}><History/> 历史</button>
          <button className="export-btn" onClick={download}><Download size={17}/> 导出 Excel</button>
          <button className="icon-btn" onClick={() => setShowSettings(true)}><Settings size={19}/></button>
        </div>
      </header>
      <div className="work-body">
        <section className="sheet-panel">
          <div className="sheet-toolbar">
            <div className="cell-name">{columnLetter(selected.col)}{selected.row}</div>
            <div className="formula-box"><span>fx</span><input readOnly value={selectedCell?.formula ? `=${selectedCell.formula}` : displayCell(selectedCell)}/></div>
            <label className="sheet-search"><Search/><input value={search} onChange={e => setSearch(e.target.value)} placeholder="在工作表中搜索"/></label>
          </div>
          <div className={`grid-wrap ${dashboards[activeSheet] ? "dashboard-wrap" : ""}`}>
            {dashboards[activeSheet] && <InteractiveDashboard data={dashboards[activeSheet]}/>}
            <table className="excel-grid" style={{ fontSize: `${zoom}%` }}>
              <thead><tr><th className="corner"></th>{Array.from({ length: Math.max(active?.colCount || 0, active?.rows[0]?.length || 0) }, (_, i) => <th key={i}>{columnLetter(i + 1)}</th>)}</tr></thead>
              <tbody>{filteredRows.map((row, ri) => {
                const actualRow = (active?.rowOffset || 0) + (active?.rows.indexOf(row) ?? ri) + 1;
                const isFilterHeader = active?.filterHeaderRow === actualRow;
                return <tr key={actualRow}><th>{actualRow}</th>{row.map((cell, ci) => <td key={ci} style={cellCss(cell)} className={`${actualRow === 1 || isFilterHeader ? "header-cell" : ""} ${isFilterHeader ? "filter-header-cell" : ""} ${selected.row === actualRow && selected.col === ci + 1 ? "selected" : ""} ${search && displayCell(cell).toLowerCase().includes(search.toLowerCase()) ? "match" : ""}`} onClick={() => setSelected({ row: actualRow, col: ci + 1 })}><span className="cell-content">{displayCell(cell)}</span>{isFilterHeader && displayCell(cell) && <button className="filter-control" title="排序和筛选" aria-label={`${displayCell(cell)}列排序和筛选`} onClick={e => { e.stopPropagation(); const rect = e.currentTarget.getBoundingClientRect(); setFilterMenu({ column: displayCell(cell), headerRow: actualRow, x: Math.min(rect.left, window.innerWidth - 255), y: Math.min(rect.bottom + 5, window.innerHeight - 330) }); }}><ArrowUpDown/><Filter/></button>}</td>)}</tr>;
              })}</tbody>
            </table>
          </div>
          <div className="sheet-tabs">
            <button className="tab-menu"><Menu/></button>
            <div className="tabs-scroll">{sheets.map(s => <button key={s.name} className={activeSheet === s.name ? "active" : ""} onClick={() => { setActiveSheet(s.name); setPreviewPage(0); if (workbook && !deferredSource) setSheets(workbookToViews(workbook, 0)); setSelected({row: 1, col: 1}); }}>{s.name}</button>)}</div>
            <div className="page-controls" title={deferredSource ? "确认执行修改后可浏览全部分页" : `共 ${totalPreviewPages} 页`}>
              <button disabled={!!deferredSource || previewPage <= 0} onClick={() => goToPreviewPage(previewPage - 1)}>‹</button>
              <span>{previewPage + 1} / {totalPreviewPages}</span>
              <button disabled={!!deferredSource || previewPage >= totalPreviewPages - 1} onClick={() => goToPreviewPage(previewPage + 1)}>›</button>
            </div>
            <div className="zoom"><button onClick={() => setZoom(z => Math.max(60, z - 10))}><ZoomOut/></button><span>{zoom}%</span><button onClick={() => setZoom(z => Math.min(160, z + 10))}><ZoomIn/></button></div>
          </div>
        </section>
        <aside className="ai-panel">
          <div className="ai-head"><div><span className="ai-avatar"><Sparkles/></span><div><strong>AI 助手</strong><small><i/> DeepSeek 已连接</small></div></div><button className="icon-btn" onClick={() => setShowSettings(true)}><Settings/></button></div>
          <div className="quick-actions">
            <button onClick={() => setPrompt("帮我美化当前工作表")}><Sparkles/> 美化报表</button>
            <button onClick={() => setPrompt("分析当前工作表并创建分析表")}><BarChart3/> 智能分析</button>
            <button onClick={() => setPrompt("检查并修复当前工作表中的公式问题")}><Bot/> 检查公式</button>
          </div>
          <div className="conversation">
            <div className="day-label">今天</div>
            {messages.map((m, i) => <div key={m.timestamp + i} className={`message ${m.role}`}>
              {m.role === "assistant" && <span className="mini-avatar"><Sparkles/></span>}
              <div className="bubble"><p>{m.text}</p>{m.plan && pendingPlan === m.plan && <PlanCard plan={m.plan} onExecute={() => executePlan(m.plan!)} onCancel={() => setPendingPlan(null)}/>}</div>
            </div>)}
            {thinking && <div className="message assistant"><span className="mini-avatar"><Sparkles/></span><div className="bubble typing"><i/><i/><i/><span>正在理解你的需求</span></div></div>}
          </div>
          <div className="composer">
            <textarea value={prompt} onChange={e => setPrompt(e.target.value)} onKeyDown={e => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); askAI(); } }} placeholder="告诉我你想对表格做什么…"/>
            <div><span>Enter 发送 · Shift + Enter 换行</span><button disabled={!prompt.trim() || thinking} onClick={askAI}><Send/></button></div>
          </div>
          <div className="privacy-note"><ShieldCheck/> 仅发送表结构与必要样例，不发送完整文件</div>
        </aside>
      </div>
      {showHistory && <HistoryDrawer entries={history} active={historyIndex} onRestore={restoreSnapshot} onClose={() => setShowHistory(false)}/>}
      {showSettings && <SettingsModal onClose={() => setShowSettings(false)}/>}
      {filterMenu && <FilterMenu menu={filterMenu} onClose={() => setFilterMenu(null)} onApply={(operation, title) => runColumnAction(operation, title)}/>}
      {loading && <div className="loading-cover"><LoaderCircle className="spin"/><span>正在处理工作簿…</span></div>}
      {toast && <div className="toast">{toast}</div>}
    </main>
  );
}

function PlanCard({ plan, onExecute, onCancel }: { plan: Plan; onExecute: () => void; onCancel: () => void }) {
  return <div className="plan-card"><div className="plan-title"><span><Check/></span><div><strong>操作计划已生成</strong><small>{plan.operations.length} 项修改等待确认</small></div></div>
    <div className="plan-list">{plan.operations.map((op, i) => <div key={i}><span>{i + 1}</span><p><strong>{
      op.type === "add_formula" ? `新增公式列：${op.column || op.header}` :
      op.type === "add_column" ? `新增列：${op.column || op.header}` :
      op.type === "delete_rows" ? `删除第 ${op.startRow || 1} 行起的 ${op.count || 1} 行` :
      op.type === "insert_rows" ? `从第 ${op.startRow || 1} 行插入数据` :
      op.type === "delete_columns" ? `删除第 ${op.startColumn || 1} 列起的 ${op.count || 1} 列` :
      op.type === "set_cells" ? `修改 ${op.cells?.length || 0} 个单元格` :
      op.type === "fill_blank" ? `填充 ${op.columns?.join("、") || "指定列"}的全部空白单元格` :
      op.type === "enable_filter" ? `为第 ${op.headerRow || 1} 行表头启用筛选与排序` :
      op.type === "sort_rows" ? `按「${op.column || "指定列"}」${op.order === "desc" ? "降序" : "升序"}排列全部数据` :
      op.type === "filter_rows" ? `按「${op.column || "指定列"}」筛选全部数据` :
      op.type === "clear_filter" ? "清除筛选并显示全部数据" :
      op.type === "highlight_rows" ? `将「${op.column || "指定列"}」匹配「${String(op.matchValue ?? op.filterValue ?? "")}」的整行标色` :
      op.type === "create_dashboard" ? `新建「${op.sheetName || op.title || "数据分析"}」报表与图表` :
      op.type === "beautify" ? "美化工作表" :
      op.type === "create_analysis" ? `创建分析表：${op.title || "智能分析"}` :
      op.type === "conditional_format" ? "添加条件颜色" :
      op.type === "find_replace" ? "查找与替换" : "修改单元格"
    }</strong><small>{op.formula ? `=${op.formula.replace(/^=/, "")}` : op.range || op.condition || op.sheet}</small></p></div>)}</div>
    {plan.warnings?.map(w => <div className="warning" key={w}>{w}</div>)}
    <div className="plan-actions"><button onClick={onCancel}>取消</button><button onClick={onExecute}><Check/> 确认执行</button></div>
  </div>;
}

function InteractiveDashboard({ data }: { data: DashboardData }) {
  const [metric, setMetric] = useState<"average" | "max" | "min" | "count">("average");
  const [descending, setDescending] = useState(true);
  const [selected, setSelected] = useState<DashboardRow | null>(null);
  const labels = { average: `平均${data.valueLabel}`, max: `最高${data.valueLabel}`, min: `最低${data.valueLabel}`, count: "样本数" };
  const rows = [...data.rows].sort((a, b) => (b[metric] - a[metric]) * (descending ? 1 : -1)).slice(0, 24);
  const maxValue = Math.max(...rows.map(row => row[metric]), 1);
  const format = (value: number) => metric === "count" ? `${value.toLocaleString()} 条` : `¥${Math.round(value).toLocaleString()}`;
  return <section className="interactive-dashboard">
    <div className="dashboard-heading"><div><span className="dashboard-icon"><BarChart3/></span><div><small>交互式数据研究</small><h2>{data.groupLabel} × {data.valueLabel}</h2><p>数据来源：{data.sourceSheet} · {data.rows.reduce((sum, row) => sum + row.count, 0)} 条有效记录</p></div></div>
      <button className="order-toggle" onClick={() => setDescending(value => !value)}><ArrowUpDown/> {descending ? "从高到低" : "从低到高"}</button>
    </div>
    <div className="metric-tabs">{(["average", "max", "min", "count"] as const).map(item => <button key={item} className={metric === item ? "active" : ""} onClick={() => { setMetric(item); setSelected(null); }}>{labels[item]}</button>)}</div>
    <div className="dashboard-content">
      <div className="interactive-bars">{rows.map(row => <button key={row.group} className={selected?.group === row.group ? "selected" : ""} onClick={() => setSelected(row)} title={`${row.group}：${format(row[metric])}`}>
        <span className="bar-label">{row.group}</span><span className="bar-track"><i style={{ width: `${Math.max(2, row[metric] / maxValue * 100)}%` }}/></span><strong>{format(row[metric])}</strong>
      </button>)}</div>
      <aside className="research-panel">{selected ? <><small>已选择区域</small><h3>{selected.group}</h3><dl><div><dt>平均价格</dt><dd>¥{Math.round(selected.average).toLocaleString()}</dd></div><div><dt>价格区间</dt><dd>¥{Math.round(selected.min).toLocaleString()} – ¥{Math.round(selected.max).toLocaleString()}</dd></div><div><dt>有效样本</dt><dd>{selected.count} 条</dd></div></dl><p>点击左侧其他区域可快速对比研究。</p></> : <><small>研究选项</small><h3>选择一个区域</h3><div className="research-options"><span>01 <b>区域价格对比</b></span><span>02 <b>价格离散程度</b></span><span>03 <b>样本可信度</b></span><span>04 <b>高低价格排序</b></span></div><p>点击图表中的任意条形查看详细指标。</p></>}</aside>
    </div>
  </section>;
}

function SettingsModal({ onClose }: { onClose: () => void }) {
  const [saved, setSaved] = useState(false);
  const save = () => {
    setSaved(true); window.setTimeout(() => setSaved(false), 1800);
  };
  return <div className="modal-backdrop" onMouseDown={e => e.target === e.currentTarget && onClose()}><div className="modal"><div className="modal-head"><div><span className="ai-avatar"><Settings/></span><div><strong>连接设置</strong><small>配置你的 DeepSeek API</small></div></div><button className="icon-btn" onClick={onClose}><X/></button></div>
    <div className="setting-status"><span><i/> Sub2API 已接入</span><strong>统一账号与中转额度</strong><small>AI 请求由服务端转发到 Sub2API，浏览器不会接触模型 API Key。</small></div>
    <div className="security-card"><ShieldCheck/><div><strong>本地优先，数据最小化</strong><p>Excel 文件始终在浏览器内解析。AI 仅接收字段名、行列数及少量结构样例，用于生成可审阅的操作计划。</p></div></div>
    <div className="settings-actions"><button className="modal-done" onClick={() => { save(); window.setTimeout(onClose, 450); }}>{saved ? <><Check/> 已保存</> : "关闭"}</button></div></div></div>;
}

function FilterMenu({ menu, onClose, onApply }: {
  menu: { column: string; headerRow: number; x: number; y: number };
  onClose: () => void;
  onApply: (operation: Operation, title: string) => void;
}) {
  const [operator, setOperator] = useState<Operation["operator"]>("contains");
  const [value, setValue] = useState("");
  const needsValue = !["blank", "not_blank"].includes(operator || "");
  return <div className="filter-menu-shield" onMouseDown={e => e.target === e.currentTarget && onClose()}>
    <div className="filter-menu" style={{ left: menu.x, top: menu.y }}>
      <div className="filter-menu-head"><div><Filter/><span><strong>{menu.column}</strong><small>排序与筛选</small></span></div><button onClick={onClose}><X/></button></div>
      <div className="sort-actions">
        <button onClick={() => onApply({ type: "sort_rows", headerRow: menu.headerRow, column: menu.column, order: "asc" }, `按「${menu.column}」升序排列`)}><ArrowUpDown/> 升序排列</button>
        <button onClick={() => onApply({ type: "sort_rows", headerRow: menu.headerRow, column: menu.column, order: "desc" }, `按「${menu.column}」降序排列`)}><ArrowUpDown/> 降序排列</button>
      </div>
      <div className="filter-form">
        <label>筛选条件<select value={operator} onChange={e => setOperator(e.target.value as Operation["operator"])}>
          <option value="contains">文本包含</option><option value="equals">等于</option><option value="not_equals">不等于</option>
          <option value="greater_than">大于</option><option value="less_than">小于</option>
          <option value="blank">空白单元格</option><option value="not_blank">非空白单元格</option>
        </select></label>
        {needsValue && <label>条件值<input autoFocus value={value} onChange={e => setValue(e.target.value)} onKeyDown={e => { if (e.key === "Enter" && value.trim()) onApply({ type: "filter_rows", headerRow: menu.headerRow, column: menu.column, operator, filterValue: value }, `筛选「${menu.column}」`); }} placeholder="输入文字或数字"/></label>}
        <button className="apply-filter" disabled={needsValue && !value.trim()} onClick={() => onApply({ type: "filter_rows", headerRow: menu.headerRow, column: menu.column, operator, filterValue: value }, `筛选「${menu.column}」`)}>应用筛选</button>
      </div>
      <button className="clear-filter" onClick={() => onApply({ type: "clear_filter", headerRow: menu.headerRow }, "清除筛选")}>显示全部数据</button>
    </div>
  </div>;
}

function HistoryDrawer({ entries, active, onRestore, onClose }: { entries: HistoryEntry[]; active: number; onRestore: (i: number) => void; onClose: () => void }) {
  return <div className="drawer-backdrop" onMouseDown={e => e.target === e.currentTarget && onClose()}><aside className="history-drawer"><div className="drawer-head"><div><History/><div><strong>操作历史</strong><small>像时光机一样回到任一步</small></div></div><button className="icon-btn" onClick={onClose}><X/></button></div>
    <div className="timeline">{[...entries].reverse().map((entry, ri) => { const i = entries.length - 1 - ri; return <button key={entry.timestamp} className={i === active ? "active" : ""} onClick={() => { onRestore(i); onClose(); }}><span className="dot">{i === active ? <Check/> : <Clock3/>}</span><div><strong>{entry.title}</strong><small>{new Date(entry.timestamp).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}</small></div>{i === active && <em>当前</em>}</button>; })}</div>
  </aside></div>;
}
