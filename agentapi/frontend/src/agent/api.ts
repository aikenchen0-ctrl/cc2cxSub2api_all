import { agentClient } from './client'

export interface AgentUser {
  id: string | number
  email?: string
  username?: string
  display_name?: string
  avatar_url?: string
  role?: string
  status?: string
  agent_admin?: boolean
  agent_id?: string
}

export interface AgentProfile {
  id: string
  email: string
  username: string
  avatar_url?: string
  can_edit: boolean
}

export interface AgentView {
  agent_id: string
  domain: string
  name: string
  site_name: string
  site_logo?: string
  status: string
  billing_mode: string
  owner_main_user_id?: string
  main_balance_cents: number
  main_balance_checked_at?: string
  billing_status: string
  wallet_available_cents: number
  wallet_allocated_cents: number
}

export interface AgentUserView {
  agent_id: string
  main_user_id: string
  email?: string
  display_name?: string
  status: string
  balance_cents: number
  created_at: string
  updated_at: string
}

export interface AgentContextResponse {
  agent: AgentView
  authenticated: boolean
  is_agent_admin: boolean
  main_user_id?: string
  user?: AgentUserView
}

export interface AgentModelPolicy {
  catalog: string[]
  enabled: string[]
  customized: boolean
}

export interface AgentUsageView {
  request_id: string
  usage_id?: string
  proxy_main_user_id: string
  billing_main_user_id: string
  reserved_cents: number
  actual_cents: number
  settlement_status: string
  model?: string
  usage_source?: string
  service_tier?: string
  reasoning_effort?: string
  inbound_endpoint?: string
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  cache_creation_5m_tokens: number
  cache_creation_1h_tokens: number
  input_cost_usd_nanos: number
  output_cost_usd_nanos: number
  cache_creation_cost_usd_nanos: number
  cache_read_cost_usd_nanos: number
  total_cost_usd_nanos: number
  actual_cost_usd_nanos: number
  actual_cost_reported: boolean
  rate_multiplier: number
  long_context_billing_applied: boolean
  image_count: number
  image_input_tokens: number
  image_input_cost_usd_nanos: number
  image_output_tokens: number
  image_output_cost_usd_nanos: number
  upstream_model?: string
  upstream_response_model?: string
  upstream_model_mismatch?: boolean
  request_type?: string
  billing_mode?: string
  billing_type: number
  openai_ws_mode: boolean
  native_compaction_v2: boolean
  duration_ms: number
  first_token_ms: number
  stream: boolean
  image_size?: string
  image_input_size?: string
  image_output_size?: string
  image_size_source?: string
  image_size_breakdown?: Record<string, number>
  media_type?: string
  cache_ttl_overridden: boolean
  created_at: string
  error?: string
}

export interface AgentTaskHistoryItem {
  task_type: 'image' | 'video'
  task_id: string
  request_id: string
  model?: string
  status: string
  settlement_status?: string
  reserved_cents: number
  actual_cents: number
  created_at: string
  updated_at: string
}

export interface AgentTaskHistoryResponse {
  items: AgentTaskHistoryItem[]
  total: number
  page: number
  page_size: number
}

// Current records are sourced from the per-Agent Owner runtime API. Keep the
// earlier admin-usage label readable for snapshots persisted by older builds.
export function isAuthoritativeAgentUsageSource(source?: string): boolean {
  return source === 'sub2api_owner_runtime_usage' || source === 'sub2api_admin_usage'
}

export interface AgentAPIKeyView {
  id: number
  name: string
  prefix: string
  status: string
  created_at: string
  last_used_at?: string
}

export interface SettlementView {
  id?: number
  request_id: string
  proxy_main_user_id?: string
  billing_main_user_id?: string
  reserved_cents: number
  actual_cents: number
  status: string
  error?: string
  usage_id?: string
  created_at?: string
  updated_at?: string
}

export interface AuditEventView {
  id: number
  actor_type: string
  actor_id: string
  agent_id: string
  operation: string
  target_type: string
  target_id?: string
  request_id: string
  result: string
  reason?: string
  created_at: string
}

export interface RechargeOrder {
  id: number
  order_no: string
  agent_id: string
  main_user_id: string
  amount_cents: number
  currency: string
  provider: string
  status: string
  provider_trade_no?: string
  payment_url?: string
  request_id?: string
  created_at: string
  expires_at: string
  paid_at?: string
  allocated_at?: string
  updated_at: string
}

export interface RechargeOrdersResponse {
  enabled: boolean
  provider: string
  currency?: string
  min_amount_cents?: number
  max_amount_cents?: number
  items: RechargeOrder[]
  total: number
}

export interface ChatCompletionResponse {
  id?: string
  model?: string
  choices?: Array<{ message?: { role?: string; content?: string }; text?: string }>
  output_text?: string
  output?: Array<{ content?: Array<{ text?: string }> }>
  [key: string]: unknown
}

export interface ImageGenerationItem {
  url?: string
  b64_json?: string
  revised_prompt?: string
  mime_type?: string
  output_format?: string
}

export interface ImageGenerationResponse {
  created?: number
  data?: ImageGenerationItem[]
  [key: string]: unknown
}

export interface ImageTaskResponse {
  id?: string
  task_id?: string
  object?: string
  status?: string
  http_status?: number
  result?: ImageGenerationResponse
  error?: unknown
  data?: ImageTaskResponse | ImageGenerationItem[]
  [key: string]: unknown
}

export interface VideoTaskResponse {
  id?: string
  request_id?: string
  task_id?: string
  video_id?: string
  status?: string
  state?: string
  progress?: number | string
  video?: { url?: string; [key: string]: unknown }
  url?: string
  video_url?: string
  download_url?: string
  data?: VideoTaskResponse
  [key: string]: unknown
}

export interface PublicSettings {
  registration_enabled?: boolean
  site_name: string
  site_logo: string
  site_subtitle?: string
  version?: string
  payment_enabled?: boolean
  payment_provider?: string
  payment_currency?: string
  payment_min_amount_cents?: number
  payment_max_amount_cents?: number
}

export interface LoginResponse {
  access_token: ''
  refresh_token: ''
  expires_in: number
  token_type: 'Cookie'
  user?: AgentUser
  requires_2fa?: boolean
  temp_token?: string
}

export interface AuthenticatedResponse {
  access_token: ''
  refresh_token: ''
  expires_in: number
  token_type: 'Cookie'
  user: AgentUser
}

export const agentAPI = {
  auth: {
    login: async (email: string, password: string) =>
      (await agentClient.post<LoginResponse>('/auth/login', { email, password })).data,
    login2FA: async (tempToken: string, totpCode: string) =>
      (await agentClient.post<AuthenticatedResponse>('/auth/login/2fa', { temp_token: tempToken, totp_code: totpCode })).data,
    register: async (email: string, password: string, username?: string) =>
      (await agentClient.post<AuthenticatedResponse>('/auth/register', { email, password, ...(username ? { username } : {}) })).data,
    me: async () => (await agentClient.get<AgentUser>('/auth/me')).data,
    refresh: async () => (await agentClient.post<AuthenticatedResponse>('/auth/refresh')).data,
    logout: async () => { await agentClient.post('/auth/logout') },
  },
  profile: {
    get: async () => (await agentClient.get<AgentProfile>('/agent/profile')).data,
    update: async (username: string) =>
      (await agentClient.put<AgentProfile>('/agent/profile', { username })).data,
    changePassword: async (oldPassword: string, newPassword: string) =>
      (await agentClient.put<{ message: string }>('/agent/password', {
        old_password: oldPassword,
        new_password: newPassword,
      })).data,
  },
  getPublicSettings: async () => (await agentClient.get<PublicSettings>('/settings/public')).data,
  getContext: async () => (await agentClient.get<AgentContextResponse>('/agent/context')).data,
  getWallet: async () => (await agentClient.get<{ agent: AgentView; user: AgentUserView }>('/agent/wallet')).data,
  getUsers: async (page = 1, pageSize = 25, search = '') =>
    (await agentClient.get<{ items: AgentUserView[]; total: number; page: number; page_size: number }>('/agent/users', {
      params: { page, page_size: pageSize, ...(search.trim() ? { q: search.trim() } : {}) },
    })).data,
  getUsage: async (page = 1, pageSize = 25) =>
    (await agentClient.get<{ items: AgentUsageView[]; total: number; page: number; page_size: number }>('/agent/usage', {
      params: { page, page_size: pageSize },
    })).data,
  getTasks: async (page = 1, pageSize = 25) =>
    (await agentClient.get<AgentTaskHistoryResponse>('/agent/tasks', {
      params: { page, page_size: pageSize },
    })).data,
  recharge: {
    list: async () => (await agentClient.get<RechargeOrdersResponse>('/agent/recharge/orders')).data,
    create: async (amount: string | number, idempotencyKey?: string) =>
      (await agentClient.post<{ order: RechargeOrder; enabled: boolean; message: string }>('/agent/recharge/orders', { amount }, {
        headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
      })).data,
  },
  getAdminWallet: async () => (await agentClient.get<AgentView>('/agent/admin/wallet')).data,
  syncAdminWallet: async () => (await agentClient.post<AgentView>('/agent/admin/wallet/sync')).data,
  getBranding: async () => (await agentClient.get<{ name: string; site_name: string; site_logo: string }>('/agent/admin/branding')).data,
  updateBranding: async (payload: { name?: string; site_name?: string; site_logo?: string }) =>
    (await agentClient.put<{ name: string; site_name: string; site_logo: string }>('/agent/admin/branding', payload)).data,
  getAdminModelPolicy: async () =>
    (await agentClient.get<AgentModelPolicy>('/agent/admin/model-policy')).data,
  updateAdminModelPolicy: async (enabled: string[]) =>
    (await agentClient.put<AgentModelPolicy>('/agent/admin/model-policy', { enabled })).data,
  getAdminRechargeOrders: async () => (await agentClient.get<RechargeOrdersResponse>('/agent/admin/recharge/orders')).data,
  allocateRechargeOrder: async (orderNo: string, idempotencyKey?: string) =>
    (await agentClient.post<{ order: RechargeOrder; allocated: boolean }>('/agent/admin/recharge/allocate', { order_no: orderNo }, {
      headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
    })).data,
  getSettlements: async () => (await agentClient.get<{ items: SettlementView[]; total: number }>('/agent/admin/settlements')).data,
  getAuditEvents: async (limit = 100) => (await agentClient.get<{ items: AuditEventView[]; total: number }>('/agent/admin/audit-events', { params: { limit } })).data,
  reconcileSettlements: async (requestId?: string) =>
    (await agentClient.post<{ items: SettlementView[]; total: number }>('/agent/admin/settlements/reconcile', requestId ? { request_id: requestId } : {})).data,
  allocate: async (mainUserID: string, amount: number, note = '', idempotencyKey?: string) =>
    (await agentClient.post<AgentUserView>(`/agent/admin/users/${encodeURIComponent(mainUserID)}/allocate`, { amount, note }, {
      headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
    })).data,
  setMappedUserStatus: async (mainUserID: string, status: 'active' | 'disabled') =>
    (await agentClient.patch<AgentUserView>(`/agent/admin/users/${encodeURIComponent(mainUserID)}/status`, { status })).data,
  keys: {
    list: async () => (await agentClient.get<{ items: AgentAPIKeyView[]; total: number }>('/api-keys')).data,
    create: async (name: string) => (await agentClient.post<{ item: AgentAPIKeyView; key: string }>('/api-keys', { name })).data,
    revoke: async (id: number) => (await agentClient.delete<{ id: number; status: string }>(`/api-keys/${id}`)).data,
  },
  model: {
    list: async () => {
      const response = await agentClient.get<{ data?: Array<{ id?: string }> }>('/v1/models', { baseURL: '' })
      return (response.data.data || []).map((item) => item.id?.trim() || '').filter(Boolean)
    },
    chat: async (model: string, messages: Array<{ role: string; content: string }>) =>
      (await agentClient.post<ChatCompletionResponse>('/v1/chat/completions', { model, messages }, { baseURL: '' })).data,
    generateImage: async (model: string, prompt: string) =>
      (await agentClient.post<ImageGenerationResponse>('/v1/images/generations', { model, prompt }, {
        baseURL: '',
        timeout: 180_000,
      })).data,
    editImage: async (model: string, prompt: string, image: File) => {
      const body = new FormData()
      body.append('model', model)
      body.append('prompt', prompt)
      body.append('image', image)
      return (await agentClient.post<ImageGenerationResponse>('/v1/images/edits', body, {
        baseURL: '',
        timeout: 180_000,
        headers: { 'Content-Type': 'multipart/form-data' },
      })).data
    },
    generateImageAsync: async (model: string, prompt: string) =>
      (await agentClient.post<ImageTaskResponse>('/v1/images/generations/async', { model, prompt }, {
        baseURL: '',
        timeout: 60_000,
      })).data,
    editImageAsync: async (model: string, prompt: string, image: File) => {
      const body = new FormData()
      body.append('model', model)
      body.append('prompt', prompt)
      body.append('image', image)
      return (await agentClient.post<ImageTaskResponse>('/v1/images/edits/async', body, {
        baseURL: '',
        timeout: 60_000,
        headers: { 'Content-Type': 'multipart/form-data' },
      })).data
    },
    getImageTask: async (taskID: string) =>
      (await agentClient.get<ImageTaskResponse>(`/v1/images/tasks/${encodeURIComponent(taskID)}`, { baseURL: '', timeout: 60_000 })).data,
    createVideo: async (model: string, prompt: string) =>
      (await agentClient.post<VideoTaskResponse>('/v1/videos', {
        model,
        prompt,
        duration: 6,
        aspect_ratio: '16:9',
        resolution: '480p',
      }, { baseURL: '', timeout: 60_000 })).data,
    getVideo: async (taskID: string) =>
      (await agentClient.get<VideoTaskResponse>(`/v1/videos/${encodeURIComponent(taskID)}`, { baseURL: '', timeout: 60_000 })).data,
  },
}
