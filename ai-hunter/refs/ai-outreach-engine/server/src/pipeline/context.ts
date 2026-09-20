import { AsyncLocalStorage } from "node:async_hooks";
import type { JobStage } from "../db/types";
import { JobCancelled } from "./errors";

export type JobContext = {
  jobId: number;
  projectId: number;
  stage: JobStage;
};

const storage = new AsyncLocalStorage<JobContext>();

/** Job ids whose running handler should stop at the next cooperative checkpoint. */
const cancelRequested = new Set<number>();

export function runWithJobContext<T>(ctx: JobContext, fn: () => Promise<T>): Promise<T> {
  return storage.run(ctx, fn);
}

export function currentJobContext(): JobContext | undefined {
  return storage.getStore();
}

export function markJobCancelRequested(jobId: number): void {
  cancelRequested.add(jobId);
}

export function clearJobCancelRequested(jobId: number): void {
  cancelRequested.delete(jobId);
}

export function isJobCancelRequested(jobId: number): boolean {
  return cancelRequested.has(jobId);
}

/**
 * Call from long-running stages between expensive provider calls.
 * Throws JobCancelled when the operator hit Stop for this job.
 */
export function throwIfCancelled(): void {
  const ctx = storage.getStore();
  if (!ctx) return;
  if (cancelRequested.has(ctx.jobId)) {
    throw new JobCancelled();
  }
}
