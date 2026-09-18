import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '../../../shared/api/http';

export interface MonitorStatus {
  user_id: string;
  is_running: boolean;
  current_task_running: boolean;
  last_run_time?: string | null;
  last_result?: Record<string, unknown> | null;
  last_error?: string | null;
}

export interface MonitorConfig {
  keywords?: string[];
  exclude_keywords?: string[];
  must_contain_keywords?: string[];
  notify_method?: string;
  interval_minutes?: number;
  crawler?: { enabled_sites?: string[]; use_selenium?: boolean; [key: string]: unknown };
  custom_sites?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export interface MonitorResultItem {
  title: string;
  url: string;
  source: string;
  publish_date?: string | null;
  purchaser?: string | null;
}

export interface MonitorResults {
  total: number;
  offset: number;
  limit: number;
  items: MonitorResultItem[];
}

const monitorKey = ['bid-monitor'];

export function useMonitorStatus() {
  return useQuery({
    queryKey: [...monitorKey, 'status'],
    queryFn: async () => (await http.get<MonitorStatus>('/monitor/status')).data,
    refetchInterval: 5000,
  });
}

export function useMonitorConfig() {
  return useQuery({
    queryKey: [...monitorKey, 'config'],
    queryFn: async () => (await http.get<{ config: MonitorConfig }>('/monitor/config')).data.config,
  });
}

export function useMonitorResults() {
  return useQuery({
    queryKey: [...monitorKey, 'results'],
    queryFn: async () => (await http.get<MonitorResults>('/monitor/results', { params: { limit: 100 } })).data,
    refetchInterval: 15000,
  });
}

export function useMonitorLogs() {
  return useQuery({
    queryKey: [...monitorKey, 'logs'],
    queryFn: async () => (await http.get<{ logs: string[] }>('/monitor/logs', { params: { limit: 100 } })).data.logs,
    refetchInterval: 5000,
  });
}

function invalidateMonitor(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: monitorKey });
}

export function useMonitorStart() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await http.post<{ accepted: boolean }>('/monitor/start')).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorStop() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await http.post<{ accepted: boolean }>('/monitor/stop')).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorRunOnce() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await http.post<{ accepted: boolean }>('/monitor/run-once')).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorSaveConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (config: MonitorConfig) => (await http.put<{ config: MonitorConfig }>('/monitor/config', config)).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorClearHistory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await http.delete<{ success: boolean }>('/monitor/history')).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}
