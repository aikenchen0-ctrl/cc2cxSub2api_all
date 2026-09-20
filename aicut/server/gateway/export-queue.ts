/** Per-user export cap on top of the global Remotion limiter. Local OCC unchanged. */

import { TaskLimiter, type ReleaseTaskPermit } from '../task-limiter.ts';
import { gatewayEnabled, maxExportsPerUser, maxQueuedExportsPerUser } from './config.ts';
import { currentTenant } from './tenant-context.ts';

const userLimiters = new Map<string, TaskLimiter>();

function limiterFor(userId: string): TaskLimiter {
  const existing = userLimiters.get(userId);
  if (existing) return existing;
  const created = new TaskLimiter(maxExportsPerUser());
  userLimiters.set(userId, created);
  return created;
}

export function gatewayExportOwner(): string | undefined {
  if (!gatewayEnabled()) return undefined;
  return currentTenant()?.userId;
}

export function assertUserExportQueue(userId: string): void {
  const snap = limiterFor(userId).snapshot();
  if (snap.active + snap.queued >= maxQueuedExportsPerUser()) {
    throw new Error('导出队列已满，请等待当前任务完成');
  }
}

export async function acquireUserExportPermit(
  userId: string,
  signal?: AbortSignal,
): Promise<ReleaseTaskPermit> {
  assertUserExportQueue(userId);
  const limiter = limiterFor(userId);
  if (!signal) return limiter.acquire();
  signal.throwIfAborted();
  const pending = limiter.acquire();
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      void pending.then((release) => release());
      reject(signal.reason ?? new DOMException('Export cancelled', 'AbortError'));
    };
    signal.addEventListener('abort', onAbort, { once: true });
    void pending.then((release) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener('abort', onAbort);
      resolve(release);
    });
  });
}

export function userExportQueueSnapshot(userId: string): { active: number; queued: number; limit: number } {
  return limiterFor(userId).snapshot();
}
