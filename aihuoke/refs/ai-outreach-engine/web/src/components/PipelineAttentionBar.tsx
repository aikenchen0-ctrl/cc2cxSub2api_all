import type { Overview } from "../lib/api";
import { Button } from "./ui";

/**
 * Compact status strip for Setup / Audiences. Full retry UI lives on Activity.
 */
export function PipelineAttentionBar({
  overview,
  onOpenActivity,
  stages,
}: {
  overview: Overview;
  onOpenActivity: () => void;
  /** When set, only count jobs in these stages (e.g. prospecting on Audiences). */
  stages?: Set<string>;
}) {
  const active = (overview.pipeline?.active ?? []).filter((job) =>
    stages ? stages.has(job.stage) : true,
  );
  const failed = (overview.pipeline?.failed ?? []).filter((job) =>
    stages ? stages.has(job.stage) : true,
  );

  if (active.length === 0 && failed.length === 0) return null;

  const running = active.find((job) => job.status === "running");
  const headline =
    failed.length > 0
      ? `${failed.length} step${failed.length === 1 ? "" : "s"} need attention`
      : `${active.length} step${active.length === 1 ? "" : "s"} in progress`;

  const detail =
    failed.length > 0
      ? failed[0]?.lastError
        ? `${failed[0].label}: ${failed[0].lastError}`
        : `${failed[0]?.label ?? "Pipeline"} failed`
      : running
        ? `Running now · ${running.label} · ${running.target}`
        : active[0]
          ? `Next · ${active[0].label} · ${active[0].target}`
          : "Pipeline is working";

  return (
    <div
      className={`flex flex-col gap-3 rounded-2xl border px-4 py-3 sm:flex-row sm:items-center sm:justify-between ${
        failed.length > 0
          ? "border-bad/20 bg-bad-soft/60"
          : "border-accent/20 bg-accent-soft/50"
      }`}
    >
      <div className="min-w-0">
        <p
          className={`text-[13px] font-semibold ${failed.length > 0 ? "text-bad" : "text-accent"}`}
        >
          {headline}
        </p>
        <p className="mt-0.5 truncate text-[11px] text-ink-soft" title={detail}>
          {detail}
        </p>
      </div>
      <Button
        onClick={onOpenActivity}
        title="Open Activity for live progress and per-step retry"
        className="shrink-0"
      >
        {failed.length > 0 ? "Review & retry" : "View progress"}
      </Button>
    </div>
  );
}
