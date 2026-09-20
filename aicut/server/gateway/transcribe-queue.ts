/** Per-user transcription cap. Local OCC is unchanged. */

import { TaskLimiter } from '../task-limiter.ts';
import { gatewayEnabled, maxQueuedTranscriptionsPerUser, maxTranscriptionsPerUser } from './config.ts';
import { currentTenant } from './tenant-context.ts';

const userLimiters = new Map<string, TaskLimiter>();

function limiterFor(userId: string): TaskLimiter {
  const existing = userLimiters.get(userId);
  if (existing) return existing;
  const created = new TaskLimiter(maxTranscriptionsPerUser());
  userLimiters.set(userId, created);
  return created;
}

export async function withUserTranscriptionPermit<T>(task: () => Promise<T>): Promise<T> {
  if (!gatewayEnabled()) return task();
  const userId = currentTenant()?.userId;
  if (!userId) throw new Error('转写需要登录');
  const limiter = limiterFor(userId);
  const snap = limiter.snapshot();
  if (snap.active + snap.queued >= maxQueuedTranscriptionsPerUser()) {
    throw new Error('转写队列已满，请等待当前任务完成');
  }
  return limiter.run(task);
}
