import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentPaymentConfigView from './AgentPaymentConfigView.vue'
import { agentAPI, type StationBillingSettings, type StationPaymentProvider } from '@/agent/api'

const alwaysVisible = [
  '支付设置',
  '配置支付系统选项',
  '支付配置指南',
  '启用支付',
  '保存设置',
]

const enabledOnly = [
  '商品名前缀',
  '商品名后缀',
  '预览',
  '最低金额',
  '最高金额',
  '每日限额',
  '模型计费倍率',
  '订阅 CNY 换算汇率',
  '充值手续费率',
  '订单超时时间',
  '最大待支付订单数',
  '限制取消频率',
  '支付宝强制二维码支付',
  '支付宝移动端当面付唤起',
  '帮助图片',
  '帮助文本',
  '启用的服务商',
  '服务商管理',
  '管理服务商',
  '添加服务商',
]

function disabledSettings(): StationBillingSettings {
  return {
    enabled: false,
    min_amount: 1,
    max_amount: 10000,
    daily_limit: 0,
    order_timeout_minutes: 30,
    max_pending_orders: 3,
    enabled_payment_types: [],
    balance_disabled: false,
    model_rate_multiplier: 1,
    subscription_usd_to_cny_rate: 0,
    recharge_fee_rate: 0,
    load_balance_strategy: 'round-robin',
    product_name_prefix: '',
    product_name_suffix: '',
    help_image_url: '',
    help_text: '',
    cancel_rate_limit_enabled: false,
    cancel_rate_limit_max: 10,
    cancel_rate_limit_window: 1,
    cancel_rate_limit_unit: 'day',
    cancel_rate_limit_window_mode: 'rolling',
    alipay_force_qrcode: false,
    alipay_mobile_precreate_deep_link: false,
    payment_visible_method_alipay_source: '',
    payment_visible_method_wxpay_source: '',
    payment_visible_method_alipay_enabled: true,
    payment_visible_method_wxpay_enabled: true,
  }
}

function provider(): StationPaymentProvider {
  return {
    id: 7,
    provider_key: 'easypay',
    name: '本站易支付',
    config: { pid: '1001' },
    supported_types: ['alipay'],
    enabled: true,
    payment_mode: 'popup',
    sort_order: 1,
    limits: '',
    refund_enabled: false,
    allow_user_refund: false,
  }
}

describe('支付配置', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('allows configuring a disabled station without enabling payments', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue({
      agent: {
        agent_id: 'agent-a', domain: 'a.example', name: 'A', site_name: 'A', status: 'active',
        billing_mode: 'user_upstream', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
      },
      authenticated: true,
      is_agent_admin: true,
    } as Awaited<ReturnType<typeof agentAPI.getContext>>)
    vi.spyOn(agentAPI.billing, 'settings').mockResolvedValue(disabledSettings())
    vi.spyOn(agentAPI.billing, 'providers').mockResolvedValue({ items: [provider()] })
    vi.spyOn(agentAPI.billing, 'saveSettings').mockImplementation(async payload => payload)
    vi.spyOn(agentAPI.billing, 'updateProvider').mockResolvedValue(provider())

    const paymentConfig = vi.spyOn(agentAPI, 'paymentConfig').mockRejectedValue(new Error('legacy payment config must not load'))
    const wrapper = mount(AgentPaymentConfigView, {
      global: { stubs: { Teleport: { template: '<div><slot /></div>' } } },
    })
    await flushPromises()

    for (const label of alwaysVisible) {
      expect(wrapper.text()).toContain(label)
    }
    expect(wrapper.text()).toContain('商品名前缀')
    expect(wrapper.text()).toContain('当前未开放收款')
    expect(wrapper.text()).not.toContain('商户号')
    expect(wrapper.text()).not.toContain('Webhook 密钥')
    expect(wrapper.text()).not.toContain('收银台地址')
    expect(wrapper.text()).not.toContain('checkout_url')
    expect(paymentConfig).not.toHaveBeenCalled()

    const enable = wrapper.find('[role="switch"]')
    expect(enable.exists()).toBe(true)
    expect(enable.attributes('aria-checked')).toBe('false')
    await wrapper.find('input[placeholder="Sub2API"]').setValue('本站商品')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(agentAPI.billing.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      enabled: false, product_name_prefix: '本站商品',
    }))
    expect(enable.attributes('aria-checked')).toBe('false')

    for (const label of enabledOnly) {
      expect(wrapper.text()).toContain(label)
    }
    expect(wrapper.text()).toContain('滚动')
    expect(wrapper.text()).toContain('分钟')
    expect(wrapper.find('input[type="number"]').exists()).toBe(true)

    await wrapper.findAll('button').find(button => button.text() === '易支付')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text().includes('添加服务商'))!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('添加服务商')
    expect(wrapper.text()).toContain('服务商名称')
    expect(enable.attributes('aria-checked')).toBe('false')

    await wrapper.findAll('button').find(button => button.text() === '删除')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('删除服务商')
    expect(wrapper.text()).toContain('确定要删除此服务商吗？')
    expect(wrapper.text()).toContain('保存设置')
    expect(wrapper.text()).toContain('服务商管理')
    expect(wrapper.text()).toContain('支付宝强制二维码支付')
    expect(wrapper.text()).toContain('支付宝移动端当面付唤起')
    expect(wrapper.text()).not.toContain('商户号')
    expect(wrapper.text()).not.toContain('Webhook 密钥')
    expect(wrapper.text()).not.toContain('收银台地址')
    expect(wrapper.text()).not.toContain('checkout_url')
    wrapper.unmount()
  })

  it('saves the payment master switch immediately and offers activation after a provider is enabled', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue({
      agent: {
        agent_id: 'agent-a', domain: 'a.example', name: 'A', site_name: 'A', status: 'active',
        billing_mode: 'user_upstream', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
      },
      authenticated: true,
      is_agent_admin: true,
    } as Awaited<ReturnType<typeof agentAPI.getContext>>)
    vi.spyOn(agentAPI.billing, 'settings').mockResolvedValue({
      ...disabledSettings(),
      enabled_payment_types: ['alipay'],
    })
    const alipayProvider = { ...provider(), provider_key: 'alipay', name: '本站支付宝', supported_types: ['alipay'] }
    vi.spyOn(agentAPI.billing, 'providers').mockResolvedValue({ items: [alipayProvider] })
    const save = vi.spyOn(agentAPI.billing, 'saveSettings').mockImplementation(async payload => payload)
    vi.spyOn(agentAPI.billing, 'updateProvider').mockResolvedValue(provider())

    const wrapper = mount(AgentPaymentConfigView, {
      global: { stubs: { Teleport: { template: '<div><slot /></div>' } } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('支付渠道已配置，但站点收款尚未开启')
    const activate = wrapper.findAll('button').find(button => button.text() === '立即开启收款')
    expect(activate).toBeDefined()
    await activate!.trigger('click')
    await flushPromises()

    expect(save).toHaveBeenLastCalledWith(expect.objectContaining({
      enabled: true,
      enabled_payment_types: ['alipay'],
    }))
    expect(wrapper.find('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.text()).toContain('支付已开启并保存')
    wrapper.unmount()
  })
})
