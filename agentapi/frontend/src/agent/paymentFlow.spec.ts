import { describe, expect, it } from 'vitest'
import type { AgentPaymentCreateResult } from './api'
import {
  createAgentPaymentRecovery,
  readAgentPaymentRecovery,
  safePaymentURL,
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
  intent_id: 'int_77',
  currency: 'CNY',
  country_code: 'CN',
  payment_env: 'demo',
  expires_at: '2026-09-29T16:00:00Z',
}

describe('agent payment recovery', () => {
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
