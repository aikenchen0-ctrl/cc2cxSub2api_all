import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentPasskeyAPI } from '@/agent/passkeys'
import AgentProfilePasskeyCard from './AgentProfilePasskeyCard.vue'

describe('AgentProfilePasskeyCard', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('lists, renames, and deletes only the current user passkeys', async () => {
    vi.spyOn(agentPasskeyAPI, 'isSupported').mockReturnValue(true)
    vi.spyOn(agentPasskeyAPI, 'list').mockResolvedValue([{
      id: 7,
      name: '办公电脑',
      created_at: '2026-09-29T08:00:00Z',
      last_used_at: '2026-09-29T09:00:00Z',
      backup: true,
    }])
    const rename = vi.spyOn(agentPasskeyAPI, 'rename').mockResolvedValue(undefined)
    const remove = vi.spyOn(agentPasskeyAPI, 'remove').mockResolvedValue(undefined)
    const wrapper = mount(AgentProfilePasskeyCard, { props: { enabled: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('办公电脑')
    expect(wrapper.text()).toContain('已同步')

    await wrapper.get('button.btn-secondary.btn-sm').trigger('click')
    await flushPromises()
    const renameInput = document.body.querySelector<HTMLInputElement>('#agent-passkey-rename-name')
    expect(renameInput?.value).toBe('办公电脑')
    renameInput!.value = '安全密钥'
    renameInput!.dispatchEvent(new Event('input', { bubbles: true }))
    ;(document.body.querySelector('[form="agent-passkey-rename-form"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(rename).toHaveBeenCalledWith(7, '安全密钥')
    expect(wrapper.text()).toContain('安全密钥')

    const deleteButton = wrapper.findAll('button').find((button) => button.text() === '删除')
    expect(deleteButton).toBeTruthy()
    await deleteButton!.trigger('click')
    await flushPromises()
    const deleteInput = document.body.querySelector<HTMLInputElement>('#agent-passkey-delete-password')
    expect(deleteInput).toBeTruthy()
    deleteInput!.value = 'current-password'
    deleteInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    ;(document.body.querySelector('[form="agent-passkey-delete-form"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(remove).toHaveBeenCalledWith(7, 'current-password')
    expect(wrapper.text()).toContain('Passkey 已删除')
    expect(wrapper.findAll('strong').some((item) => item.text() === '安全密钥')).toBe(false)
    wrapper.unmount()
  })

  it('does not query credentials when the main RP origin is unavailable', async () => {
    vi.spyOn(agentPasskeyAPI, 'isSupported').mockReturnValue(true)
    const list = vi.spyOn(agentPasskeyAPI, 'list')
    const wrapper = mount(AgentProfilePasskeyCard, { props: { enabled: false } })
    await flushPromises()
    expect(wrapper.text()).toContain('RP Origin')
    expect(list).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
