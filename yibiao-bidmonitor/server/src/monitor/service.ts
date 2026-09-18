import { createHash } from 'node:crypto';
import type { MonitorClientError } from './client';
import { sanitizeMonitorConfig } from './scope';
import type { MonitorConfig, MonitorResults, MonitorStatus } from './types';
import type { MonitorStore } from './store';

export interface MonitorClientLike {
  status(userId: number): Promise<MonitorStatus>;
  start(userId: number): Promise<{ accepted: boolean }>;
  stop(userId: number): Promise<{ accepted: boolean }>;
  runOnce(userId: number): Promise<{ accepted: boolean }>;
  updateConfig(userId: number, config: MonitorConfig): Promise<{ config: MonitorConfig }>;
  results(userId: number, limit: number, offset: number): Promise<MonitorResults>;
  logs(userId: number, limit: number): Promise<{ logs: string[] }>;
  clearHistory(userId: number): Promise<{ success: boolean }>;
}

export interface MonitorStoreLike extends Pick<MonitorStore, 'getProfile' | 'updateProfile' | 'saveBids' | 'listLogs' | 'clearHistory'> {}

function userIdNumber(userId: number): number {
  if (!Number.isSafeInteger(userId) || userId <= 0) throw new Error('user id must be a positive integer');
  return userId;
}

function fingerprintForUrl(url: string): string {
  return createHash('sha256').update(url).digest('hex');
}

export class MonitorService {
  constructor(
    private readonly client: MonitorClientLike,
    private readonly store: MonitorStoreLike,
  ) {}

  status(userId: number): Promise<MonitorStatus> {
    return this.client.status(userIdNumber(userId));
  }

  start(userId: number): Promise<{ accepted: boolean }> {
    return this.client.start(userIdNumber(userId));
  }

  stop(userId: number): Promise<{ accepted: boolean }> {
    return this.client.stop(userIdNumber(userId));
  }

  runOnce(userId: number): Promise<{ accepted: boolean }> {
    return this.client.runOnce(userIdNumber(userId));
  }

  async getConfig(userId: number): Promise<Record<string, unknown>> {
    const profile = await this.store.getProfile(userIdNumber(userId));
    return (profile?.config && typeof profile.config === 'object' ? profile.config : {}) as Record<string, unknown>;
  }

  async updateConfig(userId: number, config: MonitorConfig): Promise<{ config: MonitorConfig }> {
    const normalizedUserId = userIdNumber(userId);
    const safeConfig = sanitizeMonitorConfig(config) as MonitorConfig;
    const result = await this.client.updateConfig(normalizedUserId, safeConfig);
    await this.store.updateProfile(normalizedUserId, safeConfig as Record<string, unknown>);
    return result;
  }

  async results(userId: number, limit = 50, offset = 0): Promise<MonitorResults> {
    const normalizedUserId = userIdNumber(userId);
    const result = await this.client.results(normalizedUserId, limit, offset);
    const bids = result.items
      .filter((item) => typeof item.url === 'string' && item.url.length > 0)
      .map((item) => ({
        fingerprint: fingerprintForUrl(String(item.url)),
        title: String(item.title ?? ''),
        url: String(item.url),
        source: String(item.source ?? ''),
        publishDate: item.publish_date == null ? null : String(item.publish_date),
        purchaser: item.purchaser == null ? null : String(item.purchaser),
        content: item.content == null ? null : String(item.content),
      }));
    await this.store.saveBids(normalizedUserId, null, bids);
    return result;
  }

  logs(userId: number, limit = 100): Promise<{ logs: string[] }> {
    return this.client.logs(userIdNumber(userId), limit);
  }

  async clearHistory(userId: number): Promise<{ success: boolean }> {
    const normalizedUserId = userIdNumber(userId);
    const result = await this.client.clearHistory(normalizedUserId);
    await this.store.clearHistory(normalizedUserId);
    return result;
  }
}

export function monitorErrorStatus(error: unknown): number {
  const typed = error as Partial<MonitorClientError>;
  if (typeof typed.status === 'number' && typed.status >= 400 && typed.status <= 599) return typed.status;
  return 502;
}
