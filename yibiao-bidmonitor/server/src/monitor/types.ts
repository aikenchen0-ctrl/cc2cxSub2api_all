export interface MonitorStatus {
  user_id: string;
  data_dir?: string;
  is_running: boolean;
  current_task_running: boolean;
  last_run_time?: string | null;
  last_result?: Record<string, unknown> | null;
  last_error?: string | null;
  config?: Record<string, unknown>;
}

export interface MonitorResults {
  total: number;
  offset: number;
  limit: number;
  items: Array<Record<string, unknown>>;
}

export interface MonitorConfig {
  keywords?: string[];
  exclude_keywords?: string[];
  must_contain_keywords?: string[];
  notify_method?: string;
  interval_minutes?: number;
  crawler?: Record<string, unknown>;
  custom_sites?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export interface MonitorClientOptions {
  baseUrl: string;
  serviceToken: string;
  timeoutMs?: number;
  fetchImpl?: typeof fetch;
}
