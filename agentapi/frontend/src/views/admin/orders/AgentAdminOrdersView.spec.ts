import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminOrdersView from './AgentAdminOrdersView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Agent', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

describe('AgentAdminOrdersView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('loads tenant context before mounting the main-style order panel', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const wrapper = mount(AgentAdminOrdersView, {
      global: { stubs: { AgentAdminOrdersPanel: { template: '<div data-testid="orders-panel">tenant orders</div>' } } },
    })

    expect(wrapper.find('[data-testid="orders-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="orders-panel"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('订单管理')
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('Sub2API 主站创建的订单')
  })

  it('hides order management when tenant context cannot be confirmed and retries safely', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const wrapper = mount(AgentAdminOrdersView, {
      global: { stubs: { AgentAdminOrdersPanel: { template: '<div data-testid="orders-panel">tenant orders</div>' } } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载订单管理')
    expect(wrapper.find('[data-testid="orders-panel"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="orders-panel"]').exists()).toBe(true)
  })
})
