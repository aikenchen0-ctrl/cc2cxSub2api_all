import type {
  MonitorClientOptions,
  MonitorConfig,
  MonitorResults,
  MonitorStatus,
} from './types';
import { normalizeMonitorUserId, sanitizeMonitorConfig } from './scope';

export class MonitorClientError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly retryable: boolean,
    public readonly cause?: unknown,
  ) {
    super(message);
    this.name = 'MonitorClientError';
  }
}

type FetchImpl = typeof fetch;

export class BidMonitorClient {
  private readonly baseUrl: string;
  private readonly serviceToken: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: FetchImpl;

  constructor(options: MonitorClientOptions) {
    this.baseUrl = options.baseUrl.trim().replace(/\/+$/, '');
    this.serviceToken = options.serviceToken.trim();
    this.timeoutMs = Math.max(1000, options.timeoutMs ?? 10000);
    this.fetchImpl = options.fetchImpl ?? fetch;
    if (!this.baseUrl) throw new Error('monitor service base URL is required');
  }

  private path(userId: unknown, suffix: string): string {
    return `/internal/users/${encodeURIComponent(normalizeMonitorUserId(userId))}/${suffix}`;
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    if (!this.serviceToken) {
      throw new MonitorClientError('monitor service token is not configured', 503, false);
    }
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    const headers = new Headers(init.headers);
    headers.set('X-BidMonitor-Service-Token', this.serviceToken);
    if (init.body !== undefined) headers.set('Content-Type', 'application/json');

    try {
      const response = await this.fetchImpl(`${this.baseUrl}${path}`, {
        ...init,
        headers,
        signal: controller.signal,
      });
      const text = await response.text();
      let payload: unknown = null;
      if (text) {
        try {
          payload = JSON.parse(text);
        } catch {
          payload = { detail: text.slice(0, 500) };
        }
      }
      if (!response.ok) {
        const detail = typeof payload === 'object' && payload !== null && 'detail' in payload
          ? String((payload as { detail?: unknown }).detail ?? response.statusText)
          : response.statusText;
        throw new MonitorClientError(
          detail || 'monitor service request failed',
          response.status,
          response.status === 408 || response.status === 429 || response.status >= 500,
        );
      }
      return payload as T;
    } catch (error) {
      if (error instanceof MonitorClientError) throw error;
      if (controller.signal.aborted) {
        throw new MonitorClientError('monitor service request timed out', 504, true, error);
      }
      throw new MonitorClientError('monitor service request failed', 502, true, error);
    } finally {
      clearTimeout(timer);
    }
  }

  async status(userId: unknown): Promise<MonitorStatus> {
    return this.request<MonitorStatus>(this.path(userId, 'status'));
  }

  async start(userId: unknown): Promise<{ accepted: boolean }> {
    return this.request(this.path(userId, 'start'), { method: 'POST' });
  }

  async stop(userId: unknown): Promise<{ accepted: boolean }> {
    return this.request(this.path(userId, 'stop'), { method: 'POST' });
  }

  async runOnce(userId: unknown): Promise<{ accepted: boolean }> {
    return this.request(this.path(userId, 'run-once'), { method: 'POST' });
  }

  async updateConfig(userId: unknown, config: MonitorConfig): Promise<{ config: MonitorConfig }> {
    const safeConfig = sanitizeMonitorConfig(config) as MonitorConfig;
    return this.request(this.path(userId, 'config'), {
      method: 'PUT',
      body: JSON.stringify(safeConfig),
    });
  }

  async results(userId: unknown, limit = 50, offset = 0): Promise<MonitorResults> {
    const query = new URLSearchParams({
      limit: String(Math.max(1, Math.min(200, Math.trunc(limit)))),
      offset: String(Math.max(0, Math.trunc(offset))),
    });
    return this.request<MonitorResults>(`${this.path(userId, 'results')}?${query}`);
  }

  async logs(userId: unknown, limit = 100): Promise<{ logs: string[] }> {
    const query = new URLSearchParams({ limit: String(Math.max(1, Math.min(300, Math.trunc(limit)))) });
    return this.request(`${this.path(userId, 'logs')}?${query}`);
  }

  async clearHistory(userId: unknown): Promise<{ success: boolean }> {
    return this.request(this.path(userId, 'history'), { method: 'DELETE' });
  }
}
