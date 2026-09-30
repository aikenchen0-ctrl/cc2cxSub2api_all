import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentUsersPanel from './AgentUsersPanel.vue'
import { agentAPI } from '@/agent/api'

const users = [
  { agent_id: 'agent-1', main_user_id: 'owner-1', email: 'owner@example.com', status: 'active', balance_cents: 1000, created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z' },
  { agent_id: 'agent-1', main_user_id: 'user-1', email: 'user@example.com', status: 'active', balance_cents: 500, created_at: '2026-09-21T00:00:00Z', updated_at: '2026-09-21T00:00:00Z' },
]

describe('AgentUsersPanel', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('uses the server-side local status filter and exposes main-style column controls', async () => {
    const getUsers = vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: users, total: 2, page: 1, page_size: 25 })
    const wrapper = mount(AgentUsersPanel, { props: { ownerMainUserId: 'owner-1' } })
    await flushPromises()

    expect(wrapper.text()).toContain('列设置')
    expect(wrapper.findAll('[data-status="active"]')).toHaveLength(2)
    await wrapper.get('[aria-label="筛选用户状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('已停用'))!.click()
    await flushPromises()
    expect(getUsers).toHaveBeenLastCalledWith(1, 25, '', 'disabled')

    await wrapper.findAll('button').find(button => button.text().includes('列设置'))!.trigger('click')
    await wrapper.findAll('button').find(button => button.text().includes('更新时间'))!.trigger('click')
    expect(wrapper.text()).not.toContain('更新时间操作')
  })

  it('debounces and trims server-side user searches through the shared search input', async () => {
    vi.useFakeTimers()
    const getUsers = vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: users, total: 2, page: 1, page_size: 25 })
    const wrapper = mount(AgentUsersPanel, { props: { ownerMainUserId: 'owner-1' } })
    await flushPromises()

    await wrapper.get('[aria-label="搜索关联用户"]').setValue(' user@example.com ')
    expect(getUsers).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(getUsers).toHaveBeenLastCalledWith(1, 25, 'user@example.com')
    wrapper.unmount()
  })

  it('never includes the station owner in bulk status updates', async () => {
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: users, total: 2, page: 1, page_size: 25 })
    const update = vi.spyOn(agentAPI, 'setMappedUserStatus').mockResolvedValue({ ...users[1], status: 'disabled' })
    const wrapper = mount(AgentUsersPanel, { props: { ownerMainUserId: 'owner-1' } })
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await flushPromises()
    expect(wrapper.text()).toContain('批量停用')
    await wrapper.findAll('button').find(button => button.text().includes('批量停用'))!.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('[data-testid="agent-confirm-action"]')?.click()
    await flushPromises()

    expect(update).toHaveBeenCalledTimes(1)
    expect(update).toHaveBeenCalledWith('user-1', 'disabled')
    expect(update).not.toHaveBeenCalledWith('owner-1', expect.anything())
    wrapper.unmount()
  })
})
