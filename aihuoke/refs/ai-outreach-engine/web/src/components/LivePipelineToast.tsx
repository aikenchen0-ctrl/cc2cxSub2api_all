import { useEffect, useMemo, useRef, useState } from "react";
import { api, type Overview } from "../lib/api";
import {
  activeJobs,
  cancelScopeForJobs,
  PROSPECT_STAGES,
  SETUP_STAGES,
  summariseActiveJobs,
} from "../lib/pipelineBusy";
import { Button } from "./ui";

/**
 * Persistent bottom-right toast (Sonner-style) that mirrors live pipeline work
 * from SSE/overview refresh, with a Stop control for cooperative cancel.
 */
export function LivePipelineToast({
  overview,
  onRefresh,
  onNotice,
  onError,
}: {
  overview: Overview | null;
  onRefresh: () => void;
  onNotice: (message: string) => void;
  onError: (message: string) => void;
}) {
  const [stopping, setStopping] = useState(false);
  const prevActiveRef = useRef(0);
  const jobs = useMemo(() => activeJobs(overview), [overview]);
  const summary = useMemo(() => summariseActiveJobs(jobs), [jobs]);

  useEffect(() => {
    const prev = prevActiveRef.current;
    prevActiveRef.current = jobs.length;
    if (prev > 0 && jobs.length === 0 && !stopping) {
      onNotice("Background work finished.");
    }
  }, [jobs.length, onNotice, stopping]);

  if (!overview || jobs.length === 0) return null;

  const scope = cancelScopeForJobs(jobs);
  const scopeHint =
    scope === "setup"
      ? "Stop scan & audience generation"
      : scope === "prospecting"
        ? "Stop finding targets & contacts"
        : scope === "send"
          ? "Stop queued sends"
          : "Stop all background work";

  async function stop() {
    if (!overview) return;
    setStopping(true);
    try {
      const result = await api.cancelJobs(overview.project.id, { scope });
      onNotice(
        `Stopped ${result.cancelledPending + result.runningCancelled} step${
          result.cancelledPending + result.runningCancelled === 1 ? "" : "s"
        }.`,
      );
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to stop");
    } finally {
      setStopping(false);
    }
  }

  const accent =
    jobs.some((job) => SETUP_STAGES.has(job.stage)) &&
    !jobs.some((job) => PROSPECT_STAGES.has(job.stage))
      ? "setup"
      : "prospect";

  return (
    <div
      className="animate-rise pointer-events-auto fixed right-4 bottom-4 z-50 w-[min(100%-2rem,22rem)] sm:right-6 sm:bottom-6"
      role="status"
      aria-live="polite"
    >
      <div
        className={`rounded-2xl border bg-surface/95 px-4 py-3 shadow-[var(--shadow-lift)] backdrop-blur-md ${
          accent === "setup" ? "border-sky/25" : "border-accent/25"
        }`}
      >
        <div className="flex items-start gap-3">
          <span
            className={`mt-1 inline-flex h-2.5 w-2.5 shrink-0 rounded-full ${
              summary.running ? "animate-pulse bg-accent" : "bg-ink-faint"
            }`}
            aria-hidden
          />
          <div className="min-w-0 flex-1">
            <p
              className={`text-[13px] font-semibold tracking-tight ${
                accent === "setup" ? "text-sky" : "text-accent"
              }`}
            >
              {summary.headline}
            </p>
            <p className="mt-0.5 truncate text-[11px] text-ink-soft" title={summary.detail}>
              {summary.detail}
            </p>
            {jobs.length > 1 && (
              <ul className="mt-2 max-h-24 space-y-1 overflow-y-auto text-[11px] text-ink-faint">
                {jobs.slice(0, 6).map((job) => (
                  <li key={job.id} className="truncate">
                    <span className="font-medium text-ink-soft">
                      {job.status === "running" ? "▶" : "·"} {job.label}
                    </span>
                    {" · "}
                    {job.target}
                  </li>
                ))}
                {jobs.length > 6 && <li>+{jobs.length - 6} more</li>}
              </ul>
            )}
          </div>
          <Button
            variant="ghost"
            disabled={stopping}
            onClick={() => void stop()}
            title={scopeHint}
            className="shrink-0 !px-2.5 !py-1.5 text-[11px] text-bad hover:bg-bad-soft"
          >
            {stopping ? "Stopping…" : "Stop"}
          </Button>
        </div>
      </div>
    </div>
  );
}
