import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentConsoleView from './AgentConsoleView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1',
    domain: 'agent.example.com',
    name: 'Example Agent',
    site_name: 'Example Agent',
    status: 'active',
    billing_mode: 'owner_upstream',
    main_balance_cents: 10000,
    billing_status: 'ok',
    wallet_available_cents: 8000,
    wallet_allocated_cents: 2000,
  },
  authenticated: true,
  is_agent_admin: true,
}

describe('AgentConsoleView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows mapped users and pending settlements and can reconcile them', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({
      total: 1,
      items: [{
        agent_id: 'agent-1',
        main_user_id: 'user-1',
        email: 'user@example.com',
        status: 'active',
        balance_cents: 250,
        created_at: '2026-09-22T00:00:00Z',
        updated_at: '2026-09-22T00:00:00Z',
      }],
    })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({
      total: 1,
      items: [{ request_id: 'req-pending', reserved_cents: 25, actual_cents: 0, status: 'pending' }],
    })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({
      enabled: false,
      provider: 'manual',
      items: [],
      total: 0,
    })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 1, items: [{ id: 1, actor_type: 'agent_admin', actor_id: 'owner-1', agent_id: 'agent-1', operation: 'wallet.sync', target_type: 'agent_wallet', request_id: 'req-audit', result: 'success', created_at: '2026-09-22T00:00:00Z' }] })
    const reconcile = vi.spyOn(agentAPI, 'reconcileSettlements').mockResolvedValue({ total: 1, items: [] })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()

    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('req-pending')
    expect(wrapper.text()).toContain('wallet.sync')

    const button = wrapper.findAll('button').find((item) => item.text().includes('Reconcile pending'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()
    expect(reconcile).toHaveBeenCalledOnce()
  })

  it('saves branding through the administrator API', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({ name: 'Edited', site_name: 'Edited Site', site_logo: '/edited.svg' })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('Edited')
    await inputs[1].setValue('Edited Site')
    await inputs[2].setValue('/edited.svg')
    const button = wrapper.findAll('button').find((item) => item.text().includes('Save branding'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith({ name: 'Edited', site_name: 'Edited Site', site_logo: '/edited.svg' })
    expect(wrapper.text()).toContain('Branding saved')
  })
})
