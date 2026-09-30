import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentAdminPromoCodes, type AgentContextResponse } from '@/agent/api'
import AgentAdminPromoCodesView from './AgentAdminPromoCodesView.vue'

const context: AgentContextResponse = {
  agent: { agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active', billing_mode: 'user_upstream', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0 },
  authenticated: true,
  is_agent_admin: true,
}

const boundary: AgentAdminPromoCodes = {
  feature_enabled: false,
  funding_mode: 'unconfigured',
  authority: 'sub2api_main',
  management_scope: 'current_agent',
  redemption_enabled: false,
  can_create: false,
  can_edit: false,
  can_delete: false,
  items: [],
  total: 0,
}

describe('AgentAdminPromoCodesView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before loading the safe funding boundary', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const reader = vi.spyOn(agentAPI.adminPromoCodes, 'get').mockResolvedValue(boundary)
    const wrapper = mount(AgentAdminPromoCodesView)

    expect(reader).not.toHaveBeenCalled()
    await flushPromises()

    expect(reader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('优惠码管理')
    expect(wrapper.text()).toContain('Sub2API 主站')
    expect(wrapper.text()).toContain('优惠码资金模式尚未配置')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
  })

  it('does not expose global issuance or browser credentials', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI.adminPromoCodes, 'get').mockResolvedValue({
      ...boundary,
      items: [{ code: 'TENANT10', bonus_amount: 10, used_count: 1, max_uses: 5, status: 'active', expires_at: '', created_at: '2026-09-29T00:00:00Z' }],
      total: 1,
    })
    const wrapper = mount(AgentAdminPromoCodesView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('主站管理员 Key：')
    expect(wrapper.text()).not.toContain('SuperKey：')
    expect(wrapper.text()).not.toContain('立即发放')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[aria-label="优惠码状态"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-status="active"]').text()).toBe('有效')
  })

  it('stops before promo-code loading when tenant context fails and retries', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext').mockRejectedValueOnce(new Error('internal funding endpoint')).mockResolvedValueOnce(context)
    const reader = vi.spyOn(agentAPI.adminPromoCodes, 'get').mockResolvedValue(boundary)
    const wrapper = mount(AgentAdminPromoCodesView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载优惠码管理')
    expect(wrapper.text()).not.toContain('internal funding endpoint')
    expect(reader).not.toHaveBeenCalled()

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(reader).toHaveBeenCalledOnce()
  })
})
