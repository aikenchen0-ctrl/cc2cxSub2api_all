import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentUsersView from './AgentUsersView.vue'

const { getContext } = vi.hoisted(() => ({ getContext: vi.fn() }))

vi.mock('@/agent/api', () => ({ agentAPI: { getContext } }))

const panel = {
  props: ['ownerMainUserId'],
  template: '<div data-test="users-panel">owner={{ ownerMainUserId }}</div>',
}

describe('AgentUsersView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getContext.mockResolvedValue({ agent: { name: 'Agent One', site_name: '代理一站', owner_main_user_id: 'owner-42' } })
  })

  it('loads the tenant context before exposing user management and protects the owner', async () => {
    const wrapper = mount(AgentUsersView, { global: { stubs: { AgentUsersPanel: panel } } })
    expect(wrapper.text()).toContain('正在确认代理站管理边界')
    expect(wrapper.find('[data-test="users-panel"]').exists()).toBe(false)
    await flushPromises()
    expect(wrapper.text()).toContain('用户管理')
    expect(wrapper.text()).toContain('代理一站')
    expect(wrapper.get('[data-test="users-panel"]').text()).toContain('owner=owner-42')
  })

  it('does not render the management panel when the tenant boundary cannot be loaded', async () => {
    getContext.mockRejectedValue(new Error('offline'))
    const wrapper = mount(AgentUsersView, { global: { stubs: { AgentUsersPanel: panel } } })
    await flushPromises()
    expect(wrapper.text()).toContain('无法加载用户管理')
    expect(wrapper.find('[data-test="users-panel"]').exists()).toBe(false)
    getContext.mockResolvedValue({ agent: { name: 'Agent One', site_name: '代理一站', owner_main_user_id: 'owner-42' } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="users-panel"]').exists()).toBe(true)
  })
})
