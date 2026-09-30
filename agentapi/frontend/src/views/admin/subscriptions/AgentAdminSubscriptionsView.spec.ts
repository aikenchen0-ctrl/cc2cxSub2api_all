import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminSubscriptionsView from './AgentAdminSubscriptionsView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Agent', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

describe('AgentAdminSubscriptionsView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before mounting the read-only subscription panel', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const wrapper = mount(AgentAdminSubscriptionsView, {
      global: { stubs: { AgentAdminSubscriptionsPanel: { template: '<div data-testid="subscriptions-panel">tenant subscriptions</div>' } } },
    })

    expect(wrapper.find('[data-testid="subscriptions-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="subscriptions-panel"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('订阅管理')
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('主站订阅账本保持只读')
  })

  it('does not mount subscription readers until context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const wrapper = mount(AgentAdminSubscriptionsView, {
      global: { stubs: { AgentAdminSubscriptionsPanel: { template: '<div data-testid="subscriptions-panel">tenant subscriptions</div>' } } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载订阅管理')
    expect(wrapper.find('[data-testid="subscriptions-panel"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="subscriptions-panel"]').exists()).toBe(true)
  })
})
