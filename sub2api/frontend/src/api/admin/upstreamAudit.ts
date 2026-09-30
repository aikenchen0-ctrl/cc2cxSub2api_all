import { apiClient } from '../client'

export type UpstreamAuditStatus = 'pending' | 'probing' | 'verified' | 'mismatch' | 'failed'

export interface UpstreamAuditDiagnostic {
  index: number
  parsed_numbers: number
  minimum_numbers: number
  accepted: boolean
}

export interface UpstreamAuditItem {
  account_id: number
  account_name: string
  platform: string
  declared_model: string
  status: UpstreamAuditStatus
  prediction?: string
  probability?: number
  declared_probability?: number
  declared_similarity?: number
  family_prediction?: string
  family_probability?: number
  compatible?: boolean
  used_outputs?: number
  attempts?: number
  latency_ms?: number
  error?: string
  diagnostics?: UpstreamAuditDiagnostic[]
  started_at?: string
  completed_at?: string
}

export interface UpstreamAuditAccountError {
  account_id: number
  account_name: string
  error: string
}

export interface UpstreamAuditJob {
  id: string
  status: 'scanning' | 'completed'
  total_accounts: number
  scanned_accounts: number
  matched_models: number
  completed: number
  verified: number
  mismatched: number
  failed: number
  started_at: string
  completed_at?: string
  items: UpstreamAuditItem[]
  account_errors?: UpstreamAuditAccountError[]
}

export interface UpstreamAuditOverview {
  supported_models: string[]
  bank_sha256: string
  latest_job: UpstreamAuditJob | null
}

export async function getOverview(): Promise<UpstreamAuditOverview> {
  const { data } = await apiClient.get<UpstreamAuditOverview>('/admin/upstream-audit')
  return data
}

export async function start(): Promise<UpstreamAuditJob> {
  const { data } = await apiClient.post<UpstreamAuditJob>('/admin/upstream-audit/start')
  return data
}

export async function getJob(jobId: string): Promise<UpstreamAuditJob> {
  const { data } = await apiClient.get<UpstreamAuditJob>(`/admin/upstream-audit/jobs/${encodeURIComponent(jobId)}`)
  return data
}

export default { getOverview, start, getJob }
