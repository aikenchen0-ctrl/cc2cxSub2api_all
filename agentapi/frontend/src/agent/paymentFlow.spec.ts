import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentPaymentCreateResult } from './api'
import {
  createAgentPaymentRecovery,
  readAgentPaymentRecovery,
  safePaymentURL,
  paymentFee,
  shouldOpenPaymentWindow,
  openPaymentTab,
} from './paymentFlow'

const result: AgentPaymentCreateResult = {
  order_id: 77,
  amount: 50,
  pay_amount: 51,
  fee_rate: 2,
  status: 'PENDING',
  payment_type: 'stripe',
  out_trade_no: 'PAY-000077',
  client_secret: 'pi_secret_browser',
  stripe_publishable_key: 'pk_selected_instance',
  intent_id: 'int_77',
  currency: 'CNY',
  country_code: 'CN',
  payment_env: 'demo',
  expires_at: '2026-09-29T16:00:00Z',
}

describe('agent payment recovery', () => {
  it('applies the configured currency precision to fees', () => {
    expect(paymentFee(100, 2.5, 'JPY')).toBe(3)
    expect(paymentFee(71.5, 2.5, 'CNY')).toBe(1.79)
    expect(paymentFee(50, 2, 'USD')).toBe(1)
  })

  it('honors popup, QR and mobile precreate precedence', () => {
    const payment = { ...result, payment_type: 'alipay', pay_url: 'https://pay.example', qr_code: 'https://qr.example', payment_mode: 'popup' }
    expect(shouldOpenPaymentWindow(payment, false, false, false)).toBe(true)
    expect(shouldOpenPaymentWindow(payment, true, true, false)).toBe(false)
    expect(shouldOpenPaymentWindow(payment, true, true, true)).toBe(false)
    expect(shouldOpenPaymentWindow({ ...payment, payment_type: 'wxpay' }, true, true, false)).toBe(true)
  })
  it('round-trips the main-site payment handoff without user or admin selectors', () => {
    const snapshot = createAgentPaymentRecovery(result, 'balance', 'stripe', Date.parse('2026-09-29T14:00:00Z'), 9)
    const restored = readAgentPaymentRecovery(JSON.stringify(snapshot), {
      orderId: 77,
      outTradeNo: 'PAY-000077',
      now: Date.parse('2026-09-29T14:01:00Z'),
    })

    expect(restored).toMatchObject({
      orderId: 77,
      outTradeNo: 'PAY-000077',
      clientSecret: 'pi_secret_browser',
      stripePublishableKey: 'pk_selected_instance',
      intentId: 'int_77',
      orderType: 'balance',
      planId: 9,
    })
    expect(JSON.stringify(restored)).not.toContain('user_id')
    expect(JSON.stringify(restored)).not.toContain('admin')
  })

  it('keeps old recovery snapshots readable with no selected plan', () => {
    const snapshot = createAgentPaymentRecovery(result, 'balance', 'stripe', Date.parse('2026-09-29T14:00:00Z'))
    const legacy = JSON.parse(JSON.stringify(snapshot)) as Record<string, unknown>
    delete legacy.planId

    expect(readAgentPaymentRecovery(JSON.stringify(legacy), { now: Date.parse('2026-09-29T14:01:00Z') }))
      .toMatchObject({ planId: 0 })
  })

  it('rejects expired, mismatched, or malformed recovery state', () => {
    const snapshot = createAgentPaymentRecovery(result, 'balance', 'stripe', Date.parse('2026-09-29T14:00:00Z'))
    expect(readAgentPaymentRecovery(JSON.stringify(snapshot), { orderId: 78, now: Date.parse('2026-09-29T14:01:00Z') })).toBeNull()
    expect(readAgentPaymentRecovery(JSON.stringify(snapshot), { outTradeNo: 'OTHER', now: Date.parse('2026-09-29T14:01:00Z') })).toBeNull()
    expect(readAgentPaymentRecovery(JSON.stringify(snapshot), { now: Date.parse('2026-09-29T17:00:00Z') })).toBeNull()
    expect(readAgentPaymentRecovery('{not-json')).toBeNull()
  })

  it('accepts only HTTP(S) payment links', () => {
    expect(safePaymentURL('https://pay.example/checkout', 'https://agent.example')).toBe('https://pay.example/checkout')
    expect(safePaymentURL('/checkout', 'https://agent.example')).toBe('https://agent.example/checkout')
    expect(safePaymentURL('javascript:alert(1)', 'https://agent.example')).toBe('')
    expect(safePaymentURL('data:text/html,bad', 'https://agent.example')).toBe('')
  })
})

describe('openPaymentTab', () => {
  afterEach(() => vi.restoreAllMocks())

  it('opens the payment page as a tab without window features', () => {
    const tab = { opener: window } as unknown as Window
    const open = vi.spyOn(window, 'open').mockReturnValue(tab)
    expect(openPaymentTab('https://pay.example/checkout')).toBe(true)
    // 只传 '_blank'：带 features 会被浏览器当作弹窗新窗口。
    expect(open).toHaveBeenCalledWith('https://pay.example/checkout', '_blank')
    expect(open.mock.calls[0]).toHaveLength(2)
    expect(tab.opener).toBeNull()
  })

  it('reports a blocked tab so callers can fall back', () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    expect(openPaymentTab('https://pay.example/checkout')).toBe(false)
    expect(openPaymentTab('')).toBe(false)
  })
})
