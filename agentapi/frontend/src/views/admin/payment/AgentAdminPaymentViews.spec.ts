import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentAdminPaymentDashboardView from './AgentAdminPaymentDashboardView.vue'
import AgentAdminPaymentPlansView from './AgentAdminPaymentPlansView.vue'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

const dashboardGlobal = {
  stubs: {
    AgentPaymentDashboardPanel: { template: '<div data-testid="payment-dashboard-panel" />' },
  },
}

const plansGlobal = {
  stubs: {
    AgentPaymentPlansPanel: { template: '<div data-testid="payment-plans-panel" />' },
  },
}

describe('independent agent admin payment views', () => {
  afterEach(() => vi.restoreAllMocks())

  it('mounts tenant payment statistics only after context succeeds', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const wrapper = mount(AgentAdminPaymentDashboardView, { global: dashboardGlobal })

    expect(wrapper.find('[data-testid="payment-dashboard-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(wrapper.text()).toContain('支付统计')
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain('当前代理站映射用户')
    expect(wrapper.text()).toContain('不提供全局退款、支付密钥或供应商配置')
    expect(wrapper.find('[data-testid="payment-dashboard-panel"]').exists()).toBe(true)
  })

  it('keeps payment statistics hidden until context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const wrapper = mount(AgentAdminPaymentDashboardView, { global: dashboardGlobal })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载支付统计')
    expect(wrapper.find('[data-testid="payment-dashboard-panel"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="payment-dashboard-panel"]').exists()).toBe(true)
  })

  it('mounts tenant plan controls only after context succeeds', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const wrapper = mount(AgentAdminPaymentPlansView, { global: plansGlobal })

    expect(wrapper.find('[data-testid="payment-plans-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(wrapper.text()).toContain('套餐管理')
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain('价格与订阅事实')
    expect(wrapper.text()).toContain('按 agent_id 保存')
    expect(wrapper.find('[data-testid="payment-plans-panel"]').exists()).toBe(true)
  })

  it('keeps plan controls hidden until context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const wrapper = mount(AgentAdminPaymentPlansView, { global: plansGlobal })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载套餐管理')
    expect(wrapper.find('[data-testid="payment-plans-panel"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="payment-plans-panel"]').exists()).toBe(true)
  })
})
