import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAPIKeysView from './AgentAPIKeysView.vue'
import { agentAPI } from '@/agent/api'
import { resetAgentToastsForTests, useAgentToast } from '@/composables/useAgentToast'

describe('AgentAPIKeysView', () => {
  it('combines name/prefix search and status filters on tenant keys', async () => {
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({ total: 3, items: [
      { id: 1, name: 'Desktop', prefix: 'sk-a-one', key: 'sk-a-one-secret', status: 'active', created_at: '' },
      { id: 2, name: 'Desktop old', prefix: 'sk-a-two', key: 'sk-a-two-secret', status: 'revoked', created_at: '' },
      { id: 3, name: 'Server', prefix: 'sk-a-three', key: 'sk-a-three-secret', status: 'active', created_at: '' },
    ] })
    const wrapper = mount(AgentAPIKeysView)
    await flushPromises()
    await wrapper.get('[aria-label="搜索密钥"]').setValue('desktop')
    await wrapper.get('[aria-label="密钥状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('启用中'))!.click()
    await flushPromises()
    expect(wrapper.text()).toContain('Desktop')
    expect(wrapper.text()).not.toContain('Desktop old')
    expect(wrapper.text()).not.toContain('Server')
    expect(wrapper.text()).toContain('显示 1 / 3 个密钥')
    await wrapper.get('[aria-label="搜索密钥"]').setValue('sk-a-three')
    expect(wrapper.text()).toContain('Server')
    expect(wrapper.text()).not.toContain('Desktop')
    expect(wrapper.text()).not.toContain('历史密钥不可恢复')
    wrapper.unmount()
  })

  it('retains the modal and input after creation fails without displaying a secret', async () => {
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI.keys, 'create').mockRejectedValue(new Error('unavailable'))
    const wrapper = mount(AgentAPIKeysView, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('[data-testid="open-create-key"]').trigger('click')
    await wrapper.get('#key-name').setValue('desktop')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect((wrapper.get('#key-name').element as HTMLInputElement).value).toBe('desktop')
    expect(wrapper.get('[role="dialog"]').text()).toContain('创建 API 密钥失败')
    expect(wrapper.text()).not.toContain('新密钥 — 请立即复制')
    wrapper.unmount()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    resetAgentToastsForTests()
  })

  it('loads local keys and displays a newly created key once', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({
      total: 1,
      api_base_url: 'https://agent.example.com/v1',
      items: [{ id: 1, name: 'existing', prefix: 'sk-old', key: 'sk-existing-secret', status: 'active', created_at: '2026-09-22T00:00:00Z' }],
    })
    const create = vi.spyOn(agentAPI.keys, 'create').mockResolvedValue({
      item: { id: 2, name: 'desktop', prefix: 'sk-new', status: 'active', created_at: '2026-09-22T00:00:00Z' },
      key: 'sk-new-secret',
    })
    const wrapper = mount(AgentAPIKeysView, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    await wrapper.get('[data-testid="open-create-key"]').trigger('click')
    await wrapper.get('input[placeholder="密钥名称，例如：桌面客户端"]').setValue('desktop')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith('desktop')
    expect(wrapper.text()).toContain('sk-new-secret')
    expect(wrapper.text()).not.toContain('sk-super-')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(useAgentToast().toasts.value.at(-1)?.message).toContain('API 密钥已创建')
    await wrapper.get('[data-testid="copy-key-2"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('sk-new-secret')
    expect(wrapper.get('[data-testid="copy-key-2"]').attributes('aria-label')).toBe('密钥已复制')
    await wrapper.get('[data-testid="copy-api-base-url"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('https://agent.example.com/v1')
    wrapper.unmount()
  })

  it('uses a Chinese confirmation dialog before revoking a key', async () => {
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({
      total: 1,
      items: [{ id: 1, name: 'desktop', prefix: 'sk-old', key: 'sk-old-secret', status: 'active', created_at: '2026-09-22T00:00:00Z' }],
    })
    const revoke = vi.spyOn(agentAPI.keys, 'revoke').mockResolvedValue(undefined)
    const wrapper = mount(AgentAPIKeysView)
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '撤销')!.trigger('click')
    await flushPromises()

    expect(revoke).not.toHaveBeenCalled()
    expect(document.body.querySelector('[role="alertdialog"]')?.textContent).toContain('确定撤销“desktop”吗？')
    document.body.querySelector<HTMLButtonElement>('[data-testid="agent-confirm-action"]')?.click()
    await flushPromises()

    expect(revoke).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('已撤销')
    expect(useAgentToast().toasts.value.at(-1)?.message).toBe('API 密钥已撤销。')
    wrapper.unmount()
  })
})
