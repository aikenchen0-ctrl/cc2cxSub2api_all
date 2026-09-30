import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AgentCheckoutInfo, type AgentPaymentCreateResult } from '@/agent/api'
import {
  AGENT_PAYMENT_RECOVERY_KEY,
  createAgentPaymentRecovery,
  writeAgentPaymentRecovery,
} from '@/agent/paymentFlow'
import AgentStripePaymentView from './AgentStripePaymentView.vue'

const stripeMocks = vi.hoisted(() => {
  const element = { mount: vi.fn(), on: vi.fn(), destroy: vi.fn() }
  const elements = { create: vi.fn(() => element) }
  const stripe = {
    elements: vi.fn(() => elements),
    confirmPayment: vi.fn().mockResolvedValue({}),
    confirmAlipayPayment: vi.fn().mockResolvedValue({}),
    confirmWechatPayPayment: vi.fn().mockResolvedValue({ paymentIntent: { status: 'succeeded' } }),
  }
  return { element, elements, stripe, loadStripe: vi.fn().mockResolvedValue(stripe) }
})

vi.mock('@stripe/stripe-js/pure', () => ({ loadStripe: stripeMocks.loadStripe }))

const checkout: AgentCheckoutInfo = {
  methods: {}, global_min: 0, global_max: 0, plans: [], balance_disabled: false,
  balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 7.2, recharge_fee_rate: 0,
  help_text: '', help_image_url: '', stripe_publishable_key: 'pk_test_public',
  alipay_force_qrcode: false, alipay_mobile_precreate_deep_link: false,
}
const result: AgentPaymentCreateResult = {
  order_id: 88, amount: 25, pay_amount: 25, fee_rate: 0, status: 'PENDING', payment_type: 'stripe',
  out_trade_no: 'PAY-000088', client_secret: 'pi_88_secret_browser', currency: 'CNY',
  expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
}

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/payment/stripe', component: AgentStripePaymentView },
      { path: '/payment/result', component: { template: '<div>result</div>' } },
      { path: '/purchase', component: { template: '<div>purchase</div>' } },
    ],
  })
  await router.push('/payment/stripe?order_id=88&out_trade_no=PAY-000088')
  await router.isReady()
  const wrapper = mount(AgentStripePaymentView, { global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentStripePaymentView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    writeAgentPaymentRecovery(window.localStorage, createAgentPaymentRecovery(result, 'balance', 'stripe'))
    vi.spyOn(agentAPI.payment, 'checkoutInfo').mockResolvedValue(checkout)
    stripeMocks.element.on.mockImplementation((event: string, callback: () => void) => { if (event === 'ready') callback() })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('loads the main-site public Stripe configuration and mounts Payment Element', async () => {
    const { wrapper, router } = await mountView()

    expect(agentAPI.payment.checkoutInfo).toHaveBeenCalledTimes(1)
    expect(stripeMocks.loadStripe).toHaveBeenCalledWith('pk_test_public')
    expect(stripeMocks.stripe.elements).toHaveBeenCalledWith(expect.objectContaining({ clientSecret: 'pi_88_secret_browser' }))
    expect(stripeMocks.element.mount).toHaveBeenCalledWith('#agent-stripe-payment-element')
    expect(wrapper.text()).toContain('¥25.00')

    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(stripeMocks.stripe.confirmPayment).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/payment/result')
    expect(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY)).toBeNull()
    wrapper.unmount()
  })

  it('does not trust query parameters when the protected recovery state is missing', async () => {
    window.localStorage.clear()
    const { wrapper } = await mountView()

    expect(wrapper.text()).toContain('Stripe 支付信息已失效')
    expect(stripeMocks.loadStripe).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
