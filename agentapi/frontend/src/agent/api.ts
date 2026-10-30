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

export interface AgentAffiliateDetail {
  aff_code: string
  aff_count: number
  aff_quota: number
  aff_frozen_quota: number
  aff_history_quota: number
  effective_rebate_rate_percent: number
  invitees: Array<{ email: string; username: string; total_rebate: number; created_at?: string }>
}

export interface AgentAffiliateTransfer {
  transferred_quota: number
  balance: number
}

export interface AgentAffiliatePage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AgentAffiliateInvite {
  email: string
  username: string
  aff_code: string
  total_rebate: number
  created_at?: string
}

export interface AgentAffiliateRebate {
  out_trade_no: string
  email: string
  username: string
  order_amount: number
  pay_amount: number
  rebate_amount: number
  payment_type: string
  order_status: string
  created_at?: string
}

export interface AgentAffiliateTransferRecord {
  amount: number
  balance_after?: number
  available_quota_after?: number
  created_at?: string
}

export interface AgentRedeemResult {
  type: string
  value: number
  status: string
}

export interface AgentRedeemHistoryItem extends AgentRedeemResult {
  used_at?: string
  created_at?: string
}

export interface AgentBalanceSnapshot {
  main_user_id: string
  balance_cents: number
  frozen_balance_cents: number
  source: string
  /** Balance of the station wallet that actually pays for model usage. */
  station_balance_cents?: number
  /** Main-site balance, shown for reference only. */
  main_balance_cents?: number
}

export interface AgentBalanceHistoryItem {
  type: string
  value: number
  status: string
  created_at?: string
  used_at?: string
  note?: string
  balance_after?: number
}

export interface AgentBalanceHistoryPage {
  items: AgentBalanceHistoryItem[]
  total: number
  page: number
  page_size: number
  total_recharged: number
}

export interface AgentMainUsageItem {
  request_id: string
  model: string
  input_tokens: number
  output_tokens: number
  total_cost: number
  actual_cost: number
  created_at?: string
}

export interface AgentMemberFacts {
  main_user_id: string
  balance_cents: number
  frozen_balance_cents: number
  balance_available: boolean
  history_available: boolean
  usage_available: boolean
  history?: AgentBalanceHistoryPage
  usage?: { items: AgentMainUsageItem[]; total: number; page: number; page_size: number }
}

export interface StationBillingSettings {
  enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  model_rate_multiplier: number
  subscription_usd_to_cny_rate: number
  recharge_fee_rate: number
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string
  cancel_rate_limit_enabled: boolean
  cancel_rate_limit_max: number
  cancel_rate_limit_window: number
  cancel_rate_limit_unit: string
  cancel_rate_limit_window_mode: string
  alipay_force_qrcode: boolean
  alipay_mobile_precreate_deep_link: boolean
  payment_visible_method_alipay_source: string
  payment_visible_method_wxpay_source: string
  payment_visible_method_alipay_enabled: boolean
  payment_visible_method_wxpay_enabled: boolean
}

export interface StationPaymentProvider {
  id?: number
  provider_key: string
  name: string
  config?: Record<string, string>
  config_configured?: Record<string, boolean>
  supported_types: string[]
  enabled: boolean
  payment_mode: string
  sort_order: number
  limits: string
  refund_enabled: boolean
  allow_user_refund: boolean
  webhook_path?: string
}

export interface StationPaymentPlan {
  id?: number
  name: string
  description: string
  price: number
  original_price?: number
  currency: string
  validity_days: number
  validity_unit: string
  features: string[]
  product_name: string
  for_sale: boolean
  sort_order: number
}

export interface StationBillingDashboard {
  orders_total: number
  sampled: number
  partial: boolean
  completed_count: number
  pending_count: number
  refunded_count: number
  amount_available: boolean
  completed_pay_amount?: number
  currency?: string
}

export interface AgentPaymentConfig {
  enabled: boolean
  provider: string
  currency: string
  merchant_id?: string
  webhook_secret_configured: boolean
  min_amount_cents: number
  max_amount_cents: number
  order_ttl_seconds: number
  checkout_url_template?: string
}

export interface AgentPaymentConfigInput {
  enabled: boolean
  provider: string
  currency: string
  merchant_id: string
  webhook_secret?: string
  min_amount_cents: number
  max_amount_cents: number
  order_ttl_seconds: number
  checkout_url_template: string
}

export interface AgentStationOrder {
  order_no: string
  amount_cents: number
  currency: string
  status: string
  payment_url?: string
}

export interface AgentStationRecharge {
  enabled: boolean
  currency: string
  min_amount_cents: number
  max_amount_cents: number
  items: AgentStationOrder[]
  recharge_url?: string
}

export interface AgentPaymentStats {
  source: string
  members_total: number
  members_sampled: number
  orders_sampled: number
  orders_reported: number
  completed_count: number
  pending_count: number
  refunded_count: number
  refund_requested_count: number
  completed_pay_amount?: number
  currency?: string
  amount_available: boolean
  partial: boolean
  lookup_failures: number
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
  api_base_url?: string
}

export interface AgentView extends AgentHomeSettings {
  agent_id: string
  domain: string
  api_base_url?: string
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

export interface AgentUserPage {
  items: AgentUserView[]
  total: number
  page: number
  page_size: number
}

export interface AgentContextResponse {
  agent: AgentView
  authenticated: boolean
  is_agent_admin: boolean
  main_user_id?: string
  user?: AgentUserView
  balance_error?: string
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

export interface AgentCheckoutInfo {
  methods: Record<string, AgentPaymentMethodLimit>
  global_min: number
  global_max: number
  plans: AgentCheckoutPlan[]
  balance_disabled: boolean
  charging_enabled?: boolean
  model_rate_multiplier: number
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
  stripe_publishable_key?: string
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

export interface AgentUsageView {
  request_id: string
  usage_id?: string
  proxy_main_user_id: string
  billing_main_user_id: string
  reserved_cents: number
  actual_cents: number
  charged_cents: number
  station_rate_multiplier: number
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

// Current records are sourced from the public user-scoped usage API. Keep the
// earlier runtime/admin labels readable for snapshots persisted by older builds.
export function isAuthoritativeAgentUsageSource(source?: string): boolean {
  return source === 'sub2api_user_usage'
    || source === 'sub2api_owner_usage'
    || source === 'sub2api_owner_runtime_usage'
    || source === 'sub2api_admin_usage'
}

export interface AgentKeyGroup {
  id: number
  name: string
  platform: string
  description?: string
  status: string
  rate_multiplier: number
}

export interface AgentAPIKeyView {
  id: number
  name: string
  prefix: string
  key?: string
  group_id?: number
  group_name?: string
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
  charged_cents: number
  station_rate_multiplier: number
  status: string
  error?: string
  usage_id?: string
  created_at?: string
  updated_at?: string
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

export interface LoginOptions {
  shared_runtime: boolean
  tenant_resolved: boolean
  password_login_enabled: boolean
  main_site_sso_enabled: boolean
  registration_enabled: boolean
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
    loginOptions: async () => (await agentClient.get<LoginOptions>('/auth/login-options')).data,
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
  getWallet: async () => (await agentClient.get<{ agent: AgentView; user: AgentUserView }>('/agent/wallet')).data,
  getUsageInsights: async (window: string) =>
    (await agentClient.get<UsageInsights>('/agent/usage/insights', { params: { window } })).data,
  getUsage: async (page = 1, pageSize = 25, filters: { model?: string; request_id?: string; start_time?: string; end_time?: string } = {}) =>
    (await agentClient.get<{ items: AgentUsageView[]; total: number; page: number; page_size: number }>('/agent/usage', {
      params: { ...filters, page, page_size: pageSize },
    })).data,
  affiliate: {
    get: async () => (await agentClient.get<AgentAffiliateDetail>('/agent/affiliate')).data,
    transfer: async () => (await agentClient.post<AgentAffiliateTransfer>('/agent/affiliate/transfer', {})).data,
    bind: async (affCode: string) => (await agentClient.post<{ bound: boolean }>('/agent/affiliate/bind', { aff_code: affCode })).data,
    invites: async (page = 1, pageSize = 20) =>
      (await agentClient.get<AgentAffiliatePage<AgentAffiliateInvite>>('/agent/affiliate/invites', { params: { page, page_size: pageSize } })).data,
    rebates: async (page = 1, pageSize = 20) =>
      (await agentClient.get<AgentAffiliatePage<AgentAffiliateRebate>>('/agent/affiliate/rebates', { params: { page, page_size: pageSize } })).data,
    transfers: async (page = 1, pageSize = 20) =>
      (await agentClient.get<AgentAffiliatePage<AgentAffiliateTransferRecord>>('/agent/affiliate/transfers', { params: { page, page_size: pageSize } })).data,
  },
  redeem: {
    submit: async (code: string) => (await agentClient.post<AgentRedeemResult>('/agent/redeem', { code })).data,
    history: async () => (await agentClient.get<{ items: AgentRedeemHistoryItem[]; total: number }>('/agent/redeem/history')).data,
  },
  paymentStats: async () => (await agentClient.get<AgentPaymentStats>('/agent/admin/payments')).data,
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
  balance: {
    get: async () => (await agentClient.get<AgentBalanceSnapshot>('/agent/balance')).data,
    history: async (page = 1, pageSize = 20) =>
      (await agentClient.get<AgentBalanceHistoryPage>('/agent/balance/history', { params: { page, page_size: pageSize } })).data,
  },
  adminUsers: {
    list: async (page = 1, pageSize = 20, search = '', status = '') =>
      (await agentClient.get<AgentUserPage>('/agent/admin/users', {
        params: { page, page_size: pageSize, ...(search ? { search } : {}), ...(status ? { status } : {}) },
      })).data,
    facts: async (mainUserID: string, page = 1, pageSize = 20) =>
      (await agentClient.get<AgentMemberFacts>('/agent/admin/member-facts', {
        params: { main_user_id: mainUserID, page, page_size: pageSize },
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
  getAdminWallet: async () => (await agentClient.get<AgentView>('/agent/admin/wallet')).data,
  getBranding: async () => (await agentClient.get<AgentBranding>('/agent/admin/branding')).data,
  billing: {
    refundEligibleProviders: async () => (await agentClient.get<{ provider_instance_ids: string[] }>('/agent/billing/orders/refund-eligible-providers')).data,
    settings: async () => (await agentClient.get<StationBillingSettings>('/agent/admin/billing/config')).data,
    saveSettings: async (payload: StationBillingSettings) => (await agentClient.put<StationBillingSettings>('/agent/admin/billing/config', payload)).data,
    providers: async () => (await agentClient.get<{ items: StationPaymentProvider[] }>('/agent/admin/billing/providers')).data,
    createProvider: async (payload: StationPaymentProvider) => (await agentClient.post<StationPaymentProvider>('/agent/admin/billing/providers', payload)).data,
    updateProvider: async (id: number, payload: Partial<StationPaymentProvider>) => (await agentClient.put<StationPaymentProvider>(`/agent/admin/billing/providers/${id}`, payload)).data,
    deleteProvider: async (id: number) => (await agentClient.delete<{ message: string }>(`/agent/admin/billing/providers/${id}`)).data,
    plans: async () => (await agentClient.get<{ items: StationPaymentPlan[] }>('/agent/admin/billing/plans')).data,
    createPlan: async (payload: StationPaymentPlan) => (await agentClient.post<StationPaymentPlan>('/agent/admin/billing/plans', payload)).data,
    updatePlan: async (id: number, payload: StationPaymentPlan) => (await agentClient.put<StationPaymentPlan>(`/agent/admin/billing/plans/${id}`, payload)).data,
    deletePlan: async (id: number) => (await agentClient.delete<{ message: string }>(`/agent/admin/billing/plans/${id}`)).data,
    orders: async (page = 1, pageSize = 20, status = '') => (await agentClient.get<AgentOrderPage>('/agent/admin/billing/orders', { params: { page, page_size: pageSize, ...(status ? { status } : {}) } })).data,
    dashboard: async () => (await agentClient.get<StationBillingDashboard>('/agent/admin/billing/dashboard')).data,
    cancelOrder: async (id: number) => (await agentClient.post<{ message: string }>(`/agent/admin/billing/orders/${id}/cancel`, {})).data,
    refundOrder: async (id: number) => (await agentClient.post<{ message: string; status: string }>(`/agent/admin/billing/orders/${id}/refund`, {})).data,
    myOrders: async (page = 1, pageSize = 20, status = '') => (await agentClient.get<AgentOrderPage>('/agent/billing/orders', { params: { page, page_size: pageSize, ...(status ? { status } : {}) } })).data,
    cancelMyOrder: async (id: number) => (await agentClient.post<{ message: string }>(`/agent/billing/orders/${id}/cancel`, {})).data,
    requestMyRefund: async (id: number, reason: string) => (await agentClient.post<{ message: string }>(`/agent/billing/orders/${id}/refund-request`, { reason })).data,
  },
  paymentConfig: async () => (await agentClient.get<AgentPaymentConfig>('/agent/admin/payment-config')).data,
  updatePaymentConfig: async (payload: AgentPaymentConfigInput) =>
    (await agentClient.put<AgentPaymentConfig>('/agent/admin/payment-config', payload)).data,
  stationRecharge: {
    get: async () => (await agentClient.get<AgentStationRecharge>('/agent/recharge/orders')).data,
    create: async (amount: string) => (await agentClient.post<{ order: AgentStationOrder; enabled: boolean; message: string }>('/agent/recharge/orders', { amount })).data,
  },
  updateBranding: async (payload: Partial<AgentBranding>) =>
    (await agentClient.put<AgentBranding>('/agent/admin/branding', payload)).data,
  uploadLogo: async (file: File) => {
    const body = new FormData()
    body.append('file', file)
    return (await agentClient.post<AgentBranding>('/agent/admin/branding/logo', body, {
      transformRequest: [(data, headers) => {
        if (headers && typeof (headers as { delete?: (name: string) => void }).delete === 'function') {
          (headers as { delete: (name: string) => void }).delete('Content-Type')
        }
        return data
      }],
    })).data
  },
  adminAnnouncements: {
    list: async () => (await agentClient.get<{ items: AgentAnnouncement[]; total: number }>('/agent/admin/announcements')).data,
    create: async (payload: AgentAnnouncementInput) => (await agentClient.post<AgentAnnouncement>('/agent/admin/announcements', payload)).data,
    update: async (id: number, payload: AgentAnnouncementInput) => (await agentClient.put<AgentAnnouncement>(`/agent/admin/announcements/${id}`, payload)).data,
    delete: async (id: number) => { await agentClient.delete(`/agent/admin/announcements/${id}`) },
  },
  getSettlements: async () => (await agentClient.get<{ items: SettlementView[]; total: number }>('/agent/admin/settlements')).data,
  reconcileSettlements: async (requestId?: string) =>
    (await agentClient.post<{ items: SettlementView[]; total: number }>('/agent/admin/settlements/reconcile', requestId ? { request_id: requestId } : {})).data,
  keys: {
    list: async () => (await agentClient.get<{ items: AgentAPIKeyView[]; total: number; api_base_url?: string }>('/api-keys')).data,
    groups: async () => (await agentClient.get<{ items: AgentKeyGroup[]; total: number }>('/api-keys/groups')).data,
    create: async (name: string, groupId: number) => (await agentClient.post<{ item: AgentAPIKeyView; key: string }>('/api-keys', { name, group_id: groupId })).data,
    revoke: async (id: number) => (await agentClient.delete<{ id: number; status: string }>(`/api-keys/${id}`)).data,
  },
}
