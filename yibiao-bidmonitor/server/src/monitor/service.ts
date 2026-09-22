import { createHash } from 'node:crypto';
import type { MonitorClientError } from './client';
import { sanitizeMonitorConfig } from './scope';
import type { MonitorConfig, MonitorResults, MonitorRuntimeConfig, MonitorStatus, MonitorSites, MonitorNotificationTest } from './types';
import type { MonitorStore, MonitorNotificationInput } from './store';

export interface MonitorClientLike {
  status(userId: number): Promise<MonitorStatus>;
  start(userId: number, runtimeConfig?: MonitorRuntimeConfig): Promise<{ accepted: boolean }>;
  stop(userId: number): Promise<{ accepted: boolean }>;
  runOnce(userId: number, runtimeConfig?: MonitorRuntimeConfig): Promise<{ accepted: boolean }>;
  updateConfig(userId: number, config: MonitorConfig): Promise<{ config: MonitorConfig }>;
  results(userId: number, limit: number, offset: number): Promise<MonitorResults>;
  logs(userId: number, limit: number): Promise<{ logs: string[] }>;
  clearHistory(userId: number): Promise<{ success: boolean }>;
  sites?(userId: number): Promise<MonitorSites>;
  updateSites?(userId: number, payload: { enabled_sites: string[]; custom_sites: Array<Record<string, unknown>> }): Promise<MonitorSites>;
  testNotification?(userId: number, payload: MonitorNotificationTest & { runtime_config?: MonitorRuntimeConfig }): Promise<{ success: boolean; channel: string; target: string }>;
  testAi?(userId: number, runtimeConfig?: MonitorRuntimeConfig): Promise<Record<string, unknown>>;
}

export interface MonitorStoreLike extends Pick<MonitorStore, 'getProfile' | 'updateProfile' | 'saveBids' | 'listLogs' | 'listContacts' | 'replaceContacts' | 'clearHistory'> {
  listRuns?: MonitorStore['listRuns'];
  createRun?: MonitorStore['createRun'];
  finishRun?: MonitorStore['finishRun'];
  saveLogs?: MonitorStore['saveLogs'];
  saveNotifications?: MonitorStore['saveNotifications'];
}

function userIdNumber(userId: number): number {
  if (!Number.isSafeInteger(userId) || userId <= 0) throw new Error('user id must be a positive integer');
  return userId;
}

function fingerprintForUrl(url: string): string {
  return createHash('sha256').update(url).digest('hex');
}

export class MonitorService {
  private readonly loadRuntimeConfig?: (userId: number) => MonitorRuntimeConfig | Promise<MonitorRuntimeConfig>;
  private readonly activeRuns = new Map<number, number>();
  private readonly reconcilingRuns = new Set<number>();

  constructor(
    private readonly client: MonitorClientLike,
    private readonly store: MonitorStoreLike,
    loadRuntimeConfig?: (userId: number) => MonitorRuntimeConfig | Promise<MonitorRuntimeConfig>,
  ) {
    this.loadRuntimeConfig = loadRuntimeConfig;
  }

  async status(userId: number): Promise<MonitorStatus> {
    const normalizedUserId = userIdNumber(userId);
    const status = await this.client.status(normalizedUserId);
    await this.reconcileRun(normalizedUserId, status);
    return status;
  }

  start(userId: number): Promise<{ accepted: boolean }> {
    const normalizedUserId = userIdNumber(userId);
    return Promise.resolve(this.runtimeConfig(normalizedUserId)).then(async (runtime) => {
      const result = await this.client.start(normalizedUserId, runtime);
      if (result.accepted) await this.beginRun(normalizedUserId);
      return result;
    });
  }

  async stop(userId: number): Promise<{ accepted: boolean }> {
    const normalizedUserId = userIdNumber(userId);
    const result = await this.client.stop(normalizedUserId);
    if (result.accepted) {
      const status = await this.client.status(normalizedUserId);
      await this.reconcileRun(normalizedUserId, status);
    }
    return result;
  }

  runOnce(userId: number): Promise<{ accepted: boolean }> {
    const normalizedUserId = userIdNumber(userId);
    return Promise.resolve(this.runtimeConfig(normalizedUserId)).then(async (runtime) => {
      const result = await this.client.runOnce(normalizedUserId, runtime);
      if (result.accepted) await this.beginRun(normalizedUserId);
      return result;
    });
  }

  private async runtimeConfig(userId: number): Promise<MonitorRuntimeConfig | undefined> {
    if (!this.loadRuntimeConfig) return undefined;
    const runtime = await this.loadRuntimeConfig(userId);
    return runtime && typeof runtime === 'object' ? runtime : undefined;
  }

  async getConfig(userId: number): Promise<Record<string, unknown>> {
    const profile = await this.store.getProfile(userIdNumber(userId));
    return (profile?.config && typeof profile.config === 'object' ? profile.config : {}) as Record<string, unknown>;
  }

  private async beginRun(userId: number): Promise<void> {
    if (!this.store.createRun) return;
    const profile = await this.store.getProfile(userId);
    const run = await this.store.createRun({ userId, profileId: profile?.id ?? null, status: 'running' });
    if (run?.id != null) this.activeRuns.set(userId, Number(run.id));
  }

  private async reconcileRun(userId: number, status: MonitorStatus): Promise<void> {
    if (!this.store.finishRun || !this.store.saveLogs || !this.store.saveNotifications) return;
    const runId = this.activeRuns.get(userId);
    if (!runId || status.current_task_running || status.is_running || this.reconcilingRuns.has(userId)) return;
    this.reconcilingRuns.add(userId);
    try {
      const result = await this.client.results(userId, 200, 0);
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
      await this.store.saveBids(userId, runId, bids);
      const logs = await this.client.logs(userId, 300);
      await this.store.saveLogs(userId, runId, logs.logs);
      const notifications = Array.isArray(status.last_result?.notifications)
        ? status.last_result.notifications
            .filter((item): item is Record<string, unknown> => Boolean(item && typeof item === 'object'))
            .flatMap((item) => bids.map((bid) => ({
              fingerprint: bid.fingerprint,
              channel: String(item.channel || ''),
              status: String(item.status || 'failed'),
              lastError: item.error == null ? null : String(item.error),
            })))
        : [];
      await this.store.saveNotifications(userId, notifications as MonitorNotificationInput[]);
      const failed = Boolean(status.last_error);
      await this.store.finishRun(runId, failed ? 'failed' : 'succeeded', status.last_result ?? null, status.last_error ?? null);
      this.activeRuns.delete(userId);
    } finally {
      this.reconcilingRuns.delete(userId);
    }
  }

  async getContacts(userId: number): Promise<any[]> {
    return this.store.listContacts(userIdNumber(userId));
  }

  async runs(userId: number, limit = 20): Promise<any[]> {
    if (!this.store.listRuns) return [];
    return this.store.listRuns(userIdNumber(userId), limit);
  }

  async updateContacts(userId: number, contacts: Array<Record<string, unknown>>): Promise<{ success: boolean }> {
    await this.store.replaceContacts(userIdNumber(userId), contacts);
    return { success: true };
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
    await this.store.saveBids(normalizedUserId, this.activeRuns.get(normalizedUserId) ?? null, bids);
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

  sites(userId: number): Promise<MonitorSites> {
    if (!this.client.sites) return Promise.reject(new Error('monitor sites service is not configured'));
    return this.client.sites(userIdNumber(userId));
  }

  updateSites(userId: number, payload: { enabled_sites: string[]; custom_sites: Array<Record<string, unknown>> }): Promise<MonitorSites> {
    if (!this.client.updateSites) return Promise.reject(new Error('monitor sites service is not configured'));
    const normalizedUserId = userIdNumber(userId);
    return this.client.updateSites(normalizedUserId, payload).then(async (result) => {
      const profile = await this.store.getProfile(normalizedUserId);
      const config = profile?.config && typeof profile.config === 'object' ? { ...profile.config as Record<string, unknown> } : {};
      const crawler = config.crawler && typeof config.crawler === 'object' ? { ...config.crawler as Record<string, unknown> } : {};
      await this.store.updateProfile(normalizedUserId, {
        ...config,
        crawler: { ...crawler, enabled_sites: payload.enabled_sites },
        custom_sites: payload.custom_sites,
      });
      return result;
    });
  }

  async testNotification(userId: number, payload: MonitorNotificationTest): Promise<{ success: boolean; channel: string; target: string }> {
    const normalizedUserId = userIdNumber(userId);
    const runtime = await this.runtimeConfig(normalizedUserId);
    if (!this.client.testNotification) throw new Error('monitor notification test is not configured');
    return this.client.testNotification(normalizedUserId, { ...payload, runtime_config: runtime });
  }

  async testAi(userId: number): Promise<Record<string, unknown>> {
    const normalizedUserId = userIdNumber(userId);
    const runtime = await this.runtimeConfig(normalizedUserId);
    if (!this.client.testAi) throw new Error('monitor AI test is not configured');
    return this.client.testAi(normalizedUserId, runtime);
  }
}

export function monitorErrorStatus(error: unknown): number {
  const typed = error as Partial<MonitorClientError>;
  if (typeof typed.status === 'number' && typed.status >= 400 && typed.status <= 599) return typed.status;
  return 502;
}
