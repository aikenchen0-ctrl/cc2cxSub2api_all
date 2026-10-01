import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AgentOrder } from '@/agent/api'
import AgentPaymentResultView from './AgentPaymentResultView.vue'

const completedOrder: AgentOrder = {
  id: 77, amount: 50, pay_amount: 51, fee_rate: 2, currency: 'CNY', payment_type: 'alipay',
  out_trade_no: 'PAY-77', status: 'COMPLETED', order_type: 'balance', created_at: '2026-09-29T00:00:00Z',
  expires_at: '2026-09-29T01:00:00Z', completed_at: '2026-09-29T00:03:00Z', refund_amount: 0,
}

async function mountResult(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/payment/result', component: AgentPaymentResultView },
      { path: '/orders', component: { template: '<div>orders</div>' } },
      { path: '/purchase', component: { template: '<div>purchase</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentPaymentResultView, { global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return wrapper
}

describe('AgentPaymentResultView', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI.payment, 'verifyOrder').mockResolvedValue(completedOrder)
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('verifies the query order against the main site and clears recovery after success', async () => {
    const accountChanged = vi.fn()
    window.addEventListener('agentapi:account-facts-changed', accountChanged)
    window.localStorage.setItem('agentapi.payment.pending.v1', JSON.stringify({ out_trade_no: 'PAY-77' }))
    const wrapper = await mountResult('/payment/result?out_trade_no=PAY-77')

    expect(agentAPI.payment.verifyOrder).toHaveBeenCalledWith('PAY-77')
    expect(wrapper.text()).toContain('支付已确认')
    expect(wrapper.text()).toContain('PAY-77')
    expect(wrapper.text()).toContain('COMPLETED')
    expect(window.localStorage.getItem('agentapi.payment.pending.v1')).toBeNull()
    expect(accountChanged).toHaveBeenCalledTimes(1)
    expect((accountChanged.mock.calls[0][0] as CustomEvent).detail).toMatchObject({ refreshBalance: true, refreshSubscriptions: false })
    window.removeEventListener('agentapi:account-facts-changed', accountChanged)
    wrapper.unmount()
  })

  it('recovers a pending order number from local storage', async () => {
    window.localStorage.setItem('agentapi.payment.pending.v1', JSON.stringify({ out_trade_no: 'PAY-RECOVER' }))
    vi.mocked(agentAPI.payment.verifyOrder).mockResolvedValueOnce({ ...completedOrder, out_trade_no: 'PAY-RECOVER', status: 'PENDING' })
    const wrapper = await mountResult('/payment/result')

    expect(agentAPI.payment.verifyOrder).toHaveBeenCalledWith('PAY-RECOVER')
    expect(wrapper.text()).toContain('订单仍在处理中')
    expect(window.localStorage.getItem('agentapi.payment.pending.v1')).not.toBeNull()
    wrapper.unmount()
  })

  it('does not call the main site when no recoverable order number exists', async () => {
    const wrapper = await mountResult('/payment/result')

    expect(agentAPI.payment.verifyOrder).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('缺少可验证的订单号')
    wrapper.unmount()
  })
})
