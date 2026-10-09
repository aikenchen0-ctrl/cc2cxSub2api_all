import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";

import { HTTP_BACKEND_URL } from "../../config";
import EvalNavigation from "./EvalNavigation";
import {
  formatBytes,
  formatCost,
  formatTimestamp,
  providerBadgeClass,
  providerLabel,
  modelLabel,
} from "./report-format";
import { JsonNode } from "./report-json";

interface PromptReportSummary {
  filename: string;
  provider: string;
  model: string;
  created_at: string;
  session_id: string;
  turn: number;
  size_bytes: number;
  cost_usd: number | null;
}

interface PromptReportListResponse {
  reports: PromptReportSummary[];
  total_size_bytes: number;
  reports_directory: string;
}

interface PromptReportUsage {
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_write: number;
  total_tokens: number;
  cache_hit_rate_percent: number;
  cost_usd: number | null;
}

interface PromptReportContent {
  provider: string;
  model: string;
  api_model_name: string;
  session_id: string;
  turn: number;
  created_at: string;
  request: unknown;
  usage: PromptReportUsage | null;
}

interface SessionGroup {
  sessionId: string;
  provider: string;
  model: string;
  createdAt: string;
  sizeBytes: number;
  costUsd: number | null;
  turns: PromptReportSummary[];
}

function groupBySession(reports: PromptReportSummary[]): SessionGroup[] {
  const groups = new Map<string, SessionGroup>();
  for (const report of reports) {
    let group = groups.get(report.session_id);
    if (!group) {
      group = {
        sessionId: report.session_id,
        provider: report.provider,
        model: report.model,
        createdAt: report.created_at,
        sizeBytes: 0,
        costUsd: null,
        turns: [],
      };
      groups.set(report.session_id, group);
    }
    group.sizeBytes += report.size_bytes;
    if (typeof report.cost_usd === "number") {
      group.costUsd = (group.costUsd ?? 0) + report.cost_usd;
    }
    if (report.created_at > group.createdAt) {
      group.createdAt = report.created_at;
    }
    group.turns.push(report);
  }
  const result = [...groups.values()];
  for (const group of result) {
    group.turns.sort((a, b) => a.turn - b.turn);
  }
  result.sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
  return result;
}

function UsagePanel({ usage }: { usage: PromptReportUsage }) {
  const metrics: [string, string][] = [
    ["输入词元", usage.input_tokens.toLocaleString()],
    ["输出词元", usage.output_tokens.toLocaleString()],
    ["缓存读取", usage.cache_read.toLocaleString()],
    ["缓存写入", usage.cache_write.toLocaleString()],
    ["词元总数", usage.total_tokens.toLocaleString()],
    ["缓存命中率", `${usage.cache_hit_rate_percent.toFixed(1)}%`],
    [
      "费用",
      usage.cost_usd === null ? "n/a" : `$${usage.cost_usd.toFixed(4)}`,
    ],
  ];

  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-7">
      {metrics.map(([metricLabel, metricValue]) => (
        <div
          key={metricLabel}
          className="rounded-lg border border-zinc-800 bg-zinc-950 p-2"
        >
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">
            {metricLabel}
          </div>
          <div className="mt-1 font-mono text-sm text-zinc-100">
            {metricValue}
          </div>
        </div>
      ))}
    </div>
  );
}

function PromptReportsPage() {
  const [reports, setReports] = useState<PromptReportSummary[]>([]);
  const [totalSizeBytes, setTotalSizeBytes] = useState(0);
  const [reportsDirectory, setReportsDirectory] = useState("");
  const [selectedFilename, setSelectedFilename] = useState<string | null>(null);
  const [content, setContent] = useState<PromptReportContent | null>(null);
  const [showRawJson, setShowRawJson] = useState(false);
  const [isLoadingList, setIsLoadingList] = useState(true);
  const [isLoadingContent, setIsLoadingContent] = useState(false);
  const [isPruning, setIsPruning] = useState(false);

  const sessionGroups = useMemo(() => groupBySession(reports), [reports]);

  const fetchReports = useCallback(async (): Promise<
    PromptReportSummary[]
  > => {
    setIsLoadingList(true);
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/prompt-reports`);
      const data: PromptReportListResponse = await response.json();
      setReports(data.reports);
      setTotalSizeBytes(data.total_size_bytes);
      setReportsDirectory(data.reports_directory);
      return data.reports;
    } catch (error) {
      console.error("获取提示词报告失败", error);
      toast.error("加载提示词报告失败。");
      return [];
    } finally {
      setIsLoadingList(false);
    }
  }, []);

  const openReport = useCallback(async (filename: string) => {
    setSelectedFilename(filename);
    setShowRawJson(false);
    setIsLoadingContent(true);
    try {
      const response = await fetch(
        `${HTTP_BACKEND_URL}/prompt-reports/content?filename=${encodeURIComponent(
          filename
        )}`
      );
      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.detail || "请求失败");
      }
      setContent(await response.json());
    } catch (error) {
      console.error("获取报告内容失败", error);
      setContent(null);
      toast.error("加载报告失败。");
    } finally {
      setIsLoadingContent(false);
    }
  }, []);

  useEffect(() => {
    fetchReports().then((loadedReports) => {
      if (loadedReports.length > 0) {
        const groups = groupBySession(loadedReports);
        openReport(groups[0].turns[0].filename);
      }
    });
  }, [fetchReports, openReport]);

  const handlePrune = async () => {
    if (
      !window.confirm(
        "删除 7 天前的全部报告及旧任务日志？"
      )
    ) {
      return;
    }

    setIsPruning(true);
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/prompt-reports/prune`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ max_age_days: 7 }),
      });
      if (!response.ok) throw new Error("清理请求失败");
      const data = await response.json();
      toast.success(
        `已清理 ${data.deleted_count} 项报告，释放空间 ${formatBytes(data.freed_bytes)}。`,
        { duration: 8000 }
      );
      const refreshed = await fetchReports();
      if (
        selectedFilename &&
        !refreshed.some((report) => report.filename === selectedFilename)
      ) {
        setSelectedFilename(null);
        setContent(null);
      }
    } catch (error) {
      console.error("清理提示词报告失败", error);
      toast.error("清理报告失败。");
    } finally {
      setIsPruning(false);
    }
  };

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-zinc-950 text-white">
      <div className="shrink-0">
        <EvalNavigation />
      </div>
      <div className="mx-auto flex min-h-0 w-full max-w-[1600px] flex-1 flex-col gap-3 px-4 py-4">
        <div className="shrink-0 rounded-2xl border border-zinc-800 bg-zinc-900/80 px-5 py-4">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-2xl font-semibold tracking-tight">

                提示词报告
              </h1>
              <p className="mt-1 max-w-3xl text-sm leading-6 text-zinc-300">

                启用以下配置后，每次模型请求均记录为 JSON 报告：{" "}
                <code className="rounded bg-zinc-800 px-1.5 py-0.5 text-zinc-100">
                  PROMPT_REPORTS_ENABLED=1
                </code>{" "}

                。{" "}
                {reportsDirectory && (
                  <span className="font-mono text-xs text-zinc-500">
                    {reportsDirectory}
                  </span>
                )}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <div className="rounded-xl border border-zinc-700 bg-zinc-950 px-4 py-2 text-right">
                <div className="text-[11px] uppercase tracking-wide text-zinc-500">

                  占用空间总计
                </div>
                <div className="font-mono text-lg text-zinc-100">
                  {formatBytes(totalSizeBytes)}
                </div>
              </div>
              <button
                onClick={handlePrune}
                disabled={isPruning}
                className="rounded-lg border border-red-800 bg-red-950/60 px-3 py-2 text-sm text-red-200 transition-colors hover:bg-red-900/60 disabled:opacity-50"
              >
                {isPruning ? "正在清理…" : "清理 7 天前的报告"}
              </button>
            </div>
          </div>
        </div>

        <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
          <section className="min-h-0 overflow-y-auto rounded-2xl border border-zinc-800 bg-zinc-900/60 p-2">
            <div className="px-2 py-1 text-xs uppercase tracking-wide text-zinc-500">
              {isLoadingList
                ? "加载中…"
                : `${sessionGroups.length} 次请求 · ${reports.length} 轮调用`}
            </div>
            {!isLoadingList && reports.length === 0 && (
              <p className="px-2 py-4 text-sm text-zinc-400">

                暂无报告。启用后端配置{" "}
                <code className="rounded bg-zinc-800 px-1 py-0.5">
                  PROMPT_REPORTS_ENABLED=1
                </code>{" "}

                后生成代码即可查看。
              </p>
            )}
            {sessionGroups.map((group) => (
              <div
                key={group.sessionId}
                className="mb-2 rounded-xl border border-zinc-800 bg-zinc-950/50 px-3 py-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <span
                    className={`rounded-full border px-2 py-0.5 text-[11px] ${providerBadgeClass(
                      group.provider
                    )}`}
                  >
                    {providerLabel(group.provider)}
                  </span>
                  <span className="font-mono text-[11px] text-zinc-500">
                    {formatBytes(group.sizeBytes)}
                  </span>
                </div>
                <div className="mt-1 truncate font-mono text-sm text-zinc-100">
                  {modelLabel(group.model)}
                </div>
                <div className="mt-0.5 flex items-center justify-between gap-2 text-[11px]">
                  <span className="text-zinc-500">
                    {formatTimestamp(group.createdAt)}
                  </span>
                  <span
                    className="font-mono font-medium text-emerald-400"
                    title="此请求所有轮次的总费用"
                  >
                    {formatCost(group.costUsd)}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-1">
                  {group.turns.map((turnReport) => (
                    <button
                      key={turnReport.filename}
                      onClick={() => openReport(turnReport.filename)}
                      className={`rounded-lg border px-2 py-1 text-xs transition-colors ${
                        turnReport.filename === selectedFilename
                          ? "border-blue-600 bg-blue-950/60 text-blue-100"
                          : "border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                      }`}
                    >

                      轮次 {turnReport.turn}
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </section>

          <section className="min-h-0 overflow-y-auto rounded-2xl border border-zinc-800 bg-zinc-900/60 p-4">
            {!selectedFilename && (
              <p className="py-8 text-center text-sm text-zinc-400">

                请选择报告查看请求详情。
              </p>
            )}
            {selectedFilename && isLoadingContent && (
              <p className="py-8 text-center text-sm text-zinc-400">

                正在加载报告…
              </p>
            )}
            {selectedFilename && !isLoadingContent && content && (
              <div className="flex flex-col gap-4">
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`rounded-full border px-2.5 py-0.5 text-xs ${providerBadgeClass(
                      content.provider
                    )}`}
                  >
                    {providerLabel(content.provider)}
                  </span>
                  <span className="font-mono text-sm text-zinc-100">
                    {modelLabel(content.model)}
                  </span>
                  <span className="font-mono text-xs text-zinc-500">

                    接口： {content.api_model_name}  · 会话{" "}
                    {content.session_id}  · 轮次 {content.turn} ·{" "}
                    {formatTimestamp(content.created_at)}
                  </span>
                </div>

                {content.usage ? (
                  <UsagePanel usage={content.usage} />
                ) : (
                  <p className="text-xs italic text-zinc-500">

                    此请求暂无用量信息，当前轮次可能尚未完成。
                  </p>
                )}

                <div className="rounded-xl border border-zinc-800 bg-zinc-950/70 p-3">
                  <div className="mb-2 flex items-center justify-between">
                    <h2 className="text-sm font-semibold text-zinc-200">

                      请求结构
                    </h2>
                    <button
                      onClick={() => setShowRawJson(!showRawJson)}
                      className="text-xs text-blue-400 hover:text-blue-300"
                    >
                      {showRawJson ? "显示树状结构" : "显示原始数据"}
                    </button>
                  </div>
                  {showRawJson ? (
                    <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-zinc-950 p-3 text-xs text-zinc-300">
                      {JSON.stringify(content.request, null, 2)}
                    </pre>
                  ) : (
                    <JsonNode label="request" value={content.request} depth={0} />
                  )}
                </div>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

export default PromptReportsPage;
