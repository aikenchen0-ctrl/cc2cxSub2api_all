import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import QRCode from 'qrcode'
import { agentAPI, type AgentOrder, type AgentPaymentCreateResult } from '@/agent/api'
import {
  AGENT_PAYMENT_RECOVERY_KEY,
  createAgentPaymentRecovery,
  writeAgentPaymentRecovery,
} from '@/agent/paymentFlow'
import AgentPaymentQRCodeView from './AgentPaymentQRCodeView.vue'

vi.mock('qrcode', () => ({ default: { toCanvas: vi.fn().mockResolvedValue(undefined) } }))

const result: AgentPaymentCreateResult = {
  order_id: 77, amount: 50, pay_amount: 51, fee_rate: 2, status: 'PENDING', payment_type: 'alipay',
  out_trade_no: 'PAY-000077', qr_code: 'https://pay.example/qr/77', pay_url: 'https://pay.example/order/77',
  currency: 'CNY', expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
}
const completed: AgentOrder = {
  id: 77, amount: 50, pay_amount: 51, fee_rate: 2, currency: 'CNY', payment_type: 'alipay',
  out_trade_no: 'PAY-000077', status: 'COMPLETED', order_type: 'balance', created_at: '2026-09-29T00:00:00Z',
  expires_at: result.expires_at, refund_amount: 0,
}

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/payment/qrcode', component: AgentPaymentQRCodeView },
      { path: '/payment/result', component: { template: '<div>result</div>' } },
      { path: '/purchase', component: { template: '<div>purchase</div>' } },
    ],
  })
  await router.push('/payment/qrcode?order_id=77&out_trade_no=PAY-000077')
  await router.isReady()
  const wrapper = mount(AgentPaymentQRCodeView, { global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentPaymentQRCodeView', () => {
  beforeEach(() => {
    window.localStorage.clear()
    writeAgentPaymentRecovery(window.localStorage, createAgentPaymentRecovery(result, 'balance', 'alipay'))
    vi.spyOn(agentAPI.payment, 'verifyOrder').mockResolvedValue(completed)
    vi.spyOn(agentAPI.orders, 'cancel').mockResolvedValue({ message: 'cancelled' })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('renders the recovered QR order and verifies it against the main site', async () => {
    const { wrapper, router } = await mountView()

    expect(wrapper.text()).toContain('PAY-000077')
    expect(wrapper.get('[aria-label="主站支付二维码"]').exists()).toBe(true)
    expect(QRCode.toCanvas).toHaveBeenCalled()
    expect(wrapper.get('a[href="https://pay.example/order/77"]').attributes('rel')).toContain('noopener')

    const verifyButton = wrapper.findAll('button').find(button => button.text().includes('我已完成支付'))
    await verifyButton!.trigger('click')
    await flushPromises()
    expect(agentAPI.payment.verifyOrder).toHaveBeenCalledWith('PAY-000077')
    expect(router.currentRoute.value.path).toBe('/payment/result')
    expect(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY)).toBeNull()
    wrapper.unmount()
  })

  it('cancels only the recovered main-site order', async () => {
    vi.mocked(agentAPI.payment.verifyOrder).mockResolvedValueOnce({ ...completed, status: 'PENDING' })
    const { wrapper, router } = await mountView()
    const cancelButton = wrapper.findAll('button').find(button => button.text().includes('取消订单'))
    await cancelButton!.trigger('click')
    await flushPromises()

    expect(agentAPI.orders.cancel).toHaveBeenCalledWith(77)
    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY)).toBeNull()
    wrapper.unmount()
  })

  it('rejects missing recovery instead of trusting query payment details', async () => {
    window.localStorage.clear()
    const { wrapper } = await mountView()

    expect(wrapper.get('[role="alert"]').text()).toContain('支付信息已失效')
    expect(agentAPI.payment.verifyOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
