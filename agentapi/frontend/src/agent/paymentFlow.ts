import type { AgentPaymentCreateResult, AgentPaymentOrderRequest } from './api'

export const AGENT_PAYMENT_RECOVERY_KEY = 'agentapi.payment.pending.v1'

export interface AgentPaymentRecoverySnapshot {
  orderId: number
  outTradeNo: string
  amount: number
  payAmount: number
  feeRate: number
  paymentType: string
  orderType: AgentPaymentOrderRequest['order_type']
  payURL: string
  qrCode: string
  stripePublishableKey?: string
  alipayMobilePrecreateDeepLink?: boolean
  clientSecret: string
  intentId: string
  currency: string
  countryCode: string
  paymentEnv: string
  expiresAt: string
  paymentMode: string
  resumeToken: string
  planId: number
  createdAt: number
}

export function createAgentPaymentRecovery(
  result: AgentPaymentCreateResult,
  orderType: AgentPaymentOrderRequest['order_type'],
  paymentType: string,
  now = Date.now(),
  planId = 0,
): AgentPaymentRecoverySnapshot {
  return {
    orderId: result.order_id,
    outTradeNo: result.out_trade_no || '',
    amount: result.amount,
    payAmount: result.pay_amount,
    feeRate: result.fee_rate,
    paymentType: result.payment_type || paymentType,
    orderType,
    payURL: result.pay_url || '',
    qrCode: result.qr_code || '',
    stripePublishableKey: result.stripe_publishable_key || '',
    alipayMobilePrecreateDeepLink: result.alipay_mobile_precreate_deep_link === true,
    clientSecret: result.client_secret || '',
    intentId: result.intent_id || '',
    currency: result.currency || 'CNY',
    countryCode: result.country_code || 'CN',
    paymentEnv: result.payment_env || '',
    expiresAt: result.expires_at || '',
    paymentMode: result.payment_mode || '',
    resumeToken: result.resume_token || '',
    planId: Number.isInteger(planId) && planId > 0 ? planId : 0,
    createdAt: now,
  }
}

export function writeAgentPaymentRecovery(storage: Pick<Storage, 'setItem'>, snapshot: AgentPaymentRecoverySnapshot): void {
  storage.setItem(AGENT_PAYMENT_RECOVERY_KEY, JSON.stringify(snapshot))
}

export function clearAgentPaymentRecovery(storage: Pick<Storage, 'removeItem'>): void {
  storage.removeItem(AGENT_PAYMENT_RECOVERY_KEY)
}

export function readAgentPaymentRecovery(
  raw: string | null | undefined,
  options: { orderId?: number; outTradeNo?: string; now?: number } = {},
): AgentPaymentRecoverySnapshot | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as Partial<AgentPaymentRecoverySnapshot>
    if (parsed.planId === undefined) parsed.planId = 0
    if (
      typeof parsed.orderId !== 'number' || parsed.orderId <= 0
      || typeof parsed.outTradeNo !== 'string'
      || typeof parsed.amount !== 'number'
      || typeof parsed.payAmount !== 'number'
      || typeof parsed.feeRate !== 'number'
      || typeof parsed.paymentType !== 'string'
      || (parsed.orderType !== 'balance' && parsed.orderType !== 'subscription')
      || typeof parsed.payURL !== 'string'
      || typeof parsed.qrCode !== 'string'
      || typeof parsed.clientSecret !== 'string'
      || typeof parsed.intentId !== 'string'
      || typeof parsed.currency !== 'string'
      || typeof parsed.countryCode !== 'string'
      || typeof parsed.paymentEnv !== 'string'
      || typeof parsed.expiresAt !== 'string'
      || typeof parsed.paymentMode !== 'string'
      || typeof parsed.resumeToken !== 'string'
      || typeof parsed.planId !== 'number'
      || !Number.isInteger(parsed.planId)
      || parsed.planId < 0
      || typeof parsed.createdAt !== 'number'
    ) return null

    if (options.orderId && parsed.orderId !== options.orderId) return null
    if (options.outTradeNo && parsed.outTradeNo !== options.outTradeNo) return null
    const expiresAt = Date.parse(parsed.expiresAt)
    if (Number.isFinite(expiresAt) && expiresAt <= (options.now ?? Date.now())) return null
    return parsed as AgentPaymentRecoverySnapshot
  } catch {
    return null
  }
}

export function safePaymentURL(value: string | undefined, origin = window.location.origin): string {
  if (!value) return ''
  try {
    const parsed = new URL(value, origin)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : ''
  } catch {
    return ''
  }
}

export function paymentCurrencyDigits(currency: string): number {
  if (['BIF','CLP','DJF','GNF','JPY','KMF','KRW','MGA','PYG','RWF','VND','VUV','XAF','XOF','XPF','ISK','UGX'].includes(currency.toUpperCase())) return 0
  if (['BHD','IQD','JOD','KWD','LYD','OMR','TND'].includes(currency.toUpperCase())) return 3
  return 2
}

export function paymentFee(amount: number, rate: number, currency: string): number {
  if (amount <= 0 || rate <= 0) return 0
  const factor = 10 ** paymentCurrencyDigits(currency)
  const minor = amount * rate / 100 * factor
  return Math.ceil(minor - Number.EPSILON * Math.max(1, Math.abs(minor))) / factor
}

export function shouldOpenPaymentWindow(result: AgentPaymentCreateResult, isMobile: boolean, forceQR: boolean, mobilePrecreate: boolean): boolean {
  if (!result.pay_url) return false
  const alipay = result.payment_type === 'alipay' || result.payment_type === 'alipay_direct'
  if (alipay && isMobile && mobilePrecreate && result.qr_code) return false
  if (alipay && forceQR && !mobilePrecreate) return false
  return result.payment_mode === 'popup' || result.payment_mode === 'redirect' || isMobile || !result.qr_code
}

/**
 * 支付页统一用“新标签页”打开。
 * 注意：window.open 的第三个参数（features）里只要出现 width/height/popup 之类，
 * Chrome/Edge/Safari/Firefox 都会把它当成独立弹窗（新窗口）而不是标签页。
 * 这里刻意不传 features，只传 '_blank'，让浏览器按普通链接打开标签页。
 * 返回值 false 表示被拦截（或浏览器拒绝），调用方应回退到同标签页跳转。
 */
export function openPaymentTab(url: string): boolean {
  if (!url) return false
  const tab = window.open(url, '_blank')
  if (!tab) return false
  try {
    tab.opener = null
  } catch {
    // 跨域句柄可能只读，忽略即可；rel/noopener 语义已由清空 opener 覆盖。
  }
  return true
}