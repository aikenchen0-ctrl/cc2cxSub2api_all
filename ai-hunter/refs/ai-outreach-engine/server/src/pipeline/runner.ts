import { events, jobs } from "../db/repo";
import { STAGES } from "./stages";
import { JobCancelled, RetryLater } from "./errors";
import {
  clearJobCancelRequested,
  isJobCancelRequested,
  markJobCancelRequested,
  runWithJobContext,
} from "./context";
import { db } from "../db/index";
import type { JobStage } from "../db/types";

const POLL_INTERVAL_MS = 2_000;

const SETUP_STAGES: JobStage[] = [
  "scan_site",
  "generate_audiences",
  "generate_audience_email",
  "regenerate_audience",
];
const PROSPECT_STAGES: JobStage[] = [
  "find_companies",
  "find_contacts",
  "verify_contact",
  "draft_message",
];
const SEND_STAGES: JobStage[] = ["send_message"];

let running = false;
let timer: ReturnType<typeof setTimeout> | null = null;

async function runOne(): Promise<boolean> {
  const job = jobs.claimNext();
  if (!job) return false;

  // Cancelled while still pending/claiming — park it and move on.
  if (isJobCancelRequested(job.id)) {
    jobs.markCancelled(job.id);
    clearJobCancelRequested(job.id);
    events.log("job_cancelled", {
      projectId: job.project_id,
      ref: job.stage,
      data: { jobId: job.id },
    });
    return true;
  }

  const handler = STAGES[job.stage];
  if (!handler) {
    jobs.fail(job, `Unknown stage "${job.stage}"`);
    return true;
  }

  const startedAt = Date.now();
  try {
    const result = await runWithJobContext(
      { jobId: job.id, projectId: job.project_id, stage: job.stage },
      () => handler(job.project_id, JSON.parse(job.payload)),
    );

    if (isJobCancelRequested(job.id)) {
      jobs.markCancelled(job.id);
      clearJobCancelRequested(job.id);
      events.log("job_cancelled", {
        projectId: job.project_id,
        ref: job.stage,
        data: { jobId: job.id, after: "handler" },
      });
      return true;
    }

    jobs.complete(job.id);
    clearJobCancelRequested(job.id);
    events.log("job_done", {
      projectId: job.project_id,
      ref: job.stage,
      data: { jobId: job.id, result, ms: Date.now() - startedAt },
    });
    console.log(`[${job.stage}] ${result}`);
  } catch (error) {
    if (error instanceof JobCancelled || isJobCancelRequested(job.id)) {
      jobs.markCancelled(job.id, error instanceof Error ? error.message : "Cancelled by user");
      clearJobCancelRequested(job.id);
      events.log("job_cancelled", {
        projectId: job.project_id,
        ref: job.stage,
        data: { jobId: job.id, ms: Date.now() - startedAt },
      });
      console.log(`[${job.stage}] cancelled`);
      return true;
    }

    if (error instanceof RetryLater) {
      // Reschedule without burning an attempt.
      db.run(
        `UPDATE jobs SET status = 'pending', attempts = MAX(attempts - 1, 0),
           last_error = ?, run_after = datetime('now', ?), updated_at = datetime('now')
         WHERE id = ?`,
        [error.message, `+${error.delaySeconds} seconds`, job.id],
      );
      console.log(`[${job.stage}] deferred: ${error.message}`);
      return true;
    }

    const reason = error instanceof Error ? error.message : String(error);
    jobs.fail(job, reason);
    events.log("job_failed", {
      projectId: job.project_id,
      ref: job.stage,
      data: { jobId: job.id, attempt: job.attempts, error: reason },
    });
    console.error(`[${job.stage}] failed (attempt ${job.attempts}): ${reason}`);
  }
  return true;
}

async function loop(): Promise<void> {
  if (!running) return;
  try {
    // Drain whatever is ready, then idle. One job at a time on purpose: these
    // stages hit rate-limited free tiers and there is no reason to rush.
    while (running && (await runOne())) {
      /* keep draining */
    }
  } catch (error) {
    console.error("[runner] unexpected error:", error);
  }
  if (running) timer = setTimeout(loop, POLL_INTERVAL_MS);
}

export function startRunner(): void {
  if (running) return;
  const recovered = jobs.recoverStaleRunning();
  if (recovered > 0) console.warn(`[runner] recovered ${recovered} stale running job(s)`);
  const released = jobs.releaseScheduleDeferred();
  if (released > 0) {
    console.warn(`[runner] rechecking ${released} schedule-deferred job(s) after restart`);
  }
  running = true;
  console.log("[runner] started");
  void loop();
}

export function stopRunner(): void {
  running = false;
  if (timer) clearTimeout(timer);
  timer = null;
}

export type CancelScope = "setup" | "prospecting" | "send" | "all";

export function cancelProjectJobs(
  projectId: number,
  scope: CancelScope = "all",
): { cancelledPending: number; runningCancelled: number; scope: CancelScope } {
  const stages =
    scope === "setup"
      ? SETUP_STAGES
      : scope === "prospecting"
        ? PROSPECT_STAGES
        : scope === "send"
          ? SEND_STAGES
          : undefined;

  const { cancelledPending, runningJobIds } = jobs.cancelActive(
    projectId,
    stages ? { stages } : {},
  );

  for (const jobId of runningJobIds) {
    markJobCancelRequested(jobId);
  }

  events.log("jobs_cancel_requested", {
    projectId,
    ref: scope,
    data: { cancelledPending, runningJobIds },
  });

  return {
    cancelledPending,
    runningCancelled: runningJobIds.length,
    scope,
  };
}

export { SETUP_STAGES, PROSPECT_STAGES, SEND_STAGES };
