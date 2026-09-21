import { db } from "../db/index";
import { events, jobs } from "../db/repo";
import { markJobCancelRequested } from "../pipeline/context";

const QUOTA_LOOKBACK_HOURS = 24;

/**
 * Project-level gate for email verification spend.
 *
 * Only Reoon account quota/credits pause verification across the campaign.
 * Catch-all / risky protection is DOMAIN-scoped (see domainVerifyState) — a bad
 * domain must never block verify on the next brand-new company.
 *
 * This does NOT stop Serper company search.
 */
export function markVerifyQuotaExhausted(projectId: number, reason: string): void {
  if (isVerifyQuotaExhausted(projectId)) {
    // Already paused recently — don't spam cancel/events on every failed probe.
    return;
  }

  events.log("verify_quota_exhausted", {
    projectId,
    ref: "reoon",
    data: { reason },
  });
  // No point keeping a queue of verify jobs that will all fail the same way.
  const cancelled = jobs.cancelActive(projectId, { stages: ["verify_contact"] });
  for (const jobId of cancelled.runningJobIds) {
    markJobCancelRequested(jobId);
  }
  if (cancelled.cancelledPending > 0 || cancelled.runningJobIds.length > 0) {
    events.log("verify_quota_jobs_cancelled", {
      projectId,
      ref: "verify_contact",
      data: cancelled,
    });
  }
}

export function isVerifyQuotaExhausted(projectId: number): boolean {
  const row = db
    .query<{ id: number }, [number, string]>(
      `SELECT id FROM events
       WHERE project_id = ?
         AND kind = 'verify_quota_exhausted'
         AND created_at >= datetime('now', ?)
       ORDER BY id DESC
       LIMIT 1`,
    )
    .get(projectId, `-${QUOTA_LOOKBACK_HOURS} hours`);
  return Boolean(row);
}

export function verificationGate(projectId: number): {
  allow: boolean;
  reason?: string;
  kind?: "quota";
} {
  if (isVerifyQuotaExhausted(projectId)) {
    return {
      allow: false,
      kind: "quota",
      reason:
        "Email verification paused: Reoon credits/quota exhausted. Company search still works; top up REOON_API_KEY credits to resume verify.",
    };
  }
  return { allow: true };
}

/**
 * Clear the local quota pause after the user tops up Reoon.
 * Does not restore provider credits — only lifts our 24h circuit breaker.
 */
export function clearVerifyQuotaPause(projectId: number): number {
  const result = db.run(
    `DELETE FROM events
     WHERE project_id = ?
       AND kind = 'verify_quota_exhausted'`,
    [projectId],
  );
  if (result.changes > 0) {
    events.log("verify_quota_cleared", {
      projectId,
      ref: "reoon",
      data: { cleared: result.changes },
    });
  }
  return result.changes;
}
