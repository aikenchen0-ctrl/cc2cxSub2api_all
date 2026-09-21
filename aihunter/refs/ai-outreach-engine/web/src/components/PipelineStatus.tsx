import { useMemo, useState } from "react";
import { api, type Overview, type PipelineJob } from "../lib/api";
import { Button, Card, Empty, Pill } from "./ui";

function groupByStage(jobs: PipelineJob[]) {
  const groups = new Map<string, PipelineJob[]>();
  for (const job of jobs) {
    const list = groups.get(job.stage) ?? [];
    list.push(job);
    groups.set(job.stage, list);
  }
  return [...groups.entries()].map(([stage, items]) => ({
    stage,
    label: items[0]?.label ?? stage,
    items,
  }));
}

function statusTone(status: PipelineJob["status"]): "good" | "warn" | "bad" | "info" | "neutral" {
  if (status === "failed") return "bad";
  if (status === "running") return "info";
  if (status === "pending") return "warn";
  if (status === "done") return "good";
  return "neutral";
}

function JobRow({
  job,
  busyId,
  onRetry,
  onDismiss,
}: {
  job: PipelineJob;
  busyId: number | "all" | "dismiss-all" | string | null;
  onRetry: (jobId: number) => void;
  onDismiss: (jobId: number) => void;
}) {
  const runAfterMs = Date.parse(job.runAfter.includes("T") ? job.runAfter : `${job.runAfter.replace(" ", "T")}Z`);
  const waiting =
    job.status === "pending" && Number.isFinite(runAfterMs) && runAfterMs > Date.now();

  return (
    <li className="rounded-2xl border border-line bg-surface-2/50 px-3.5 py-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[13px] font-semibold text-ink">{job.label}</span>
            <Pill tone={statusTone(job.status)}>
              {job.status === "running"
                ? "running now"
                : waiting
                  ? "waiting to retry"
                  : job.status}
            </Pill>
            <span className="text-[11px] tabular-nums text-ink-faint">
              attempt {Math.max(job.attempts, job.status === "failed" ? job.maxAttempts : 0)}/
              {job.maxAttempts}
            </span>
          </div>
          <p className="truncate text-xs text-ink-soft" title={job.target}>
            {job.target}
          </p>
          {job.lastError && (
            <p className="text-[11px] leading-relaxed text-bad">{job.lastError}</p>
          )}
        </div>
        {job.status === "failed" && (
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button
              disabled={busyId !== null}
              onClick={() => onRetry(job.id)}
              title="Requeue only this failed step. Downstream work already done is left alone."
            >
              {busyId === job.id ? "Retrying…" : "Retry"}
            </Button>
            <Button
              variant="ghost"
              disabled={busyId !== null}
              onClick={() => onDismiss(job.id)}
              title="Hide from Setup, Audiences, and Needs attention. Stays in the activity log."
            >
              {busyId === `dismiss:${job.id}` ? "Dismissing…" : "Dismiss"}
            </Button>
          </div>
        )}
      </div>
    </li>
  );
}

export function PipelineStatus({
  overview,
  onRefresh,
  onError,
  onNotice,
}: {
  overview: Overview;
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice?: (message: string) => void;
}) {
  const [busyId, setBusyId] = useState<number | "all" | "dismiss-all" | string | null>(null);
  const [expandedStages, setExpandedStages] = useState<Record<string, boolean>>({});

  const pipeline = overview.pipeline;
  const active = pipeline?.active ?? [];
  const failed = pipeline?.failed ?? [];
  const failedCount = pipeline?.failedCount ?? failed.length;
  const pendingJobs = pipeline?.pendingJobs ?? overview.counts.pendingJobs;

  const failedGroups = useMemo(() => groupByStage(failed), [failed]);
  const activeGroups = useMemo(() => groupByStage(active), [active]);

  if (active.length === 0 && failed.length === 0) {
    return (
      <Card title="Pipeline">
        <Empty>No jobs in flight. Failures and live work will show up here.</Empty>
      </Card>
    );
  }

  async function retry(opts?: { jobIds?: number[]; stage?: string }) {
    const key =
      opts?.jobIds?.length === 1
        ? opts.jobIds[0]!
        : opts?.stage
          ? `stage:${opts.stage}`
          : "all";
    setBusyId(key);
    try {
      const result = await api.retryJobs(overview.project.id, opts);
      const label =
        result.scope === "selected"
          ? `Requeued ${result.retried} failed step${result.retried === 1 ? "" : "s"} (only those jobs — not the whole pipeline).`
          : result.scope === "stage"
            ? `Requeued ${result.retried} failed “${opts?.stage}” step${result.retried === 1 ? "" : "s"}.`
            : `Requeued ${result.retried} failed step${result.retried === 1 ? "" : "s"}. Already-finished work is not rerun.`;
      onNotice?.(label);
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Retry failed");
    } finally {
      setBusyId(null);
    }
  }

  async function dismiss(opts: { jobIds?: number[]; all?: boolean }) {
    const key =
      opts.jobIds?.length === 1
        ? `dismiss:${opts.jobIds[0]}`
        : "dismiss-all";
    setBusyId(key);
    try {
      const result = await api.dismissJobs(overview.project.id, opts);
      onNotice?.(
        result.scope === "all_failed"
          ? `Marked ${result.dismissed} failure${result.dismissed === 1 ? "" : "s"} as resolved. Hidden from Setup/Audiences/Needs attention; still in the activity log.`
          : `Marked ${result.dismissed} failure${result.dismissed === 1 ? "" : "s"} as resolved.`,
      );
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Dismiss failed");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="space-y-4">
      {active.length > 0 && (
        <Card
          title="In progress"
          action={
            <span className="text-xs font-semibold tabular-nums text-accent">
              {pendingJobs} active
            </span>
          }
        >
          <p className="mb-3 text-xs leading-relaxed text-ink-soft">
            Live pipeline work. This updates as the runner claims each step — search, crawl, LLM,
            verify, and send each appear here when they run.
          </p>
          <div className="space-y-3">
            {activeGroups.map((group) => {
              const showAll = expandedStages[`active:${group.stage}`] ?? group.items.length <= 4;
              const visible = showAll ? group.items : group.items.slice(0, 3);
              return (
                <div key={`active-${group.stage}`} className="space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <div className="text-[11px] font-semibold tracking-[0.08em] text-ink-faint uppercase">
                      {group.label}
                      <span className="ml-2 tabular-nums opacity-70">{group.items.length}</span>
                    </div>
                  </div>
                  <ul className="space-y-2">
                    {visible.map((job) => (
                      <JobRow
                        key={job.id}
                        job={job}
                        busyId={busyId}
                        onRetry={(id) => void retry({ jobIds: [id] })}
                        onDismiss={(id) => void dismiss({ jobIds: [id] })}
                      />
                    ))}
                  </ul>
                  {group.items.length > 3 && (
                    <button
                      type="button"
                      className="text-xs font-semibold text-accent hover:underline"
                      onClick={() =>
                        setExpandedStages((current) => ({
                          ...current,
                          [`active:${group.stage}`]: !showAll,
                        }))
                      }
                    >
                      {showAll ? "Show fewer" : `Show all ${group.items.length}`}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      )}

      {failed.length > 0 && (
        <Card
          title="Needs attention"
          action={
            <div className="flex flex-wrap gap-2">
              <Button
                variant="ghost"
                disabled={busyId !== null}
                onClick={() => void dismiss({ all: true })}
                title="Hide all failures from Setup, Audiences, and this panel. Activity log entries stay."
              >
                {busyId === "dismiss-all"
                  ? "Dismissing…"
                  : `Dismiss all ${failedCount}`}
              </Button>
              <Button
                disabled={busyId !== null}
                onClick={() =>
                  void (failedCount > failed.length
                    ? retry()
                    : retry({ jobIds: failed.map((job) => job.id) }))
                }
                title="Requeues these terminal failed jobs only. Successful steps are not rerun."
              >
                {busyId === "all" ? "Retrying…" : `Retry all ${failedCount} failed`}
              </Button>
            </div>
          }
        >
          <p className="mb-3 text-xs leading-relaxed text-ink-soft">
            Each row is one failed unit of work. Retry requeues{" "}
            <span className="font-semibold text-ink">only that step</span>. Dismiss marks it
            resolved so Setup, Audiences, and this banner stop prompting — the failure still
            appears in the activity log below.
          </p>
          {failedCount > failed.length && (
            <p className="mb-3 text-xs text-warn">
              Showing {failed.length} of {failedCount} failed jobs. Retry/Dismiss all still covers
              every open failure.
            </p>
          )}
          <div className="space-y-4">
            {failedGroups.map((group) => {
              const showAll = expandedStages[`failed:${group.stage}`] ?? group.items.length <= 5;
              const visible = showAll ? group.items : group.items.slice(0, 5);
              return (
                <div key={`failed-${group.stage}`} className="space-y-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="text-[11px] font-semibold tracking-[0.08em] text-ink-faint uppercase">
                      {group.label}
                      <span className="ml-2 tabular-nums text-bad">{group.items.length} failed</span>
                    </div>
                    {group.items.length > 1 && (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          variant="ghost"
                          disabled={busyId !== null}
                          onClick={() =>
                            void dismiss({ jobIds: group.items.map((job) => job.id) })
                          }
                          title={`Dismiss failed ${group.label} jobs`}
                        >
                          Dismiss {group.items.length}
                        </Button>
                        <Button
                          disabled={busyId !== null}
                          onClick={() => void retry({ stage: group.stage })}
                          title={`Retry only failed ${group.label} jobs`}
                        >
                          {busyId === `stage:${group.stage}`
                            ? "Retrying…"
                            : `Retry ${group.items.length}`}
                        </Button>
                      </div>
                    )}
                  </div>
                  <ul className="space-y-2">
                    {visible.map((job) => (
                      <JobRow
                        key={job.id}
                        job={job}
                        busyId={busyId}
                        onRetry={(id) => void retry({ jobIds: [id] })}
                        onDismiss={(id) => void dismiss({ jobIds: [id] })}
                      />
                    ))}
                  </ul>
                  {group.items.length > 5 && (
                    <button
                      type="button"
                      className="text-xs font-semibold text-accent hover:underline"
                      onClick={() =>
                        setExpandedStages((current) => ({
                          ...current,
                          [`failed:${group.stage}`]: !showAll,
                        }))
                      }
                    >
                      {showAll ? "Show fewer" : `Show all ${group.items.length} failures`}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      )}
    </div>
  );
}
