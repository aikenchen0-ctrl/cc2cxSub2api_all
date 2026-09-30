import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentBalanceNotifySettings } from '@/agent/api'
import AgentProfileBalanceNotifyCard from './AgentProfileBalanceNotifyCard.vue'

enableAutoUnmount(afterEach)

const initial: AgentBalanceNotifySettings = {
  feature_enabled: true,
  system_default_threshold: 5,
  enabled: true,
  threshold: 2.5,
  extra_emails: [],
}

describe('AgentProfileBalanceNotifyCard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(agentAPI.profile.balanceNotify, 'get').mockResolvedValue(initial)
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('copies the main-site switch, threshold and verified email flow', async () => {
    const update = vi.spyOn(agentAPI.profile.balanceNotify, 'update').mockImplementation(async payload => ({
      ...initial,
      enabled: payload.enabled ?? initial.enabled,
      threshold: payload.threshold ?? initial.threshold,
    }))
    const sendCode = vi.spyOn(agentAPI.profile.balanceNotify, 'sendCode').mockResolvedValue({ success: true })
    const verify = vi.spyOn(agentAPI.profile.balanceNotify, 'verify').mockResolvedValue({ success: true })
    vi.spyOn(agentAPI.profile.balanceNotify, 'get')
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce({
        ...initial,
        extra_emails: [{ email: 'alerts@example.com', disabled: false, verified: true }],
      })
    const wrapper = mount(AgentProfileBalanceNotifyCard)
    await flushPromises()

    const enabled = wrapper.get('#balance-notify-enabled')
    await enabled.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith({ enabled: false })
    await enabled.trigger('click')
    await flushPromises()

    await wrapper.get('#balance-notify-threshold').setValue('7.25')
    const save = wrapper.findAll('button').find(button => button.text() === '保存')!
    await save.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith({ threshold: 7.25 })

    await wrapper.get('input[type="email"]').setValue('Alerts@Example.com')
    const add = wrapper.findAll('button').find(button => button.text() === '添加并验证')!
    await add.trigger('click')
    await flushPromises()
    expect(sendCode).toHaveBeenCalledWith('alerts@example.com')
    expect(wrapper.get('[data-testid="balance-notify-verification"]').text()).toContain('alerts@example.com')

    await wrapper.get('[data-testid="balance-notify-verification"] input').setValue('123456')
    const verifyButton = wrapper.get('[data-testid="balance-notify-verification"]').findAll('button').find(button => button.text() === '验证')!
    await verifyButton.trigger('click')
    await flushPromises()
    expect(verify).toHaveBeenCalledWith('alerts@example.com', '123456')
    expect(wrapper.text()).toContain('已验证')
  })

  it('toggles and removes only the selected saved notification email', async () => {
    const configured = {
      ...initial,
      extra_emails: [
        { email: 'first@example.com', disabled: false, verified: true },
        { email: 'second@example.com', disabled: false, verified: true },
      ],
    }
    vi.spyOn(agentAPI.profile.balanceNotify, 'get')
      .mockResolvedValueOnce(configured)
      .mockResolvedValueOnce({ ...configured, extra_emails: [configured.extra_emails[1]!] })
    const toggle = vi.spyOn(agentAPI.profile.balanceNotify, 'toggle').mockResolvedValue({
      ...configured,
      extra_emails: [
        { email: 'first@example.com', disabled: true, verified: true },
        configured.extra_emails[1]!,
      ],
    })
    const remove = vi.spyOn(agentAPI.profile.balanceNotify, 'remove').mockResolvedValue({ success: true })
    const wrapper = mount(AgentProfileBalanceNotifyCard)
    await flushPromises()

    await wrapper.get('input[aria-label="启用 first@example.com"]').setValue(false)
    await flushPromises()
    expect(toggle).toHaveBeenCalledWith('first@example.com', true)
    const firstRow = wrapper.findAll('.rounded-lg.bg-gray-50').find(row => row.text().includes('first@example.com'))!
    await firstRow.findAll('button').find(button => button.text() === '移除')!.trigger('click')
    await flushPromises()
    expect(remove).toHaveBeenCalledWith('first@example.com')
    expect(wrapper.text()).not.toContain('first@example.com')
    expect(wrapper.text()).toContain('second@example.com')
  })

  it('stays hidden when the main-site feature is disabled and exposes retry on errors', async () => {
    vi.spyOn(agentAPI.profile.balanceNotify, 'get').mockResolvedValueOnce({ ...initial, feature_enabled: false })
    const wrapper = mount(AgentProfileBalanceNotifyCard)
    await flushPromises()
    expect(wrapper.find('[data-testid="profile-balance-notify-card"]').exists()).toBe(false)

    vi.restoreAllMocks()
    vi.spyOn(agentAPI.profile.balanceNotify, 'get').mockRejectedValue(new Error('offline'))
    const failed = mount(AgentProfileBalanceNotifyCard)
    await flushPromises()
    expect(failed.get('[role="alert"]').text()).toContain('加载余额通知设置失败')
    expect(failed.text()).toContain('重新加载')
  })
})
