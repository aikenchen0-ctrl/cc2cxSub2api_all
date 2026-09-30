import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentAdminSubscription } from '@/agent/api'
import AgentAdminSubscriptionsPanel from './AgentAdminSubscriptionsPanel.vue'

const subscription: AgentAdminSubscription = {
  id: 20,
  group_id: 4,
  starts_at: '2026-09-20T00:00:00Z',
  expires_at: '2026-10-20T00:00:00Z',
  status: 'active',
  daily_window_start: '2026-09-29T00:00:00Z',
  weekly_window_start: '2026-09-23T00:00:00Z',
  monthly_window_start: '2026-09-01T00:00:00Z',
  daily_usage_usd: 2,
  weekly_usage_usd: 4,
  monthly_usage_usd: 9,
  main_user_id: '84',
  user_email: 'member@example.com',
  user_display_name: 'Member',
  group: {
    id: 4,
    name: 'Gemini Pro',
    description: '主站订阅分组',
    platform: 'gemini',
    rate_multiplier: 1,
    subscription_type: 'subscription',
    daily_limit_usd: 10,
    weekly_limit_usd: 50,
    monthly_limit_usd: 100,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
  },
}

afterEach(() => vi.restoreAllMocks())

describe('AgentAdminSubscriptionsPanel', () => {
  it('shows tenant-enriched main-site subscription and quota facts', async () => {
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: [{ agent_id: 'agent-1', main_user_id: '84', email: 'member@example.com', display_name: 'Member', status: 'active', balance_cents: 0, created_at: '', updated_at: '' }], total: 1, page: 1, page_size: 500 })
    const list = vi.spyOn(agentAPI.adminSubscriptions, 'list').mockResolvedValue({ items: [subscription], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = mount(AgentAdminSubscriptionsPanel)
    await flushPromises()

    expect(list).toHaveBeenCalledWith(1, 20, {})
    expect(wrapper.text()).toContain('Member')
    expect(wrapper.text()).toContain('Gemini Pro')
    expect(wrapper.get('[data-group-id="4"]').text()).toContain('Gemini Pro')
    expect(wrapper.get('[data-group-id="4"]').attributes('data-platform')).toBe('gemini')
    expect(wrapper.text()).toContain('$2.00/$10.00')
    expect(wrapper.text()).toContain('主站订阅和额度事实')
    expect(wrapper.get('[data-status="active"]').text()).toBe('生效中')
    expect(wrapper.findAll('button').some(button => button.text().includes('分配订阅'))).toBe(false)
    expect(wrapper.text()).not.toContain('upstream_secret')
    wrapper.unmount()
  })

  it('applies mapped-user, status, group and platform filters on the server', async () => {
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: [{ agent_id: 'agent-1', main_user_id: '84', email: 'member@example.com', display_name: 'Member', status: 'active', balance_cents: 0, created_at: '', updated_at: '' }], total: 1, page: 1, page_size: 500 })
    const list = vi.spyOn(agentAPI.adminSubscriptions, 'list').mockResolvedValue({ items: [subscription], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = mount(AgentAdminSubscriptionsPanel)
    await flushPromises()

    await wrapper.get('[aria-label="筛选订阅用户"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('Member'))!.click()
    await flushPromises()
    await wrapper.get('[aria-label="筛选订阅状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('生效中'))!.click()
    await flushPromises()
    await wrapper.get('[aria-label="筛选订阅分组"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('Gemini Pro'))!.click()
    await flushPromises()
    await wrapper.get('[aria-label="筛选订阅平台"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.trim() === 'gemini')!.click()
    await flushPromises()

    expect(list).toHaveBeenLastCalledWith(1, 20, {
      status: 'active', main_user_id: '84', group_id: 4, platform: 'gemini',
    })
    wrapper.unmount()
  })

  it('opens a main-style read-only subscription detail dialog', async () => {
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 500 })
    vi.spyOn(agentAPI.adminSubscriptions, 'list').mockResolvedValue({ items: [subscription], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = mount(AgentAdminSubscriptionsPanel)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '详情')!.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('订阅详情')
    expect(document.body.textContent).toContain('周期额度')
    expect(document.body.textContent).toContain('member@example.com')
    expect(document.body.querySelector('[data-group-id="4"]')?.textContent).toContain('Gemini Pro')
    expect(document.body.textContent).not.toContain('延长订阅')
    wrapper.unmount()
  })
})
