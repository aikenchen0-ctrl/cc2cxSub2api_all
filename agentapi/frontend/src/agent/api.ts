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

export type AgentIdentityProvider = 'email' | 'linuxdo' | 'oidc' | 'wechat' | 'dingtalk'

export interface AgentIdentityBinding {
  provider: AgentIdentityProvider
  bound: boolean
  bound_count: number
  can_bind: boolean
  can_unbind: boolean
  display_name?: string
  subject_hint?: string
  note?: string
}

export interface AgentIdentityBindingsResponse {
  items: AgentIdentityBinding[]
  can_edit: boolean
  oauth_binding_supported: boolean
  profile?: AgentProfile
}

export interface AgentOAuthBindingStartResponse {
  provider: Exclude<AgentIdentityProvider, 'email'>
  authorize_url: string
  method: 'GET'
}

export interface AgentTOTPStatus {
  enabled: boolean
  enabled_at?: number
  feature_enabled: boolean
}

export interface AgentTOTPVerificationMethod {
  method: 'email' | 'password'
}

export interface AgentTOTPSetupResponse {
  secret: string
  qr_code_url: string
  setup_token: string
  countdown: number
}

export interface AgentNotifyEmailEntry {
  email: string
  disabled: boolean
  verified: boolean
}

export interface AgentBalanceNotifySettings {
  feature_enabled: boolean
  system_default_threshold: number
  enabled: boolean
  threshold: number | null
  extra_emails: AgentNotifyEmailEntry[]
}

export interface AgentHomeSettings {
  site_subtitle?: string
  compact_home_enabled?: boolean
  home_content?: string
}

export interface AgentBranding extends AgentHomeSettings {
  name: string
  site_name: string
  site_logo: string
  doc_url: string
  contact_info: string
}

export interface AgentView extends AgentHomeSettings {
  agent_id: string
  domain: string
  name: string
  site_name: string
  site_logo?: string
  doc_url?: string
  contact_info?: string
  status: string
  billing_mode: string
  owner_main_user_id?: string
  main_balance_cents: number
  main_balance_checked_at?: string
  billing_status: string
  wallet_available_cents: number
  wallet_allocated_cents: number
}

export interface AgentAdminProvisioning {
  agent_id: string
  domain: string
  display_name: string
  site_name: string
  site_logo?: string
  owner_main_user_id: string
  configured_status: string
  runtime_status: string
  control_enabled: boolean
  control_available: boolean
  ready: boolean
  last_checked_at?: string
  stale_after_seconds: number
  billing_mode: string
  satellite_slug: string
  lifecycle_authority: 'sub2api_main'
  management_scope: 'current_agent_read_only'
}

export interface AgentAdminPromoCode {
  code: string
  bonus_amount: number
  max_uses: number
  used_count: number
  status: string
  expires_at?: string | null
  created_at: string
}

export interface AgentAdminPromoCodes {
  feature_enabled: boolean
  funding_mode: 'unconfigured' | 'owner_balance_on_redemption'
  authority: 'sub2api_main'
  management_scope: 'current_agent'
  redemption_enabled: boolean
  can_create: boolean
  can_edit: boolean
  can_delete: boolean
  items: AgentAdminPromoCode[]
  total: number
}

export type AgentAnnouncementStatus = 'draft' | 'active' | 'archived'
export type AgentAnnouncementNotifyMode = 'silent' | 'popup'

export interface AgentAnnouncement {
  id: number
  title: string
  content: string
  status: AgentAnnouncementStatus
  notify_mode: AgentAnnouncementNotifyMode
  starts_at?: string
  ends_at?: string
  created_at: string
  updated_at: string
  read_at?: string
}

export interface AgentAnnouncementInput {
  title: string
  content: string
  status: AgentAnnouncementStatus
  notify_mode: AgentAnnouncementNotifyMode
  starts_at?: string
  ends_at?: string
}

export type AgentContentPageKind = 'legal' | 'custom'
export type AgentContentPageStatus = 'draft' | 'active' | 'archived'

export interface AgentContentPage {
  id: number
  slug: string
  kind: AgentContentPageKind
  title: string
  content: string
  status: AgentContentPageStatus
  sort_order: number
  created_at: string
  updated_at: string
}

export interface AgentContentPageInput {
  slug: string
  kind: AgentContentPageKind
  title: string
  content: string
  status: AgentContentPageStatus
  sort_order: number
}

export interface AgentUserView {
  agent_id: string
  main_user_id: string
  email?: string
  display_name?: string
  status: string
  balance_cents: number
  frozen_balance_cents?: number
  balance_error?: string
  created_at: string
  updated_at: string
}

export interface AgentContextResponse {
  agent: AgentView
  authenticated: boolean
  is_agent_admin: boolean
  main_user_id?: string
  user?: AgentUserView
  balance_error?: string
}

export interface AgentModelPolicy {
  catalog: string[]
  enabled: string[]
  customized: boolean
}

export interface AgentAvailableGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  is_exclusive: boolean
}

export interface AgentAvailableModelPricing {
  billing_mode?: string
  input_price?: number | null
  output_price?: number | null
  cache_write_price?: number | null
  cache_read_price?: number | null
  image_input_price?: number | null
  image_output_price?: number | null
  per_request_price?: number | null
  intervals?: unknown[]
}

export interface AgentAvailableModel {
  name: string
  platform: string
  pricing: AgentAvailableModelPricing | null
}

export interface AgentAvailableChannelSection {
  platform: string
  groups: AgentAvailableGroup[]
  supported_models: AgentAvailableModel[]
}

export interface AgentAvailableChannel {
  name: string
  description: string
  platforms: AgentAvailableChannelSection[]
}

export interface AgentAvailableChannelsResponse {
  channels: AgentAvailableChannel[]
  user_group_rates: Record<string, number>
}

export interface AgentSubscriptionGroup {
  id: number
  name: string
  description: string
  platform: string
  rate_multiplier: number
  subscription_type: string
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
}

export interface AgentSubscription {
  id: number
  group_id: number
  starts_at: string
  expires_at: string
  status: string
  daily_window_start: string | null
  weekly_window_start: string | null
  monthly_window_start: string | null
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  group?: AgentSubscriptionGroup
}

export interface AgentAdminSubscription extends AgentSubscription {
  main_user_id: string
  user_email?: string
  user_display_name?: string
}

export interface AgentAdminSubscriptionPage {
  items: AgentAdminSubscription[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AgentOrder {
  id: number
  amount: number
  pay_amount: number
  fee_rate: number
  currency: string
  payment_type: string
  out_trade_no: string
  status: string
  order_type: string
  created_at: string
  expires_at: string
  paid_at?: string
  completed_at?: string
  refund_amount: number
  refund_reason?: string
  refund_requested_at?: string
  refund_requested_by?: string
  refund_request_reason?: string
  plan_id?: number
  provider_instance_id?: string
}

export interface AgentOrderPage {
  items: AgentOrder[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AgentAdminOrder extends AgentOrder {
  main_user_id: string
  user_email?: string
  user_display_name?: string
}

export interface AgentAdminOrderPage {
  items: AgentAdminOrder[]
  total: number
  page: number
  page_size: number
  pages: number
}

export type AgentCurrencyAmounts = Record<string, number>

export interface AgentAdminDailyPaymentStats {
  date: string
  amount: AgentCurrencyAmounts
  count: number
}

export interface AgentAdminPaymentMethodStats {
  type: string
  amount: AgentCurrencyAmounts
  count: number
}

export interface AgentAdminTopUserPaymentStats {
  main_user_id: string
  email?: string
  name?: string
  amount: number
}

export interface AgentAdminPaymentDashboard {
  today_amount: AgentCurrencyAmounts
  total_amount: AgentCurrencyAmounts
  today_count: number
  total_count: number
  avg_amount: AgentCurrencyAmounts
  pending_orders: number
  daily_series: AgentAdminDailyPaymentStats[]
  payment_methods: AgentAdminPaymentMethodStats[]
  top_users: Record<string, AgentAdminTopUserPaymentStats[]>
}

export interface AgentPaymentMethodLimit {
  currency?: string
  display_name?: string
  daily_limit: number
  daily_used: number
  daily_remaining: number
  single_min: number
  single_max: number
  fee_rate: number
  available: boolean
}

export interface AgentCheckoutPlan {
  id: number
  group_id: number
  group_platform?: string
  group_name?: string
  rate_multiplier?: number
  peak_rate_enabled?: boolean
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  supported_model_scopes?: string[]
  name: string
  description: string
  price: number
  original_price?: number
  currency?: string
  validity_days: number
  validity_unit: string
  features: string[]
  product_name: string
}

export interface AgentAdminCheckoutPlan extends AgentCheckoutPlan {
  enabled: boolean
  sort_order: number
  customized: boolean
  source_name: string
  source_description: string
  source_features: string[]
}

export interface AgentPlanPolicyInput {
  enabled: boolean
  sort_order: number
  display_name: string
  description: string
  features: string[]
}

export interface AgentCheckoutInfo {
  methods: Record<string, AgentPaymentMethodLimit>
  global_min: number
  global_max: number
  plans: AgentCheckoutPlan[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  subscription_usd_to_cny_rate: number
  recharge_fee_rate: number
  help_text: string
  help_image_url: string
  stripe_publishable_key: string
  alipay_force_qrcode: boolean
  alipay_mobile_precreate_deep_link: boolean
}

export interface AgentPaymentOrderRequest {
  amount: number
  payment_type: string
  order_type: 'balance' | 'subscription'
  plan_id?: number
  openid?: string
  wechat_resume_token?: string
  is_mobile?: boolean
  is_wechat_browser?: boolean
}

export interface AgentPaymentCreateResult {
  order_id: number
  amount: number
  pay_amount: number
  fee_rate: number
  status: string
  result_type?: 'order_created' | 'oauth_required' | 'jsapi_ready' | string
  payment_type?: string
  out_trade_no?: string
  pay_url?: string
  qr_code?: string
  client_secret?: string
  intent_id?: string
  currency?: string
  country_code?: string
  payment_env?: string
  expires_at: string
  payment_mode?: string
  resume_token?: string
  alipay_mobile_precreate_deep_link?: boolean
  oauth?: {
    authorize_url?: string
    appid?: string
    openid?: string
    scope?: string
    state?: string
    redirect_url?: string
  }
  jsapi?: Record<string, string>
  jsapi_payload?: Record<string, string>
}

export interface AgentAffiliateInvitee {
  email: string
  username: string
  created_at?: string
  total_rebate: number
}

export interface AgentAffiliateDetail {
  aff_code: string
  aff_count: number
  aff_quota: number
  aff_frozen_quota: number
  aff_history_quota: number
  effective_rebate_rate_percent: number
  invitees: AgentAffiliateInvitee[]
}

export interface AgentAffiliateTransfer {
  transferred_quota: number
  balance: number
}

export interface AgentAffiliateInviteRecord {
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  aff_code: string
  total_rebate: number
  created_at: string
}

export interface AgentAffiliateRebateRecord {
  order_id: number
  out_trade_no: string
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  order_amount: number
  pay_amount: number
  rebate_amount: number
  payment_type: string
  order_status: string
  created_at: string
}

export interface AgentAffiliateTransferRecord {
  ledger_id: number
  user_id: number
  user_email: string
  username: string
  amount: number
  balance_after?: number
  available_quota_after?: number
  frozen_quota_after?: number
  history_quota_after?: number
  snapshot_available: boolean
  created_at: string
}

export interface AgentAffiliateRecordPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AgentAffiliateRecordFilters {
  search?: string
  start_at?: string
  end_at?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  main_user_id?: string
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

export interface UsageMetrics {
  requests: number
  measured: number
  missing_actual: number
  unobserved: number
  pending: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  standard_cost_usd_nanos: number
  actual_cost_usd_nanos: number
  route_observed: number
  route_mismatch: number
}

export interface UsageInsights extends UsageMetrics {
  source: string
  status: 'empty' | 'measured' | 'partial'
  start: string
  end: string
  updated_at: string
  scope: 'user' | 'agent'
  models: Array<UsageMetrics & { key: string }>
  trend: Array<UsageMetrics & { key: string }>
}

export interface AgentTenantBackupRecord {
  id: number
  status: 'completed'
  file_name: string
  size_bytes: number
  parts: string[]
  triggered_by: 'manual'
  created_by?: string
  started_at: string
  restored_at?: string
}

export interface AgentTenantBackupSnapshot {
  version: number
  source_agent_id: string
  created_at: string
  branding: { name: string; site_name: string; site_logo?: string; doc_url?: string; contact_info?: string }
  model_policy: { customized: boolean; enabled: string[] }
  announcements: Array<{
    title: string
    content: string
    status: AgentAnnouncementStatus
    notify_mode: AgentAnnouncementNotifyMode
    starts_at?: string
    ends_at?: string
  }>
  content_pages: Array<{
    slug: string
    kind: AgentContentPageKind
    title: string
    content: string
    status: AgentContentPageStatus
    sort_order: number
  }>
  plan_policies: Array<AgentPlanPolicyInput & { plan_id: number }>
}

export interface AgentTenantBackupList {
  items: AgentTenantBackupRecord[]
  total: number
  parts: string[]
}

export interface AgentTenantBackupDownload {
  record: AgentTenantBackupRecord
  snapshot: AgentTenantBackupSnapshot
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

// Current records are sourced from the public user-scoped usage API. Keep the
// earlier runtime/admin labels readable for snapshots persisted by older builds.
export function isAuthoritativeAgentUsageSource(source?: string): boolean {
  return source === 'sub2api_user_usage'
    || source === 'sub2api_owner_usage'
    || source === 'sub2api_owner_runtime_usage'
    || source === 'sub2api_admin_usage'
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
  model?: string
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

export interface PublicSettings extends AgentHomeSettings {
  registration_enabled?: boolean
  email_verify_enabled?: boolean
  main_site_sso_enabled?: boolean
  password_reset_enabled?: boolean
  turnstile_enabled?: boolean
  turnstile_site_key?: string
  tencent_captcha_enabled?: boolean
  tencent_captcha_app_id?: string
  tencent_captcha_region?: string
  aliyun_captcha_enabled?: boolean
  aliyun_captcha_scene_id?: string
  aliyun_captcha_prefix?: string
  aliyun_captcha_region?: string
  passkey_enabled?: boolean
  passkey_configured?: boolean
  site_name: string
  site_logo: string
  doc_url?: string
  contact_info?: string
  site_subtitle?: string
  version?: string
  payment_enabled?: boolean
  payment_provider?: string
  payment_currency?: string
  payment_min_amount_cents?: number
  payment_max_amount_cents?: number
  recharge_url?: string
}

export interface PasswordRecoveryConfig {
  password_reset_enabled: boolean
  turnstile_enabled: boolean
  turnstile_site_key: string
  tencent_captcha_enabled: boolean
  tencent_captcha_app_id: string
  tencent_captcha_region: string
  aliyun_captcha_enabled: boolean
  aliyun_captcha_scene_id: string
  aliyun_captcha_prefix: string
  aliyun_captcha_region: string
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
    register: async (email: string, password: string, username?: string, affiliateCode?: string, verifyCode?: string) =>
      (await agentClient.post<LoginResponse>('/auth/register', {
        email, password, ...(username ? { username } : {}), ...(affiliateCode ? { aff_code: affiliateCode } : {}),
        ...(verifyCode ? { verify_code: verifyCode } : {}),
      })).data,
    sendVerifyCode: async (email: string) =>
      (await agentClient.post<{ message: string; countdown: number }>('/auth/send-verify-code', { email })).data,
    forgotPassword: async (payload: {
      email: string
      turnstile_token?: string
      tencent_captcha_ticket?: string
      tencent_captcha_randstr?: string
    }) => (await agentClient.post<{ message: string }>('/auth/forgot-password', payload)).data,
    resetPassword: async (email: string, token: string, newPassword: string) =>
      (await agentClient.post<{ message: string }>('/auth/reset-password', {
        email, token, new_password: newPassword,
      })).data,
    passwordRecoveryConfig: async () =>
      (await agentClient.get<PasswordRecoveryConfig>('/auth/password-recovery/config')).data,
    me: async () => (await agentClient.get<AgentUser>('/auth/me')).data,
    refresh: async () => (await agentClient.post<AuthenticatedResponse>('/auth/refresh')).data,
    logout: async () => { await agentClient.post('/auth/logout') },
  },
  profile: {
    get: async () => (await agentClient.get<AgentProfile>('/agent/profile')).data,
    update: async (username: string) =>
      (await agentClient.put<AgentProfile>('/agent/profile', { username })).data,
    avatar: {
      update: async (avatarUrl: string) =>
        (await agentClient.put<AgentProfile>('/agent/profile/avatar', { avatar_url: avatarUrl })).data,
    },
    bindings: {
      get: async () =>
        (await agentClient.get<AgentIdentityBindingsResponse>('/agent/profile/bindings')).data,
      sendEmailCode: async (email: string) =>
        (await agentClient.post<{ success: boolean }>('/agent/profile/bindings/email/send-code', { email })).data,
      bindEmail: async (payload: { email: string; verify_code: string; password: string }) =>
        (await agentClient.post<AgentIdentityBindingsResponse>('/agent/profile/bindings/email', payload)).data,
      start: async (provider: Exclude<AgentIdentityProvider, 'email'>) =>
        (await agentClient.post<AgentOAuthBindingStartResponse>(`/agent/profile/bindings/${provider}/start`, {})).data,
      unbind: async (provider: Exclude<AgentIdentityProvider, 'email'>) =>
        (await agentClient.delete<{ success: boolean; reauthenticate: boolean }>(`/agent/profile/bindings/${provider}`)).data,
    },
    changePassword: async (oldPassword: string, newPassword: string) =>
      (await agentClient.put<{ message: string }>('/agent/password', {
        old_password: oldPassword,
        new_password: newPassword,
      })).data,
    totp: {
      getStatus: async () => (await agentClient.get<AgentTOTPStatus>('/agent/totp/status')).data,
      getVerificationMethod: async () =>
        (await agentClient.get<AgentTOTPVerificationMethod>('/agent/totp/verification-method')).data,
      sendVerifyCode: async () =>
        (await agentClient.post<{ success: boolean }>('/agent/totp/send-code', {})).data,
      initiateSetup: async (payload: { email_code?: string; password?: string }) =>
        (await agentClient.post<AgentTOTPSetupResponse>('/agent/totp/setup', payload)).data,
      enable: async (totpCode: string, setupToken: string) =>
        (await agentClient.post<{ success: boolean }>('/agent/totp/enable', {
          totp_code: totpCode,
          setup_token: setupToken,
        })).data,
      disable: async (payload: { email_code?: string; password?: string }) =>
        (await agentClient.post<{ success: boolean }>('/agent/totp/disable', payload)).data,
    },
    balanceNotify: {
      get: async () => (await agentClient.get<AgentBalanceNotifySettings>('/agent/balance-notify')).data,
      update: async (payload: { enabled?: boolean; threshold?: number }) =>
        (await agentClient.put<AgentBalanceNotifySettings>('/agent/balance-notify', payload)).data,
      sendCode: async (email: string) =>
        (await agentClient.post<{ success: boolean }>('/agent/balance-notify/send-code', { email })).data,
      verify: async (email: string, code: string) =>
        (await agentClient.post<{ success: boolean }>('/agent/balance-notify/verify', { email, code })).data,
      toggle: async (email: string, disabled: boolean) =>
        (await agentClient.put<AgentBalanceNotifySettings>('/agent/balance-notify/toggle', { email, disabled })).data,
      remove: async (email: string) =>
        (await agentClient.delete<{ success: boolean }>('/agent/balance-notify/email', { data: { email } })).data,
    },
  },
  getPublicSettings: async () => (await agentClient.get<PublicSettings>('/settings/public')).data,
  getContext: async () => (await agentClient.get<AgentContextResponse>('/agent/context')).data,
  announcements: {
    list: async () => (await agentClient.get<{ items: AgentAnnouncement[]; total: number; unread: number }>('/agent/announcements')).data,
    markRead: async (id: number) => (await agentClient.post<{ id: number; read: boolean }>(`/agent/announcements/${id}/read`, {})).data,
    markAllRead: async () => (await agentClient.post<{ read: boolean }>('/agent/announcements/read-all', {})).data,
  },
  contentPages: {
    list: async () => (await agentClient.get<{ items: AgentContentPage[]; total: number }>('/agent/content-pages')).data,
    get: async (kind: AgentContentPageKind, slug: string) =>
      (await agentClient.get<AgentContentPage>(`/agent/content/${kind}/${encodeURIComponent(slug)}`)).data,
  },
  getWallet: async () => (await agentClient.get<{ agent: AgentView; user: AgentUserView }>('/agent/wallet')).data,
  getUsers: async (page = 1, pageSize = 25, search = '', status: '' | 'active' | 'disabled' = '') =>
    (await agentClient.get<{ items: AgentUserView[]; total: number; page: number; page_size: number }>('/agent/users', {
      params: { page, page_size: pageSize, ...(search.trim() ? { q: search.trim() } : {}), ...(status ? { status } : {}) },
    })).data,
  getUsageInsights: async (window: string) =>
    (await agentClient.get<UsageInsights>('/agent/usage/insights', { params: { window } })).data,
  getUsage: async (page = 1, pageSize = 25, filters: { model?: string; request_id?: string; start_time?: string; end_time?: string } = {}) =>
    (await agentClient.get<{ items: AgentUsageView[]; total: number; page: number; page_size: number }>('/agent/usage', {
      params: { ...filters, page, page_size: pageSize },
    })).data,
  getTasks: async (page = 1, pageSize = 25, taskType: '' | 'image' | 'video' = '') =>
    (await agentClient.get<AgentTaskHistoryResponse>('/agent/tasks', {
      params: { page, page_size: pageSize, ...(taskType ? { task_type: taskType } : {}) },
    })).data,
  getAdminChannels: async () =>
    (await agentClient.get<AgentAvailableChannelsResponse>('/agent/admin/channels')).data,
  adminBackups: {
    list: async () =>
      (await agentClient.get<AgentTenantBackupList>('/agent/admin/backups')).data,
    create: async () =>
      (await agentClient.post<AgentTenantBackupRecord>('/agent/admin/backups', {})).data,
    download: async (id: number) =>
      (await agentClient.get<AgentTenantBackupDownload>(`/agent/admin/backups/${id}/download`)).data,
    import: async (snapshot: AgentTenantBackupSnapshot) =>
      (await agentClient.post<AgentTenantBackupRecord>('/agent/admin/backups/import', snapshot)).data,
    restore: async (id: number) =>
      (await agentClient.post<AgentTenantBackupRecord>(`/agent/admin/backups/${id}/restore`, {})).data,
    remove: async (id: number) => { await agentClient.delete(`/agent/admin/backups/${id}`) },
  },
  orders: {
    list: async (page = 1, pageSize = 20, status = '') =>
      (await agentClient.get<AgentOrderPage>('/agent/orders', {
        params: { page, page_size: pageSize, ...(status ? { status } : {}) },
      })).data,
    cancel: async (id: number) =>
      (await agentClient.post<{ message: string }>(`/agent/orders/${id}/cancel`, {})).data,
    requestRefund: async (id: number, reason: string) =>
      (await agentClient.post<{ message: string }>(`/agent/orders/${id}/refund-request`, { reason })).data,
    refundEligibleProviders: async () =>
      (await agentClient.get<{ provider_instance_ids: string[] }>('/agent/orders/refund-eligible-providers')).data,
  },
  adminOrders: {
    list: async (page = 1, pageSize = 20, status = '', mainUserID = '') =>
      (await agentClient.get<AgentAdminOrderPage>('/agent/admin/orders', {
        params: {
          page,
          page_size: pageSize,
          ...(status ? { status } : {}),
          ...(mainUserID ? { main_user_id: mainUserID } : {}),
        },
      })).data,
    cancel: async (mainUserID: string, id: number) =>
      (await agentClient.post<{ message: string }>(`/agent/admin/orders/${encodeURIComponent(mainUserID)}/${id}/cancel`, {})).data,
    dashboard: async (days: 7 | 30 | 90 = 30) =>
      (await agentClient.get<AgentAdminPaymentDashboard>('/agent/admin/orders/dashboard', { params: { days } })).data,
  },
  adminPaymentPlans: {
    list: async () =>
      (await agentClient.get<{ items: AgentAdminCheckoutPlan[]; total: number }>('/agent/admin/payment/plans')).data,
    update: async (id: number, payload: AgentPlanPolicyInput) =>
      (await agentClient.put<AgentAdminCheckoutPlan>(`/agent/admin/payment/plans/${id}`, payload)).data,
  },
  adminSubscriptions: {
    list: async (page = 1, pageSize = 20, filters: { status?: string; main_user_id?: string; group_id?: number; platform?: string } = {}) =>
      (await agentClient.get<AgentAdminSubscriptionPage>('/agent/admin/subscriptions', {
        params: { page, page_size: pageSize, ...filters },
      })).data,
  },
  adminProvisioning: {
    get: async () =>
      (await agentClient.get<AgentAdminProvisioning>('/agent/admin/agent-provisioning')).data,
  },
  adminPromoCodes: {
    get: async () =>
      (await agentClient.get<AgentAdminPromoCodes>('/agent/admin/promo-codes')).data,
  },
  adminAffiliates: {
    invites: async (page = 1, pageSize = 20, filters: AgentAffiliateRecordFilters = {}) =>
      (await agentClient.get<AgentAffiliateRecordPage<AgentAffiliateInviteRecord>>('/agent/admin/affiliates/invites', {
        params: { page, page_size: pageSize, ...filters },
      })).data,
    rebates: async (page = 1, pageSize = 20, filters: AgentAffiliateRecordFilters = {}) =>
      (await agentClient.get<AgentAffiliateRecordPage<AgentAffiliateRebateRecord>>('/agent/admin/affiliates/rebates', {
        params: { page, page_size: pageSize, ...filters },
      })).data,
    transfers: async (page = 1, pageSize = 20, filters: AgentAffiliateRecordFilters = {}) =>
      (await agentClient.get<AgentAffiliateRecordPage<AgentAffiliateTransferRecord>>('/agent/admin/affiliates/transfers', {
        params: { page, page_size: pageSize, ...filters },
      })).data,
  },
  payment: {
    checkoutInfo: async () =>
      (await agentClient.get<AgentCheckoutInfo>('/agent/payment/checkout-info')).data,
    createOrder: async (payload: AgentPaymentOrderRequest) =>
      (await agentClient.post<AgentPaymentCreateResult>('/agent/payment/orders', payload)).data,
    verifyOrder: async (outTradeNo: string) =>
      (await agentClient.post<AgentOrder>('/agent/payment/orders/verify', { out_trade_no: outTradeNo })).data,
  },
  affiliate: {
    get: async () => (await agentClient.get<AgentAffiliateDetail>('/agent/affiliate')).data,
    transfer: async () => (await agentClient.post<AgentAffiliateTransfer>('/agent/affiliate/transfer', {})).data,
  },
  getAdminWallet: async () => (await agentClient.get<AgentView>('/agent/admin/wallet')).data,
  getBranding: async () => (await agentClient.get<AgentBranding>('/agent/admin/branding')).data,
  updateBranding: async (payload: Partial<AgentBranding>) =>
    (await agentClient.put<AgentBranding>('/agent/admin/branding', payload)).data,
  adminAnnouncements: {
    list: async () => (await agentClient.get<{ items: AgentAnnouncement[]; total: number }>('/agent/admin/announcements')).data,
    create: async (payload: AgentAnnouncementInput) => (await agentClient.post<AgentAnnouncement>('/agent/admin/announcements', payload)).data,
    update: async (id: number, payload: AgentAnnouncementInput) => (await agentClient.put<AgentAnnouncement>(`/agent/admin/announcements/${id}`, payload)).data,
    delete: async (id: number) => { await agentClient.delete(`/agent/admin/announcements/${id}`) },
  },
  adminContentPages: {
    list: async () => (await agentClient.get<{ items: AgentContentPage[]; total: number }>('/agent/admin/content-pages')).data,
    create: async (payload: AgentContentPageInput) => (await agentClient.post<AgentContentPage>('/agent/admin/content-pages', payload)).data,
    update: async (id: number, payload: AgentContentPageInput) => (await agentClient.put<AgentContentPage>(`/agent/admin/content-pages/${id}`, payload)).data,
    delete: async (id: number) => { await agentClient.delete(`/agent/admin/content-pages/${id}`) },
  },
  getAdminModelPolicy: async () =>
    (await agentClient.get<AgentModelPolicy>('/agent/admin/model-policy')).data,
  updateAdminModelPolicy: async (enabled: string[]) =>
    (await agentClient.put<AgentModelPolicy>('/agent/admin/model-policy', { enabled })).data,
  getSettlements: async () => (await agentClient.get<{ items: SettlementView[]; total: number }>('/agent/admin/settlements')).data,
  getAuditEvents: async (limit = 100) => (await agentClient.get<{ items: AuditEventView[]; total: number }>('/agent/admin/audit-events', { params: { limit } })).data,
  reconcileSettlements: async (requestId?: string) =>
    (await agentClient.post<{ items: SettlementView[]; total: number }>('/agent/admin/settlements/reconcile', requestId ? { request_id: requestId } : {})).data,
  setMappedUserStatus: async (mainUserID: string, status: 'active' | 'disabled') =>
    (await agentClient.patch<AgentUserView>(`/agent/admin/users/${encodeURIComponent(mainUserID)}/status`, { status })).data,
  keys: {
    listForUser: async (userID: string) => (await agentClient.get<{ items: AgentAPIKeyView[]; total: number }>('/api-keys', { params: { main_user_id: userID } })).data,
    createForUser: async (userID: string, name: string) => (await agentClient.post<{ item: AgentAPIKeyView; key: string }>('/api-keys', { name }, { params: { main_user_id: userID } })).data,
    revokeForUser: async (userID: string, id: number) => (await agentClient.delete(`/api-keys/${id}`, { params: { main_user_id: userID } })).data,
    list: async () => (await agentClient.get<{ items: AgentAPIKeyView[]; total: number }>('/api-keys')).data,
    create: async (name: string) => (await agentClient.post<{ item: AgentAPIKeyView; key: string }>('/api-keys', { name })).data,
    revoke: async (id: number) => (await agentClient.delete<{ id: number; status: string }>(`/api-keys/${id}`)).data,
  },
  model: {
    publicList: async () => {
      const response = await agentClient.get<{ data?: Array<{ id?: string }> }>('/public/models')
      return (response.data.data || []).map((item) => item.id?.trim() || '').filter(Boolean)
    },
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
