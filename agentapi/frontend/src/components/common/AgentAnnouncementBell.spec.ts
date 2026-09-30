import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AgentAnnouncementBell from './AgentAnnouncementBell.vue'

const { list, markRead, markAllRead } = vi.hoisted(() => ({
  list: vi.fn(),
  markRead: vi.fn(),
  markAllRead: vi.fn(),
}))

vi.mock('@/agent/api', () => ({
  agentAPI: { announcements: { list, markRead, markAllRead } },
}))

describe('AgentAnnouncementBell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    list.mockResolvedValue({
      items: [{ id: 1, title: '维护通知', content: '<img src=x onerror=alert(1)>\n今晚维护', status: 'active', notify_mode: 'silent', created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z' }],
      total: 1,
      unread: 1,
    })
    markRead.mockResolvedValue({ id: 1, read: true })
    markAllRead.mockResolvedValue({ read: true })
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('shows tenant announcements and renders content as text, not HTML', async () => {
    const wrapper = mount(AgentAnnouncementBell, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.find('[aria-label="站内公告"] span.bg-red-500').exists()).toBe(true)
    await wrapper.get('[aria-label="站内公告"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('维护通知')
    const item = Array.from(document.body.querySelectorAll('button')).find(node => node.textContent?.includes('维护通知')) as HTMLButtonElement
    item.click()
    await flushPromises()
    expect(markRead).toHaveBeenCalledWith(1)
    expect(document.body.textContent).toContain('<img src=x onerror=alert(1)>')
    expect(document.body.querySelector('img[src="x"]')).toBeNull()
    wrapper.unmount()
  })

  it('marks all announcements as read through the same-origin API', async () => {
    const wrapper = mount(AgentAnnouncementBell, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('[aria-label="站内公告"]').trigger('click')
    await flushPromises()
    const button = Array.from(document.body.querySelectorAll('button')).find(node => node.textContent?.includes('全部已读')) as HTMLButtonElement
    button.click()
    await flushPromises()
    expect(markAllRead).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('已全部阅读')
    wrapper.unmount()
  })

  it('acknowledges a popup announcement through the shared detail dialog', async () => {
    list.mockResolvedValue({
      items: [{ id: 2, title: '重要公告', content: '请确认已阅读', status: 'active', notify_mode: 'popup', created_at: '2026-09-29T01:00:00Z', updated_at: '2026-09-29T01:00:00Z' }],
      total: 1,
      unread: 1,
    })

    const wrapper = mount(AgentAnnouncementBell, { attachTo: document.body })
    await flushPromises()

    const dialog = document.body.querySelector<HTMLElement>('[role="dialog"]')
    expect(dialog?.textContent).toContain('重要公告')
    expect(dialog?.textContent).toContain('请确认已阅读')
    const acknowledge = Array.from(dialog!.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent === '我知道了')
    acknowledge!.click()
    await flushPromises()

    expect(markRead).toHaveBeenCalledWith(2)
    expect(sessionStorage.getItem('agentapi_announcement_seen_2')).toBe('1')
    wrapper.unmount()
  })
})
