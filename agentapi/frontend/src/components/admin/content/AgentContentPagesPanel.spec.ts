import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentContentPagesPanel from './AgentContentPagesPanel.vue'

describe('AgentContentPagesPanel', () => {
  afterEach(() => vi.restoreAllMocks())

  it('creates a tenant page through the isolated AgentAPI admin endpoint', async () => {
    vi.spyOn(agentAPI.adminContentPages, 'list').mockResolvedValue({ items: [], total: 0 })
    const create = vi.spyOn(agentAPI.adminContentPages, 'create').mockResolvedValue({
      id: 1, slug: 'guide', kind: 'custom', title: '本站指南', content: '# 指南',
      status: 'active', sort_order: 3, created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
    })
    const wrapper = mount(AgentContentPagesPanel)
    await flushPromises()

    await wrapper.get('[aria-label="选择页面类型"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('自定义页面'))!.click()
    await flushPromises()
    await wrapper.get('[aria-label="选择发布状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('已发布'))!.click()
    await flushPromises()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('guide')
    await inputs[1].setValue('3')
    await inputs[2].setValue('本站指南')
    await wrapper.get('textarea').setValue('# 指南')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith({
      slug: 'guide', kind: 'custom', title: '本站指南', content: '# 指南', status: 'active', sort_order: 3,
    })
  })

  it('shows legal and custom routes without a main-site settings control', async () => {
    vi.spyOn(agentAPI.adminContentPages, 'list').mockResolvedValue({ items: [{
      id: 2, slug: 'privacy', kind: 'legal', title: '隐私政策', content: '# 隐私',
      status: 'active', sort_order: 0, created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
    }], total: 1 })
    const wrapper = mount(AgentContentPagesPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('/legal/privacy')
    expect(wrapper.text()).toContain('只存放在当前代理站')
    expect(wrapper.text()).not.toContain('主站全局设置')
  })

  it('deletes a tenant page through the shared confirmation dialog', async () => {
    const page = {
      id: 3, slug: 'guide', kind: 'custom' as const, title: '本站指南', content: '# 指南',
      status: 'active' as const, sort_order: 0, created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
    }
    vi.spyOn(agentAPI.adminContentPages, 'list')
      .mockResolvedValueOnce({ items: [page], total: 1 })
      .mockResolvedValueOnce({ items: [], total: 0 })
    const remove = vi.spyOn(agentAPI.adminContentPages, 'delete').mockResolvedValue()
    const wrapper = mount(AgentContentPagesPanel)
    await flushPromises()

    await wrapper.get('button.btn-danger').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('本站指南')
    expect(remove).not.toHaveBeenCalled()

    ;(dialog?.querySelector('[data-testid="agent-confirm-action"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(remove).toHaveBeenCalledWith(3)
    // BaseDialog follows the main-site leave transition before removing the teleported node.
    await new Promise(resolve => window.setTimeout(resolve, 250))
    expect(document.body.querySelector('[role="alertdialog"]')).toBeNull()
    wrapper.unmount()
  })
})
