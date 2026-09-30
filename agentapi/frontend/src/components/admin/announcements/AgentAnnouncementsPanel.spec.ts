import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentAnnouncementsPanel from './AgentAnnouncementsPanel.vue'

const { list, create, update, remove } = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/agent/api', () => ({
  agentAPI: { adminAnnouncements: { list, create, update, delete: remove } },
}))

describe('AgentAnnouncementsPanel', () => {
  const mountPanel = () => mount(AgentAnnouncementsPanel, {
    global: { stubs: { Teleport: true } },
  })

  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('scrollTo', vi.fn())
    list.mockResolvedValue({ items: [], total: 0 })
    create.mockResolvedValue({ id: 1 })
    update.mockResolvedValue({ id: 1 })
    remove.mockResolvedValue(undefined)
  })

  it('creates a tenant-local announcement with main-compatible status and notify mode', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-test="create-announcement"]').trigger('click')
    await wrapper.get('input[placeholder="公告标题"]').setValue('维护通知')
    await wrapper.get('textarea').setValue('今晚维护')
    await wrapper.get('[aria-label="公告编辑状态"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('[role="option"]').find(option => option.text().includes('已发布'))!.trigger('click')
    await flushPromises()
    await wrapper.get('[aria-label="公告提醒方式"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('[role="option"]').find(option => option.text().includes('首次进入弹窗'))!.trigger('click')
    await wrapper.get('#agent-announcement-form').trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ title: '维护通知', content: '今晚维护', status: 'active', notify_mode: 'popup' }))
    expect(wrapper.text()).toContain('公告已创建')
    expect(wrapper.text()).toContain('不会修改 Sub2API 主站公告')
  })

  it('edits and deletes only the selected announcement', async () => {
    list.mockResolvedValue({ items: [{ id: 7, title: '旧标题', content: '旧正文', status: 'draft', notify_mode: 'silent', created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z' }], total: 1 })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-status="draft"]').text()).toBe('草稿')
    await wrapper.get('button[title="预览"]').trigger('click')
    expect(wrapper.findAll('[data-status="draft"]')).toHaveLength(2)
    await wrapper.get('button[aria-label="关闭对话框"]').trigger('click')
    await wrapper.get('button[title="编辑"]').trigger('click')
    await wrapper.get('input[placeholder="公告标题"]').setValue('新标题')
    await wrapper.get('#agent-announcement-form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ title: '新标题' }))
    list.mockResolvedValue({ items: [{ id: 7, title: '旧标题', content: '旧正文', status: 'draft', notify_mode: 'silent', created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z' }], total: 1 })
    await wrapper.get('button[title="刷新"]').trigger('click')
    await flushPromises()
    await wrapper.get('button[title="删除"]').trigger('click')
    await wrapper.get('[data-testid="agent-confirm-action"]').trigger('click')
    await flushPromises()
    expect(remove).toHaveBeenCalledWith(7)
  })

  it('filters the main-style table by status and text without querying another tenant', async () => {
    list.mockResolvedValue({ items: [
      { id: 1, title: '维护通知', content: '今晚维护', status: 'active', notify_mode: 'popup', created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z' },
      { id: 2, title: '内部草稿', content: '待确认', status: 'draft', notify_mode: 'silent', created_at: '2026-09-28T00:00:00Z', updated_at: '2026-09-28T00:00:00Z' },
    ], total: 2 })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[aria-label="公告状态"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('[role="option"]').find(option => option.text().includes('已发布'))!.trigger('click')
    expect(wrapper.get('[data-status="active"]').text()).toBe('已发布')
    expect(wrapper.text()).toContain('维护通知')
    expect(wrapper.text()).not.toContain('内部草稿')
    await wrapper.get('input[placeholder="搜索公告标题或正文"]').setValue('不存在')
    expect(wrapper.text()).toContain('暂无公告')
    expect(list).toHaveBeenCalledTimes(1)
  })
})
