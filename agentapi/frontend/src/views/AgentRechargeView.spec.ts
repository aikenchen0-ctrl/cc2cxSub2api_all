import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import QRCode from 'qrcode'
import {
  agentAPI,
  type AgentCheckoutInfo,
  type AgentContextResponse,
  type AgentPaymentCreateResult,
} from '@/agent/api'
import AgentRechargeView from './AgentRechargeView.vue'
import type { WeixinJSBridgeLike } from '@/agent/wechatPayment'
import { createAgentPaymentRecovery, writeAgentPaymentRecovery } from '@/agent/paymentFlow'

vi.mock('qrcode', () => ({
  default: { toCanvas: vi.fn().mockResolvedValue(undefined) },
}))

const context: AgentContextResponse = {
  authenticated: true,
  is_agent_admin: false,
  main_user_id: '43',
  agent: {
    agent_id: 'agent-demo', domain: 'demo.example.com', name: 'Demo Agent', site_name: 'Demo API', status: 'active',
    billing_mode: 'user_direct', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  user: {
    agent_id: 'agent-demo', main_user_id: '43', email: 'user@example.com', display_name: 'Example User', status: 'active',
    balance_cents: 1234, created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
  },
}

const checkout: AgentCheckoutInfo = {
  methods: {
    alipay: {
      currency: 'CNY', display_name: '支付宝', daily_limit: 0, daily_used: 0, daily_remaining: 0,
      single_min: 10, single_max: 1000, fee_rate: 2, available: true,
    },
    stripe: {
      currency: 'CNY', display_name: 'Stripe', daily_limit: 2000, daily_used: 100, daily_remaining: 1900,
      single_min: 20, single_max: 500, fee_rate: 0, available: true,
    },
  },
  global_min: 10,
  global_max: 1000,
  plans: [{
    id: 9, group_id: 3, group_name: 'Pro', group_platform: 'openai', rate_multiplier: 1, name: '专业版月包', description: '面向稳定生产调用',
    price: 20, currency: 'USD', validity_days: 1, validity_unit: 'month', features: ['高并发'], product_name: 'Pro',
  }],
  balance_disabled: false,
  balance_recharge_multiplier: 1,
  subscription_usd_to_cny_rate: 7.2,
  recharge_fee_rate: 2,
  help_text: '支付帮助',
  help_image_url: '',
  stripe_publishable_key: '',
  alipay_force_qrcode: false,
  alipay_mobile_precreate_deep_link: false,
}

const createdPayment: AgentPaymentCreateResult = {
  order_id: 77,
  amount: 50,
  pay_amount: 51,
  fee_rate: 2,
  status: 'PENDING',
  result_type: 'order_created',
  payment_type: 'alipay',
  out_trade_no: 'PAY-77',
  qr_code: 'https://pay.example.com/qr/PAY-77',
  currency: 'CNY',
  expires_at: '2026-09-29T01:00:00Z',
}

function order(status = 'PENDING') {
  return {
    id: 77, amount: 50, pay_amount: 51, fee_rate: 2, currency: 'CNY', payment_type: 'alipay',
    out_trade_no: 'PAY-77', status, order_type: 'balance', created_at: '2026-09-29T00:00:00Z',
    expires_at: '2026-09-29T01:00:00Z', refund_amount: 0,
  }
}

async function mountView(path = '/purchase'): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/purchase', component: AgentRechargeView },
      { path: '/payment/qrcode', component: { template: '<div>qrcode</div>' } },
      { path: '/payment/stripe', component: { template: '<div>stripe</div>' } },
      { path: '/payment/airwallex', component: { template: '<div>airwallex</div>' } },
      { path: '/payment/result', component: { template: '<div>payment result</div>' } },
      { path: '/orders', component: { template: '<div>orders</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentRechargeView, {
    global: { plugins: [router], stubs: { Icon: true } },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentRechargeView', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI.payment, 'checkoutInfo').mockResolvedValue(checkout)
    vi.spyOn(agentAPI.payment, 'createOrder').mockResolvedValue(createdPayment)
    vi.spyOn(agentAPI.payment, 'verifyOrder').mockResolvedValue(order())
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(window, 'open').mockReturnValue({} as Window)
    window.localStorage.clear()
  })

  afterEach(() => {
    delete (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('renders the main-site checkout configuration and authoritative balance in place', async () => {
    const { wrapper } = await mountView()

    expect(wrapper.text()).toContain('Demo API')
    expect(wrapper.text()).toContain('$12.34')
    expect(wrapper.text()).toContain('余额充值')
    expect(wrapper.text()).toContain('订阅套餐')
    expect(wrapper.text()).toContain('支付宝')
    expect(wrapper.text()).toContain('Stripe')
    expect(wrapper.text()).toContain('支付帮助')
    expect(wrapper.text()).toContain('点击后将安全创建并计费订单')
    expect(wrapper.find('a[href="https://main.example.com/purchase"]').exists()).toBe(false)
    expect(agentAPI.payment.checkoutInfo).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('enforces the selected main-site payment method limits before creating an order', async () => {
    const { wrapper } = await mountView()
    await wrapper.get('#recharge-amount').setValue('5')

    expect(wrapper.text()).toContain('最低充值金额')
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('creates a main-site order and routes its QR flow without exposing a browser-selected user', async () => {
    const { wrapper, router } = await mountView()
    await wrapper.get('#recharge-amount').setValue('50')
    await wrapper.get('button.btn-primary.w-full').trigger('click')
    await flushPromises()

    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith({
      amount: 50,
      payment_type: 'alipay',
      order_type: 'balance',
      is_mobile: false,
      is_wechat_browser: false,
    })
    expect(router.currentRoute.value.path).toBe('/payment/qrcode')
    expect(router.currentRoute.value.query).toMatchObject({ order_id: '77', out_trade_no: 'PAY-77' })
    expect(QRCode.toCanvas).not.toHaveBeenCalled()
    expect(JSON.parse(window.localStorage.getItem('agentapi.payment.pending.v1') || '{}')).toMatchObject({ outTradeNo: 'PAY-77', orderId: 77, qrCode: createdPayment.qr_code })
    expect(agentAPI.payment.verifyOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('uses the selected main-site subscription plan when creating a subscription order', async () => {
    const { wrapper } = await mountView()
    const subscriptionTab = wrapper.findAll('button').find(button => button.text() === '订阅套餐')
    expect(subscriptionTab).toBeDefined()
    await subscriptionTab!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('专业版月包')
    expect(wrapper.get('[data-group-id="3"]').text()).toContain('Pro')

    await wrapper.get('button.btn-primary.w-full').trigger('click')
    await flushPromises()
    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith({
      amount: 20,
      payment_type: 'alipay',
      order_type: 'subscription',
      plan_id: 9,
      is_mobile: false,
      is_wechat_browser: false,
    })
    wrapper.unmount()
  })

  it('invokes the WeChat JSAPI payload returned by the main site and verifies on the result page', async () => {
    const wechatCheckout: AgentCheckoutInfo = {
      ...checkout,
      methods: {
        wxpay: {
          currency: 'CNY', display_name: '微信支付', daily_limit: 0, daily_used: 0, daily_remaining: 0,
          single_min: 10, single_max: 1000, fee_rate: 0, available: true,
        },
      },
    }
    const jsapi = {
      appId: 'wx-app', timeStamp: '123', nonceStr: 'nonce', package: 'prepay_id=1', signType: 'RSA', paySign: 'signed',
    }
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValueOnce(wechatCheckout)
    vi.mocked(agentAPI.payment.createOrder).mockResolvedValueOnce({
      ...createdPayment,
      payment_type: 'wxpay',
      result_type: 'jsapi_ready',
      qr_code: '',
      jsapi,
    })
    const invoke = vi.fn((_action: string, _payload: Record<string, unknown>, callback: (result: Record<string, unknown>) => void) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })
    ;(window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge = { invoke }

    const { wrapper, router } = await mountView()
    await wrapper.get('button.btn-primary.w-full').trigger('click')
    await flushPromises()

    expect(invoke).toHaveBeenCalledWith('getBrandWCPayRequest', jsapi, expect.any(Function))
    expect(router.currentRoute.value.path).toBe('/payment/result')
    expect(router.currentRoute.value.query).toMatchObject({ order_id: '77', out_trade_no: 'PAY-77' })
    expect(JSON.parse(window.localStorage.getItem('agentapi.payment.pending.v1') || '{}')).toMatchObject({ paymentType: 'wxpay', orderId: 77 })
    wrapper.unmount()
  })

  it('resumes a main-site WeChat OAuth order with the callback token and local recovery state', async () => {
    const wechatCheckout: AgentCheckoutInfo = {
      ...checkout,
      methods: {
        wxpay: {
          currency: 'CNY', display_name: '微信支付', daily_limit: 0, daily_used: 0, daily_remaining: 0,
          single_min: 10, single_max: 1000, fee_rate: 0, available: true,
        },
      },
    }
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValueOnce(wechatCheckout)
    const oauthResult: AgentPaymentCreateResult = {
      ...createdPayment,
      payment_type: 'wxpay',
      expires_at: '2099-09-29T01:00:00Z',
    }
    writeAgentPaymentRecovery(window.localStorage, createAgentPaymentRecovery(
      oauthResult,
      'balance',
      'wxpay',
      Date.now(),
    ))

    const { wrapper, router } = await mountView('/purchase?wechat_resume=1&wechat_resume_token=resume-secret')
    await flushPromises()

    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith({
      amount: 50,
      payment_type: 'wxpay',
      order_type: 'balance',
      wechat_resume_token: 'resume-secret',
      is_mobile: false,
      is_wechat_browser: false,
    })
    expect(router.currentRoute.value.query.wechat_resume_token).toBeUndefined()
    expect(router.currentRoute.value.path).toBe('/payment/qrcode')
    wrapper.unmount()
  })

  it('supports the legacy OpenID callback payload without putting identity selectors in the request', async () => {
    const wechatCheckout: AgentCheckoutInfo = {
      ...checkout,
      methods: {
        wxpay: {
          currency: 'CNY', display_name: '微信支付', daily_limit: 0, daily_used: 0, daily_remaining: 0,
          single_min: 10, single_max: 1000, fee_rate: 0, available: true,
        },
      },
    }
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValueOnce(wechatCheckout)

    const { wrapper } = await mountView('/purchase?wechat_resume=1&openid=wx-user&payment_type=wxpay&amount=20&order_type=balance')
    await flushPromises()

    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith({
      amount: 20,
      payment_type: 'wxpay',
      order_type: 'balance',
      openid: 'wx-user',
      is_mobile: false,
      is_wechat_browser: false,
    })
    expect(JSON.stringify(vi.mocked(agentAPI.payment.createOrder).mock.calls[0]?.[0])).not.toContain('user_id')
    wrapper.unmount()
  })

  it('shows a recoverable error when the main-site checkout configuration fails', async () => {
    vi.mocked(agentAPI.payment.checkoutInfo).mockRejectedValueOnce(new Error('checkout unavailable'))
    const { wrapper } = await mountView()

    expect(wrapper.get('[role="alert"]').text()).toContain('加载支付配置失败')
    expect(wrapper.text()).toContain('支付配置暂不可用')
    wrapper.unmount()
  })

  it('selects the matching renewal plan, not the first unrelated plan, without creating an order', async () => {
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValue({ ...checkout, plans: [
      checkout.plans[0], { ...checkout.plans[0], id: 12, group_id: 4, name: 'Gemini 续费', price: 30 },
    ] })
    const { wrapper } = await mountView('/purchase?tab=subscription&group=4')
    expect(wrapper.find('[data-plan-id="9"]').exists()).toBe(false)
    expect(wrapper.get('[data-plan-id="12"] button').attributes('aria-pressed')).toBe('true')
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    await wrapper.get('button.btn-primary.w-full').trigger('click')
    await flushPromises()
    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith(expect.objectContaining({ plan_id: 12, order_type: 'subscription', amount: 30 }))
    wrapper.unmount()
  })

  it('requires selection when several renewal plans share the group', async () => {
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValue({ ...checkout, plans: [
      checkout.plans[0], { ...checkout.plans[0], id: 12, name: '专业版年包', price: 200 },
    ] })
    const { wrapper } = await mountView('/purchase?tab=subscription&group=3')
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('选择续费套餐')
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    document.body.querySelector<HTMLButtonElement>('[role="dialog"] [data-plan-id="12"] button')!.click()
    await flushPromises()
    expect(wrapper.get('[data-plan-id="12"] button').attributes('aria-pressed')).toBe('true')
    await wrapper.get('button.btn-primary.w-full').trigger('click')
    await flushPromises()
    expect(agentAPI.payment.createOrder).toHaveBeenCalledWith(expect.objectContaining({ plan_id: 12, amount: 200 }))
    wrapper.unmount()
  })

  it.each(['999', '-1', '3.5', 'bad'])('does not silently purchase a different plan for unavailable group %s', async group => {
    const { wrapper } = await mountView(`/purchase?tab=subscription&group=${group}`)
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-plan-id]').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toMatch(/续费分组无效|没有可购买/)
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(button => button.text() === '查看全部套餐')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-plan-id="9"]').exists()).toBe(true)
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('revalidates a renewed catalog and removes a selected plan that is no longer sold', async () => {
    const { wrapper } = await mountView('/purchase?tab=subscription&group=3')
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValueOnce({ ...checkout, plans: [] })
    await wrapper.get('[aria-label="刷新支付配置"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('没有可购买的续费套餐')
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not create a resumed subscription order for an unavailable plan', async () => {
    vi.mocked(agentAPI.payment.checkoutInfo).mockResolvedValueOnce({ ...checkout, methods: { wxpay: checkout.methods.alipay } })
    const { wrapper } = await mountView('/purchase?wechat_resume=1&wechat_resume_token=resume&order_type=subscription&plan_id=999')
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('原订阅套餐已不可用')
    wrapper.unmount()
  })

  it('handles a renewal group change without remounting the purchase page', async () => {
    const { wrapper, router } = await mountView('/purchase?tab=subscription&group=3')
    expect(wrapper.get('[data-plan-id="9"] button').attributes('aria-pressed')).toBe('true')
    await router.push('/purchase?tab=subscription&group=999')
    await flushPromises()
    expect(wrapper.find('[data-plan-id]').exists()).toBe(false)
    expect(wrapper.get('button.btn-primary.w-full').attributes('disabled')).toBeDefined()
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('clears checkout facts after a failed refresh so stale plans cannot be submitted', async () => {
    const { wrapper } = await mountView('/purchase?tab=subscription&group=3')
    vi.mocked(agentAPI.payment.checkoutInfo).mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('[aria-label="刷新支付配置"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('button.btn-primary.w-full').exists()).toBe(false)
    expect(wrapper.text()).toContain('加载支付配置失败')
    expect(agentAPI.payment.createOrder).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
