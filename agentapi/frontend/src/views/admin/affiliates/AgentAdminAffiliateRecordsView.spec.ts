import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminAffiliateRecordsView from './AgentAdminAffiliateRecordsView.vue'
import { agentAPI } from '@/agent/api'

function createContext() {
  return {
    agent: {
      agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active',
      billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
    },
    authenticated: true,
    is_agent_admin: true,
  }
}

const global = {
  stubs: {
    RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
    AgentAffiliateRecordsPanel: { props: ['type'], template: '<div data-testid="affiliate-panel">{{ type }}</div>' },
  },
}

describe('AgentAdminAffiliateRecordsView', () => {
  afterEach(() => vi.restoreAllMocks())

  it.each([
    ['invites', '推广邀请记录'],
    ['rebates', '推广返利记录'],
    ['transfers', '推广划转记录'],
  ] as const)('renders the independent %s page after tenant context succeeds', async (type, title) => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const wrapper = mount(AgentAdminAffiliateRecordsView, { props: { type }, global })

    expect(wrapper.find('[data-testid="affiliate-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(wrapper.text()).toContain(title)
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.get('[data-testid="affiliate-panel"]').text()).toBe(type)
    expect(wrapper.findAll('a').map(link => link.attributes('href'))).toEqual([
      '/admin/affiliates/invites', '/admin/affiliates/rebates', '/admin/affiliates/transfers',
    ])
    expect(wrapper.text()).toContain('不能赠送返利、修改比例、重置额度')
  })

  it('does not mount the record reader until tenant context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(createContext())
    const wrapper = mount(AgentAdminAffiliateRecordsView, { props: { type: 'invites' }, global })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载推广邀请记录')
    expect(wrapper.find('[data-testid="affiliate-panel"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="affiliate-panel"]').text()).toBe('invites')
  })
})
