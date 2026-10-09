import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";
import { BsBoxArrowUpRight } from "react-icons/bs";
import { useSearchParams } from "react-router-dom";

import { HTTP_BACKEND_URL } from "../../config";
import EvalNavigation from "./EvalNavigation";
import AgentRunTimeline, {
  ExpandMode,
  ExpandSignal,
  RunEvent,
  TimelineViewMode,
} from "./agent-run-timeline";
import { LightboxProvider } from "./image-lightbox";
import {
  entryPointLabel,
  formatBytes,
  formatCost,
  formatMs,
  formatTimestamp,
  providerBadgeClass,
  providerLabel,
  modelLabel,
  stackLabel,
} from "./report-format";

interface AgentRunSummary {
  run_id: string;
  generation_id: string;
  variant_index: number;
  entry_point: string;
  provider: string | null;
  model: string | null;
  api_model_name: string | null;
  stack: string | null;
  input_mode: string | null;
  generation_type: string | null;
  status: string;
  error: string | null;
  created_at: string;
  completed_at: string | null;
  total_duration_ms: number | null;
  llm_time_ms: number | null;
  tool_time_ms: number | null;
  num_llm_calls: number;
  num_tool_calls: number;
  input_tokens: number | null;
  output_tokens: number | null;
  cache_read_tokens: number | null;
  cache_write_tokens: number | null;
  total_tokens: number | null;
  total_cost_usd: number | null;
  has_unpriced_calls: boolean;
  size_bytes: number | null;
  eval_session: string | null;
  eval_set: string | null;
  input_file: string | null;
  input_image_sha256: string | null;
}

interface AgentRunListResponse {
  runs: AgentRunSummary[];
  total_size_bytes: number;
  runs_directory: string;
}

interface AgentRunDetailResponse {
  run: AgentRunSummary;
  events: RunEvent[];
}

interface GenerationGroup {
  generationId: string;
  entryPoint: string;
  createdAt: string;
  costUsd: number | null;
  runs: AgentRunSummary[];
}

function groupByGeneration(runs: AgentRunSummary[]): GenerationGroup[] {
  const groups = new Map<string, GenerationGroup>();
  for (const run of runs) {
    let group = groups.get(run.generation_id);
    if (!group) {
      group = {
        generationId: run.generation_id,
        entryPoint: run.entry_point,
        createdAt: run.created_at,
        costUsd: null,
        runs: [],
      };
      groups.set(run.generation_id, group);
    }
    if (typeof run.total_cost_usd === "number") {
      group.costUsd = (group.costUsd ?? 0) + run.total_cost_usd;
    }
    if (run.created_at > group.createdAt) group.createdAt = run.created_at;
    group.runs.push(run);
  }
  const result = [...groups.values()];
  for (const group of result) {
    group.runs.sort((a, b) => a.variant_index - b.variant_index);
  }
  result.sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
  return result;
}

function statusDotClass(status: string): string {
  if (status === "completed") return "bg-emerald-400";
  if (status === "failed") return "bg-red-400";
  return "bg-amber-400 animate-pulse";
}

function runOutputUrl(runId: string): string {
  return `${HTTP_BACKEND_URL}/agent-runs/${encodeURIComponent(runId)}/output`;
}

function StatChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="whitespace-nowrap">
      <span className="font-mono text-zinc-100">{value}</span>{" "}
      <span className="text-zinc-500">{label}</span>
    </span>
  );
}

function SummaryStats({ run }: { run: AgentRunSummary }) {
  const costLabel =
    formatCost(run.total_cost_usd) + (run.has_unpriced_calls ? "*" : "");
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-zinc-800 bg-zinc-950 px-2.5 py-1.5 text-xs"
      title={
        run.has_unpriced_calls
          ? "* 部分请求缺少价格记录，实际费用可能更高"
          : undefined
      }
    >
      <StatChip label="费用" value={costLabel} />
      <StatChip label="总耗时" value={formatMs(run.total_duration_ms)} />
      <StatChip label="模型耗时" value={formatMs(run.llm_time_ms)} />
      <StatChip label="工具耗时" value={formatMs(run.tool_time_ms)} />
      <StatChip label="模型调用" value={String(run.num_llm_calls)} />
      <StatChip label="工具调用" value={String(run.num_tool_calls)} />
      <StatChip label="输入令牌" value={(run.input_tokens ?? 0).toLocaleString()} />
      <StatChip label="输出令牌" value={(run.output_tokens ?? 0).toLocaleString()} />
      <StatChip
        label="缓存读取"
        value={(run.cache_read_tokens ?? 0).toLocaleString()}
      />
      <StatChip
        label="总令牌数"
        value={(run.total_tokens ?? 0).toLocaleString()}
      />
    </div>
  );
}

function AgentRunsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [runs, setRuns] = useState<AgentRunSummary[]>([]);
  const [totalSizeBytes, setTotalSizeBytes] = useState(0);
  const [runsDirectory, setRunsDirectory] = useState("");
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [detail, setDetail] = useState<AgentRunDetailResponse | null>(null);
  const [activeTab, setActiveTab] = useState<"timeline" | "output" | "raw">(
    "timeline"
  );
  const [viewMode, setViewMode] = useState<TimelineViewMode>("readable");
  const [expandSignal, setExpandSignal] = useState<ExpandSignal>({
    mode: "best",
    version: 0,
  });
  const [includeDeltas, setIncludeDeltas] = useState(false);
  const [isLoadingList, setIsLoadingList] = useState(true);
  const [isLoadingDetail, setIsLoadingDetail] = useState(false);
  const [isPruning, setIsPruning] = useState(false);

  const generationGroups = useMemo(() => groupByGeneration(runs), [runs]);

  const fetchRuns = useCallback(async (): Promise<AgentRunSummary[]> => {
    setIsLoadingList(true);
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/agent-runs`);
      const data: AgentRunListResponse = await response.json();
      setRuns(data.runs);
      setTotalSizeBytes(data.total_size_bytes);
      setRunsDirectory(data.runs_directory);
      return data.runs;
    } catch (error) {
      console.error("获取任务记录失败", error);
      toast.error("加载任务记录失败。");
      return [];
    } finally {
      setIsLoadingList(false);
    }
  }, []);

  const openRun = useCallback(
    async (runId: string, withDeltas: boolean = false) => {
      setSelectedRunId(runId);
      setActiveTab("timeline");
      setIncludeDeltas(withDeltas);
      setExpandSignal({ mode: "best", version: 0 });
      setSearchParams({ run: runId }, { replace: true });
      setIsLoadingDetail(true);
      try {
        const response = await fetch(
          `${HTTP_BACKEND_URL}/agent-runs/${encodeURIComponent(
            runId
          )}?include_stream_deltas=${withDeltas}`
        );
        if (!response.ok) {
          const data = await response.json();
          throw new Error(data.detail || "请求失败");
        }
        setDetail(await response.json());
      } catch (error) {
        console.error("获取任务详情失败", error);
        setDetail(null);
        toast.error("加载任务详情失败。");
      } finally {
        setIsLoadingDetail(false);
      }
    },
    [setSearchParams]
  );

  useEffect(() => {
    fetchRuns().then((loadedRuns) => {
      // Deep link (?run=...) wins over the newest-run default so matrix
      // cells and shared URLs land on the intended run.
      const requestedRun = searchParams.get("run");
      if (requestedRun) {
        openRun(requestedRun);
      } else if (loadedRuns.length > 0) {
        const groups = groupByGeneration(loadedRuns);
        openRun(groups[0].runs[0].run_id);
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchRuns, openRun]);

  const handlePrune = async () => {
    if (!window.confirm("删除 7 天前的全部任务记录？")) {
      return;
    }
    setIsPruning(true);
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/agent-runs/prune`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ max_age_days: 7 }),
      });
      if (!response.ok) throw new Error("清理请求失败");
      const data = await response.json();
      toast.success(
        `已清理 ${data.deleted_count} 条任务记录，释放空间 ${formatBytes(data.freed_bytes)}。`,
        { duration: 8000 }
      );
      const refreshed = await fetchRuns();
      if (
        selectedRunId &&
        !refreshed.some((run) => run.run_id === selectedRunId)
      ) {
        setSelectedRunId(null);
        setDetail(null);
      }
    } catch (error) {
      console.error("清理任务记录失败", error);
      toast.error("清理任务记录失败。");
    } finally {
      setIsPruning(false);
    }
  };

  const selectedRun = detail?.run ?? null;

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-zinc-950 text-white">
      <div className="shrink-0">
        <EvalNavigation />
      </div>
      <div className="mx-auto flex min-h-0 w-full max-w-[1600px] flex-1 flex-col gap-2 px-4 py-2">
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 rounded-xl border border-zinc-800 bg-zinc-900/80 px-4 py-2">
          <div className="flex min-w-0 items-baseline gap-3">
            <h1 className="text-base font-semibold tracking-tight">

              智能体任务
            </h1>
            {runsDirectory && (
              <span
                className="hidden truncate font-mono text-[11px] text-zinc-500 md:inline"
                title={runsDirectory}
              >
                {runsDirectory}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 text-xs">
            <span className="font-mono text-zinc-400">
              {formatBytes(totalSizeBytes)}
            </span>
            <button
              onClick={handlePrune}
              disabled={isPruning}
              className="rounded-md border border-red-800 bg-red-950/60 px-2 py-1 text-red-200 transition-colors hover:bg-red-900/60 disabled:opacity-50"
            >
              {isPruning ? "正在清理…" : "清理 7 天前记录"}
            </button>
          </div>
        </div>

        <div className="grid min-h-0 flex-1 gap-3 lg:grid-cols-[340px_minmax(0,1fr)]">
          <section className="min-h-0 overflow-y-auto rounded-xl border border-zinc-800 bg-zinc-900/60 p-2">
            <div className="px-2 py-1 text-xs uppercase tracking-wide text-zinc-500">
              {isLoadingList
                ? "加载中…"
                : `${generationGroups.length} 次生成 · ${runs.length} 条任务记录`}
            </div>
            {!isLoadingList && runs.length === 0 && (
              <p className="px-2 py-4 text-sm text-zinc-400">

                暂无任务记录。启用后端配置{" "}
                <code className="rounded bg-zinc-800 px-1 py-0.5">
                  PROMPT_REPORTS_ENABLED=1
                </code>{" "}

                后生成代码即可查看。
              </p>
            )}
            {generationGroups.map((group) => (
              <div
                key={group.generationId}
                className="mb-2 rounded-xl border border-zinc-800 bg-zinc-950/50 px-3 py-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="rounded-full border border-zinc-600 bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-200">
                    {entryPointLabel(group.entryPoint)}
                  </span>
                  <span className="text-[11px] text-zinc-500">
                    {formatTimestamp(group.createdAt)}
                  </span>
                  <span
                    className="font-mono text-[11px] font-medium text-emerald-400"
                    title="所有方案的总费用"
                  >
                    {formatCost(group.costUsd)}
                  </span>
                </div>
                <div className="mt-2 flex flex-col gap-1">
                  {group.runs.map((run) => (
                    <div
                      key={run.run_id}
                      onClick={() => openRun(run.run_id)}
                      className={`flex cursor-pointer items-center gap-2 rounded-lg border px-2 py-1.5 text-left text-xs transition-colors ${
                        run.run_id === selectedRunId
                          ? "border-blue-600 bg-blue-950/60 text-blue-100"
                          : "border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                      }`}
                    >
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${statusDotClass(
                          run.status
                        )}`}
                      />
                      <span className="shrink-0 font-mono text-zinc-500">
                        方案 {run.variant_index + 1}
                      </span>
                      <span className="min-w-0 flex-1 truncate font-mono">
                        {modelLabel(run.model)}
                      </span>
                      <span className="shrink-0 font-mono text-zinc-500">
                        {formatMs(run.total_duration_ms)}
                      </span>
                      <a
                        href={runOutputUrl(run.run_id)}
                        target="_blank"
                        rel="noreferrer"
                        onClick={(e) => e.stopPropagation()}
                        title="在新标签页打开记录的 HTML"
                        className="shrink-0 rounded p-1 text-zinc-500 hover:bg-zinc-700 hover:text-zinc-200"
                      >
                        <BsBoxArrowUpRight />
                      </a>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </section>

          <section className="min-h-0 overflow-y-auto rounded-xl border border-zinc-800 bg-zinc-900/60 p-3">
            {!selectedRunId && (
              <p className="py-8 text-center text-sm text-zinc-400">

                请选择一个任务查看详情。
              </p>
            )}
            {selectedRunId && isLoadingDetail && (
              <p className="py-8 text-center text-sm text-zinc-400">

                正在加载任务…
              </p>
            )}
            {selectedRunId && !isLoadingDetail && detail && selectedRun && (
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  {selectedRun.provider && (
                    <span
                      className={`rounded-full border px-2 py-0.5 ${providerBadgeClass(
                        selectedRun.provider
                      )}`}
                    >
                      {providerLabel(selectedRun.provider)}
                    </span>
                  )}
                  <span className="font-mono text-sm text-zinc-100">
                    {modelLabel(selectedRun.model)}
                  </span>
                  <span className="font-mono text-[11px] text-zinc-500">
                    {stackLabel(selectedRun.stack)} · {entryPointLabel(selectedRun.generation_type ?? "")} · 方案{" "}
                    {selectedRun.variant_index + 1} ·{" "}
                    {formatTimestamp(selectedRun.created_at)}
                  </span>
                  <a
                    href={runOutputUrl(selectedRun.run_id)}
                    target="_blank"
                    rel="noreferrer"
                    className="ml-auto flex items-center gap-1.5 rounded-md border border-zinc-700 px-2 py-1 text-zinc-300 transition-colors hover:bg-zinc-800"
                  >
                    <BsBoxArrowUpRight />  打开 HTML
                  </a>
                </div>

                {selectedRun.status === "failed" && (
                  <div className="rounded-lg border border-red-900 bg-red-950/40 px-2.5 py-1.5 font-mono text-xs text-red-200">
                    {selectedRun.error ?? "任务失败"}
                  </div>
                )}

                <SummaryStats run={selectedRun} />

                <div className="flex flex-wrap items-center gap-2 border-b border-zinc-800 pb-2 text-xs">
                  {(["timeline", "output", "raw"] as const).map((tab) => (
                    <button
                      key={tab}
                      onClick={() => setActiveTab(tab)}
                      className={`rounded-md px-2.5 py-1 capitalize transition-colors ${
                        activeTab === tab
                          ? "bg-zinc-800 text-zinc-100"
                          : "text-zinc-400 hover:text-zinc-200"
                      }`}
                    >
                      {{ raw: "原始事件", timeline: "执行过程", output: "输出" }[tab]}
                    </button>
                  ))}
                  {activeTab === "timeline" && (
                    <>
                      <div className="mx-1 h-4 w-px bg-zinc-800" />
                      <div className="flex overflow-hidden rounded-md border border-zinc-700">
                        {(["readable", "raw"] as const).map((mode) => (
                          <button
                            key={mode}
                            onClick={() => setViewMode(mode)}
                            className={`px-2.5 py-1 capitalize transition-colors ${
                              viewMode === mode
                                ? "bg-zinc-700 text-zinc-100"
                                : "text-zinc-400 hover:text-zinc-200"
                            }`}
                          >
                            {{ readable: "易读视图", raw: "原始数据" }[mode]}
                          </button>
                        ))}
                      </div>
                      <div className="flex overflow-hidden rounded-md border border-zinc-700">
                        {(
                          [
                            ["expand", "展开"],
                            ["best", "最佳方案"],
                            ["collapse", "收起"],
                          ] as [ExpandMode, string][]
                        ).map(([mode, label]) => (
                          <button
                            key={mode}
                            title={
                              mode === "best"
                                ? "展开除文件创建与编辑外的全部内容"
                                : undefined
                            }
                            onClick={() =>
                              setExpandSignal((s) => ({
                                mode,
                                version: s.version + 1,
                              }))
                            }
                            className={`px-2.5 py-1 transition-colors ${
                              expandSignal.mode === mode
                                ? "bg-zinc-700 text-zinc-100"
                                : "text-zinc-400 hover:text-zinc-200"
                            }`}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
                      <label className="ml-auto flex items-center gap-1.5 text-zinc-400">
                        <input
                          type="checkbox"
                          checked={includeDeltas}
                          onChange={(e) =>
                            openRun(selectedRunId, e.target.checked)
                          }
                        />

                        流式片段
                      </label>
                    </>
                  )}
                </div>

                {activeTab === "timeline" && (
                  <LightboxProvider>
                    <AgentRunTimeline
                      key={`${selectedRunId}-${viewMode}`}
                      events={detail.events}
                      viewMode={viewMode}
                      expandSignal={expandSignal}
                    />
                  </LightboxProvider>
                )}
                {activeTab === "output" && (
                  <iframe
                    title="记录的最终结果"
                    src={runOutputUrl(selectedRunId)}
                    className="h-[75vh] w-full rounded-xl border border-zinc-800 bg-white"
                  />
                )}
                {activeTab === "raw" && (
                  <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-zinc-950 p-3 text-xs text-zinc-300">
                    {JSON.stringify(detail.events, null, 2)}
                  </pre>
                )}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

export default AgentRunsPage;
