import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminContentView from './AgentAdminContentView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Agent', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

describe('AgentAdminContentView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before loading the tenant content panel', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const list = vi.spyOn(agentAPI.adminContentPages, 'list').mockResolvedValue({ items: [], total: 0 })
    const wrapper = mount(AgentAdminContentView)

    expect(list).not.toHaveBeenCalled()
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('内容页面')
    expect(wrapper.text()).toContain('公开协议')
    expect(wrapper.text()).toContain('自定义页面')
    expect(wrapper.text()).toContain('不能读取或修改其他代理站内容')
  })

  it('keeps content readers hidden until context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const list = vi.spyOn(agentAPI.adminContentPages, 'list').mockResolvedValue({ items: [], total: 0 })
    const wrapper = mount(AgentAdminContentView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载内容页面')
    expect(list).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('尚未创建内容页面')
  })
})
