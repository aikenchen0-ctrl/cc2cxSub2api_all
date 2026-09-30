import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type PasswordRecoveryConfig } from '@/agent/api'
import AgentForgotPasswordView from './AgentForgotPasswordView.vue'

const enabled: PasswordRecoveryConfig = {
  password_reset_enabled: true,
  turnstile_enabled: false,
  turnstile_site_key: '',
  tencent_captcha_enabled: false,
  tencent_captcha_app_id: '',
  tencent_captcha_region: 'cn',
  aliyun_captcha_enabled: false,
  aliyun_captcha_scene_id: '',
  aliyun_captcha_prefix: '',
  aliyun_captcha_region: 'cn',
}

async function setup(config: PasswordRecoveryConfig = enabled) {
  vi.spyOn(agentAPI.auth, 'passwordRecoveryConfig').mockResolvedValue(config)
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/forgot-password', component: AgentForgotPasswordView },
    { path: '/login', component: { template: '<div>Login</div>' } },
  ] })
  await router.push('/forgot-password')
  await router.isReady()
  const wrapper = mount(AgentForgotPasswordView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return wrapper
}

describe('AgentForgotPasswordView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('submits the authoritative email recovery request without exposing credentials', async () => {
    const forgot = vi.spyOn(agentAPI.auth, 'forgotPassword').mockResolvedValue({ message: 'ok' })
    const wrapper = await setup()
    await wrapper.get('input[type="email"]').setValue(' user@example.com ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(forgot).toHaveBeenCalledWith({ email: 'user@example.com' })
    expect(wrapper.text()).toContain('邮件已提交发送')
    expect(wrapper.html()).not.toContain('app-secret')
    wrapper.unmount()
  })

  it('does not submit when the main site disables password recovery', async () => {
    const forgot = vi.spyOn(agentAPI.auth, 'forgotPassword')
    const wrapper = await setup({ ...enabled, password_reset_enabled: false })
    await wrapper.get('input[type="email"]').setValue('user@example.com')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('当前站点尚未启用密码找回')
    expect(forgot).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
