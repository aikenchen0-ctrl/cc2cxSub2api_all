import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminUsageView from './AgentAdminUsageView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Agent', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

const settlements = [
  { request_id: 'req-pending', proxy_main_user_id: 'user-1', billing_main_user_id: 'user-1', reserved_cents: 20, actual_cents: 0, status: 'pending', model: 'gpt-5.5' },
  { request_id: 'req-confirmed', proxy_main_user_id: 'user-2', billing_main_user_id: 'user-2', reserved_cents: 20, actual_cents: 18, status: 'confirmed', model: 'gpt-image-2' },
]

describe('AgentAdminUsageView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before loading and rendering settlement records', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const settlementReader = vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: settlements, total: settlements.length })
    const wrapper = mount(AgentAdminUsageView)

    expect(settlementReader).not.toHaveBeenCalled()
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(settlementReader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('req-pending')
    expect(wrapper.text()).toContain('req-confirmed')
    expect(wrapper.text()).toContain('用户余额计费')
    expect(wrapper.text()).toContain('不能查看其他站点数据')
  })

  it('does not read usage before context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const settlementReader = vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: settlements, total: settlements.length })
    const wrapper = mount(AgentAdminUsageView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载用量同步')
    expect(settlementReader).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(settlementReader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('req-confirmed')
  })

  it('reconciles pending records through the tenant endpoint', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: settlements, total: settlements.length })
    const reconcile = vi.spyOn(agentAPI, 'reconcileSettlements').mockResolvedValue({ items: [settlements[1]], total: 1 })
    const wrapper = mount(AgentAdminUsageView)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text().includes('核对待结算记录'))!.trigger('click')
    await flushPromises()

    expect(reconcile).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('已核对 1 条待确认记录')
    expect(wrapper.text()).not.toContain('req-pending')
    expect(wrapper.text()).toContain('req-confirmed')
  })
})
