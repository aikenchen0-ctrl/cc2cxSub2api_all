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
  ai_enabled?: boolean;
  ai_prompt?: string;
  [key: string]: unknown;
}

export interface MonitorSite {
  key: string;
  name: string;
  url: string;
  enabled: boolean;
}

export interface MonitorSites {
  sites: MonitorSite[];
  custom_sites: Array<{ name: string; url: string }>;
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

export interface MonitorRun {
  id: number;
  status: string;
  counts?: Record<string, unknown> | null;
  error?: string | null;
  startedAt: string;
  finishedAt?: string | null;
}

export interface MonitorContact {
  id?: number;
  channel: 'email' | 'sms' | 'wechat' | 'voice' | string;
  target: string;
  enabled: boolean;
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

export function useMonitorRuns() {
  return useQuery({
    queryKey: [...monitorKey, 'runs'],
    queryFn: async () => (await http.get<{ runs: MonitorRun[] }>('/monitor/runs', { params: { limit: 20 } })).data.runs,
    refetchInterval: 10000,
  });
}

export function useMonitorContacts() {
  return useQuery({
    queryKey: [...monitorKey, 'contacts'],
    queryFn: async () => (await http.get<{ contacts: MonitorContact[] }>('/monitor/contacts')).data.contacts,
  });
}

export function useMonitorSites() {
  return useQuery({
    queryKey: [...monitorKey, 'sites'],
    queryFn: async () => (await http.get<MonitorSites>('/monitor/sites')).data,
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

export function useMonitorSaveContacts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (contacts: MonitorContact[]) => (await http.put<{ success: boolean }>('/monitor/contacts', { contacts })).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorSaveSites() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (payload: { enabled_sites: string[]; custom_sites: Array<{ name: string; url: string }> }) =>
      (await http.put<MonitorSites>('/monitor/sites', payload)).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}

export function useMonitorTestNotification() {
  return useMutation({
    mutationFn: async (payload: { channel: 'email' | 'sms' | 'wechat' | 'voice'; target: string }) =>
      (await http.post<{ success: boolean }>('/monitor/test-notification', payload)).data,
  });
}

export function useMonitorTestAi() {
  return useMutation({
    mutationFn: async () => (await http.post<{ success: boolean; relevant?: boolean; reason?: string }>('/monitor/test-ai')).data,
  });
}

export function useMonitorClearHistory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await http.delete<{ success: boolean }>('/monitor/history')).data,
    onSuccess: () => invalidateMonitor(qc),
  });
}
