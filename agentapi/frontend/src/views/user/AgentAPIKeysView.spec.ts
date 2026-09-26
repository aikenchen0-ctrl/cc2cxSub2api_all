import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAPIKeysView from './AgentAPIKeysView.vue'
import { agentAPI } from '@/agent/api'

describe('AgentAPIKeysView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads local keys and displays a newly created key once', async () => {
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({
      total: 1,
      items: [{ id: 1, name: 'existing', prefix: 'sk-agent-old', status: 'active', created_at: '2026-09-22T00:00:00Z' }],
    })
    const create = vi.spyOn(agentAPI.keys, 'create').mockResolvedValue({
      item: { id: 2, name: 'desktop', prefix: 'sk-agent-new', status: 'active', created_at: '2026-09-22T00:00:00Z' },
      key: 'sk-agent-new-secret',
    })
    const wrapper = mount(AgentAPIKeysView)
    await flushPromises()

    await wrapper.get('input[placeholder="密钥名称，例如：桌面客户端"]').setValue('desktop')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith('desktop')
    expect(wrapper.text()).toContain('sk-agent-new-secret')
    expect(wrapper.text()).not.toContain('sk-super-')
  })

  it('uses a Chinese confirmation dialog before revoking a key', async () => {
    vi.spyOn(agentAPI.keys, 'list').mockResolvedValue({
      total: 1,
      items: [{ id: 1, name: 'desktop', prefix: 'sk-agent-old', status: 'active', created_at: '2026-09-22T00:00:00Z' }],
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
    wrapper.unmount()
  })
})
