import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { AgentPaymentCreateResult } from '@/agent/api'
import { createAgentPaymentRecovery, writeAgentPaymentRecovery } from '@/agent/paymentFlow'
import AgentAirwallexPaymentView from './AgentAirwallexPaymentView.vue'

const airwallexMocks = vi.hoisted(() => ({
  redirectToCheckout: vi.fn(),
  init: vi.fn(),
}))

vi.mock('@airwallex/components-sdk', () => ({ init: airwallexMocks.init }))

const result: AgentPaymentCreateResult = {
  order_id: 99, amount: 30, pay_amount: 30, fee_rate: 0, status: 'PENDING', payment_type: 'airwallex',
  out_trade_no: 'PAY-000099', client_secret: 'awx_secret_browser', intent_id: 'int_99', currency: 'CNY',
  country_code: 'CN', payment_env: 'demo', expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
}

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/payment/airwallex', component: AgentAirwallexPaymentView },
      { path: '/purchase', component: { template: '<div>purchase</div>' } },
    ],
  })
  await router.push('/payment/airwallex?order_id=99&out_trade_no=PAY-000099')
  await router.isReady()
  const wrapper = mount(AgentAirwallexPaymentView, { global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return wrapper
}

describe('AgentAirwallexPaymentView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    writeAgentPaymentRecovery(window.localStorage, createAgentPaymentRecovery(result, 'balance', 'airwallex'))
    airwallexMocks.init.mockResolvedValue({ payments: { redirectToCheckout: airwallexMocks.redirectToCheckout } })
    airwallexMocks.redirectToCheckout.mockReturnValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('restores the main-site intent and starts Airwallex checkout', async () => {
    const wrapper = await mountView()

    expect(airwallexMocks.init).toHaveBeenCalledWith(expect.objectContaining({ env: 'demo', enabledElements: ['payments'] }))
    expect(airwallexMocks.redirectToCheckout).toHaveBeenCalledWith(expect.objectContaining({
      intent_id: 'int_99',
      client_secret: 'awx_secret_browser',
      currency: 'CNY',
      country_code: 'CN',
      successUrl: expect.stringContaining('/payment/result?out_trade_no=PAY-000099'),
    }))
    expect(wrapper.text()).toContain('支付窗口即将打开')
    wrapper.unmount()
  })

  it('rejects a query-only handoff without matching recovery state', async () => {
    window.localStorage.clear()
    const wrapper = await mountView()

    expect(wrapper.get('[role="alert"]').text()).toContain('Airwallex 支付信息已失效')
    expect(airwallexMocks.init).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
