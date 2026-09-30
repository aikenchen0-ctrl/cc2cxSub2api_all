import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import QRCode from 'qrcode'
import { agentAPI } from '@/agent/api'
import AgentProfileTotpCard from './AgentProfileTotpCard.vue'

vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn().mockResolvedValue('data:image/png;base64,totp') } }))

describe('AgentProfileTotpCard', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('copies the main-site password verification setup flow and refreshes status', async () => {
    const status = vi.spyOn(agentAPI.profile.totp, 'getStatus')
      .mockResolvedValueOnce({ enabled: false, feature_enabled: true })
      .mockResolvedValueOnce({ enabled: true, enabled_at: 1_800_000_000, feature_enabled: true })
    vi.spyOn(agentAPI.profile.totp, 'getVerificationMethod').mockResolvedValue({ method: 'password' })
    const initiate = vi.spyOn(agentAPI.profile.totp, 'initiateSetup').mockResolvedValue({
      secret: 'JBSWY3DPEHPK3PXP',
      qr_code_url: 'otpauth://totp/Agent:user@example.com?secret=JBSWY3DPEHPK3PXP',
      setup_token: 'one-time-token',
      countdown: 300,
    })
    const enable = vi.spyOn(agentAPI.profile.totp, 'enable').mockResolvedValue({ success: true })
    const wrapper = mount(AgentProfileTotpCard)
    await flushPromises()

    expect(wrapper.text()).toContain('尚未启用两步验证')
    await wrapper.get('[data-testid="totp-enable"]').trigger('click')
    await flushPromises()
    const passwordInput = document.body.querySelector<HTMLInputElement>('#totp-password')
    expect(passwordInput).toBeTruthy()
    passwordInput!.value = 'current-password'
    passwordInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    ;(document.body.querySelector('[data-testid="totp-setup-modal"] button.btn-primary') as HTMLButtonElement).click()
    await flushPromises()

    expect(initiate).toHaveBeenCalledWith({ password: 'current-password' })
    expect(QRCode.toDataURL).toHaveBeenCalledWith(expect.stringMatching(/^otpauth:\/\/totp\//), expect.any(Object))
    const modal = document.body.querySelector<HTMLElement>('[data-testid="totp-setup-modal"]')
    expect(modal?.textContent).toContain('JBSWY3DPEHPK3PXP')
    ;([...modal!.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === '下一步') as HTMLButtonElement).click()
    await flushPromises()
    const authCode = document.body.querySelector<HTMLInputElement>('#totp-auth-code')
    expect(authCode).toBeTruthy()
    authCode!.value = '123456'
    authCode!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.body.querySelector<HTMLFormElement>('[data-testid="totp-setup-modal"] form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(enable).toHaveBeenCalledWith('123456', 'one-time-token')
    expect(status).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('两步验证已启用')
    await new Promise(resolve => window.setTimeout(resolve, 220))
    expect(document.body.querySelector('[data-testid="totp-setup-modal"]')).toBeNull()
  })

  it('supports the main-site email verification disable flow', async () => {
    vi.spyOn(agentAPI.profile.totp, 'getStatus')
      .mockResolvedValueOnce({ enabled: true, enabled_at: 1_800_000_000, feature_enabled: true })
      .mockResolvedValueOnce({ enabled: false, feature_enabled: true })
    vi.spyOn(agentAPI.profile.totp, 'getVerificationMethod').mockResolvedValue({ method: 'email' })
    const sendCode = vi.spyOn(agentAPI.profile.totp, 'sendVerifyCode').mockResolvedValue({ success: true })
    const disable = vi.spyOn(agentAPI.profile.totp, 'disable').mockResolvedValue({ success: true })
    const wrapper = mount(AgentProfileTotpCard)
    await flushPromises()

    await wrapper.get('[data-testid="totp-disable"]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector<HTMLElement>('[data-testid="totp-disable-dialog"]')
    expect(dialog).toBeTruthy()
    const send = [...dialog!.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === '发送验证码')!
    send.click()
    await flushPromises()
    expect(sendCode).toHaveBeenCalledTimes(1)
    const emailCode = document.body.querySelector<HTMLInputElement>('#totp-disable-email-code')
    expect(emailCode).toBeTruthy()
    emailCode!.value = '654321'
    emailCode!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.body.querySelector<HTMLFormElement>('[data-testid="totp-disable-dialog"] form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(disable).toHaveBeenCalledWith({ email_code: '654321' })
    expect(wrapper.text()).toContain('尚未启用两步验证')
    expect(wrapper.text()).toContain('两步验证已停用')
  })

  it('shows a retry state when status cannot be loaded', async () => {
    vi.spyOn(agentAPI.profile.totp, 'getStatus').mockRejectedValue(new Error('offline'))
    const wrapper = mount(AgentProfileTotpCard)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('加载两步验证状态失败')
    expect(wrapper.text()).toContain('重新加载')
  })
})
