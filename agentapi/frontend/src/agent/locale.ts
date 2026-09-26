const labels: Record<string, string> = {
  active: '正常',
  disabled: '已停用',
  suspended: '已暂停',
  revoked: '已撤销',
  pending: '待处理',
  paid_pending_allocation: '已付款，待分配',
  allocated: '已分配',
  paid: '已付款',
  failed: '失败',
  closed: '已关闭',
  expired: '已过期',
  confirmed: '已结算',
  released: '已退回',
  queued: '排队中',
  processing: '处理中',
  in_progress: '处理中',
  completed: '已完成',
  complete: '已完成',
  done: '已完成',
  succeeded: '成功',
  success: '成功',
  error: '出错',
  cancelled: '已取消',
  canceled: '已取消',
  rejected: '已拒绝',
  not_recorded: '未记录',
  unknown: '未知',
  owner_upstream: '由代理站主账户计费',
  local: '本地',
  manual: '手动',
  enabled: '已启用',
  ready: '就绪',
  ok: '正常',
  owner_changed: '主账户已变更',
  redacted: '已隐藏',
  funded: '余额充足',
  insufficient: '余额不足',
  insufficient_balance: '余额不足',
  overallocated: '已超额分配',
  unavailable: '暂不可用',
  generated: '已生成',
  provisioning: '正在开通',
  waiting: '等待中',
  standard: '标准',
  priority: '优先',
  flex: '弹性',
  xhigh: '极高',
  low: '低',
  medium: '中',
  high: '高',
  minimal: '最低',
  none: '无',
  user: '用户',
  assistant: '助手',
  chat: '对话',
  token: '按令牌计费',
  text: '文本',
  image: '图片',
  video: '视频',
  images: '图片',
  videos: '视频',
  responses: '响应接口',
  completions: '文本补全',
  streaming: '流式传输',
  synchronous: '同步',
  asynchronous: '异步',
  agent_admin: '代理站管理员',
  agent_user: '代理站用户',
  system: '系统',
}

const operationLabels: Record<string, string> = {
  'password_change': '修改密码',
  'recharge.allocate': '分配充值款',
  'branding.update': '更新品牌信息',
  'models.update': '更新模型权限',
  'wallet.sync': '同步主账户余额',
  'settlements.reconcile': '核对待结算记录',
  'wallet.allocate': '分配余额',
  'user.status.update': '更新用户状态',
  'api_key.create': '创建 API 密钥',
  'api_key.revoke': '撤销 API 密钥',
}

const targetLabels: Record<string, string> = {
  agent: '代理站',
  agent_model_policy: '模型权限',
  agent_wallet: '代理站钱包',
  recharge_order: '充值订单',
  agent_user: '代理站用户',
  settlement: '结算记录',
  agent_api_key: 'API 密钥',
  user: '用户',
}

const providerLabels: Record<string, string> = {
  manual: '手动转账',
  stripe: 'Stripe',
  airwallex: 'Airwallex',
  alipay: '支付宝',
  wechat: '微信支付',
  wxpay: '微信支付',
}

const currencyLabels: Record<string, string> = {
  CNY: '人民币',
  USD: '美元',
  EUR: '欧元',
  JPY: '日元',
  GBP: '英镑',
}

const reasonMessages: Record<string, string> = {
  HOST_NOT_ALLOWED: '当前访问域名未配置。',
  CORS_ORIGIN_REJECTED: '访问来源不受允许。',
  CSRF_ORIGIN_REJECTED: '请求来源校验失败，请刷新页面后重试。',
  AGENT_CONTROL_UNAVAILABLE: '代理站状态暂不可用，请稍后重试。',
  AGENT_SUSPENDED: '此代理站当前已暂停服务。',
  AGENT_REVOKED: '此代理站已停用。',
  AGENT_NOT_ACTIVE: '此代理站当前未启用。',
  AGENT_NOT_READY: '代理站配置尚未就绪，请稍后重试。',
  AGENT_ADMIN_REQUIRED: '需要代理站管理员权限。',
  NOT_FOUND: '请求的内容不存在。',
  METHOD_NOT_ALLOWED: '不支持此操作。',
  SSO_NOT_CONFIGURED: '单点登录尚未配置，请联系管理员。',
  SSO_TICKET_INVALID: '登录凭证无效或已过期，请重新进入。',
  SSO_USER_INVALID: '无法验证当前用户身份，请重新登录。',
  SESSION_CREATE_FAILED: '创建登录状态失败，请重试。',
  REFRESH_TOKEN_UNAVAILABLE: '登录状态已失效，请重新登录。',
  INVALID_REQUEST: '请求内容无效，请检查后重试。',
  INVALID_USERNAME: '用户名无效，请检查后重试。',
  INVALID_PASSWORD: '密码不符合要求，请检查后重试。',
  INVALID_AMOUNT: '金额无效，请输入正确金额。',
  UNAUTHORIZED: '登录状态已失效，请重新登录。',
  MAIN_SESSION_REQUIRED: '请使用主站账号和密码登录后再修改此信息。',
  MAIN_OWNER_MISSING: '代理站主账户尚未配置，请联系管理员。',
  AGENT_RUNTIME_CONTROL_CREDENTIAL_MISSING: '代理站控制凭据尚未配置，请联系管理员。',
  MAIN_APP_CREDENTIAL_MISSING: '模型服务凭据尚未配置，请联系管理员。',
  AGENT_USER_NOT_MAPPED: '此主站账号尚未关联到当前代理站。',
  AGENT_USER_NOT_FOUND: '当前用户未关联到此代理站。',
  AGENT_USER_DISABLED: '此账号在当前代理站已停用。',
  INVALID_AGENT_API_KEY: 'API 密钥无效或已撤销。',
  API_KEY_NOT_FOUND: '找不到此 API 密钥。',
  API_KEY_CREATE_FAILED: '创建 API 密钥失败，请重试。',
  API_KEY_REVOKE_FAILED: '撤销 API 密钥失败，请重试。',
  AGENT_INSUFFICIENT_BALANCE: '余额不足，请先充值或联系管理员分配余额。',
  MAIN_OWNER_BALANCE_INSUFFICIENT: '代理站主账户余额不足，暂时无法完成操作。',
  MAIN_OWNER_BALANCE_OVERALLOCATED: '主账户余额低于已分配额度，请联系管理员。',
  BILLING_BALANCE_UNAVAILABLE: '暂时无法读取计费余额，请稍后重试。',
  BILLING_NOT_READY: '计费配置尚未就绪，请联系管理员。',
  BILLING_MODE_INVALID: '计费配置无效，请联系管理员。',
  SETTLEMENT_PENDING: '此请求仍在核对中，请稍后再试。',
  IDEMPOTENCY_CONFLICT: '请求编号已用于其他操作，请刷新后重试。',
  IDEMPOTENCY_REPLAY: '此请求已处理，不会重复提交。',
  LOCAL_SETTLEMENT_FAILED: '操作结果尚未确认，请联系管理员核对后再重试。',
  SETTLEMENT_LOOKUP_FAILED: '读取结算记录失败，请稍后重试。',
  SETTLEMENT_PREPARE_FAILED: '准备结算失败，请稍后重试。',
  RECHARGE_NOT_ENABLED: '当前代理站未开放充值。',
  RECHARGE_ORDER_NOT_FOUND: '找不到此充值订单。',
  RECHARGE_NOT_PAID: '订单尚未付款，暂不能分配。',
  RECHARGE_ALLOCATION_FAILED: '分配充值款失败，请稍后重试。',
  MAIN_USER_STATUS_UPDATE_FAILED: '主站未确认用户状态更新，代理站访问仍保持关闭，请重试。',
  AGENT_USER_STATUS_CONFLICT: '此用户当前状态无法更改。',
  OWNER_STATUS_PROTECTED: '不能在此控制台停用代理站主账户。',
  BRANDING_UPDATE_FAILED: '保存品牌信息失败，请重试。',
  INVALID_BRANDING: '品牌信息无效，请检查后重试。',
  INVALID_MODEL_POLICY: '模型权限设置无效，请检查后重试。',
  MODEL_SCOPE_SYNC_UNAVAILABLE: '主站模型权限服务暂不可用，请稍后重试。',
  MODEL_SCOPE_SYNC_FAILED: '主站未确认模型权限变更，请稍后重试。',
  MODEL_POLICY_UNAVAILABLE: '读取模型权限失败，请刷新后重试。',
  MODEL_ROUTE_FORBIDDEN: '此模型接口不可用。',
  REQUEST_TOO_LARGE: '请求内容过大，请缩小内容后重试。',
  UPSTREAM_UNAVAILABLE: '主站服务暂时无法连接，请稍后重试。',
  UPSTREAM_AUTH_INVALID: '主站身份验证失败，请重新登录。',
  IMAGE_TASK_NOT_FOUND: '找不到此图片任务。',
  VIDEO_TASK_NOT_FOUND: '找不到此视频任务。',
  STORE_ERROR: '读取或保存数据失败，请稍后重试。',
}

const knownMessages: Record<string, string> = {
  'Network error. Check the AgentAPI connection.': '网络连接失败，请检查与 AgentAPI 的连接。',
  'AgentAPI request failed': '请求失败，请稍后重试。',
  'main site is temporarily unavailable': '主站服务暂时无法连接，请稍后重试。',
  'not found': '请求的内容不存在。',
  'method not allowed': '不支持此操作。',
}

export function statusLabel(value?: string | null): string {
  if (!value) return '未知'
  return labels[value.trim().toLowerCase().replace(/-/g, '_')] || '其他状态'
}

export function enumLabel(value?: string | null): string {
  if (!value) return '未知'
  return labels[value.trim().toLowerCase().replace(/-/g, '_')] || '其他'
}

export function taskTypeLabel(value?: string | null): string {
  if (value === 'image') return '图片'
  if (value === 'video') return '视频'
  return '任务'
}

export function operationLabel(value?: string | null): string {
  if (!value) return '其他操作'
  return operationLabels[value] || '其他操作'
}

export function targetLabel(value?: string | null): string {
  if (!value) return '其他对象'
  return targetLabels[value] || '其他对象'
}

export function providerLabel(value?: string | null): string {
  if (!value) return '未配置'
  return providerLabels[value.trim().toLowerCase()] || '其他支付渠道'
}

export function currencyLabel(value?: string | null): string {
  if (!value) return '未注明币种'
  return currencyLabels[value.trim().toUpperCase()] || '其他币种'
}

export function errorMessage(error: unknown, fallback: string): string {
  if (typeof error !== 'object' || error === null) return fallback
  const value = error as { message?: unknown; reason?: unknown; code?: unknown }
  const reason = typeof value.reason === 'string' ? value.reason : typeof value.code === 'string' ? value.code : ''
  if (reason && reasonMessages[reason]) return reasonMessages[reason]
  const message = typeof value.message === 'string' ? value.message.trim() : ''
  if (!message) return fallback
  if (/\p{Script=Han}/u.test(message)) {
    const withoutProductTerms = message.replace(/\b(?:AgentAPI|Sub2API|API|HTTP|HTTPS|SSO)\b/gi, '')
    return /[A-Za-z]{2,}/.test(withoutProductTerms) ? fallback : message
  }
  return knownMessages[message] || fallback
}
