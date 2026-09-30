import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type LoginResponse, type AuthenticatedResponse } from '@/agent/api'
import { useAgentSession } from '@/agent/session'
import AgentRegisterView from './AgentRegisterView.vue'

const authenticated: AuthenticatedResponse = {
  access_token: '', refresh_token: '', expires_in: 259200, token_type: 'Cookie',
  user: { id: '43', agent_admin: false },
}
const challenge: LoginResponse = { ...authenticated, user: undefined, requires_2fa: true, temp_token: 'temporary-proof' }

async function setup(availability: 'enabled' | 'disabled' | 'error' = 'enabled', path = '/register', emailVerifyEnabled = false) {
  const settings = vi.spyOn(agentAPI, 'getPublicSettings')
  if (availability === 'error') settings.mockRejectedValue(new Error('unavailable'))
  else settings.mockResolvedValue({ registration_enabled: availability === 'enabled', email_verify_enabled: emailVerifyEnabled, site_name: 'AgentAPI', site_logo: '' })
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/register', component: AgentRegisterView },
    { path: '/email-verify', component: { template: '<div>Email verify</div>' } },
    { path: '/login', component: { template: '<div>Login</div>' } },
    { path: '/dashboard', component: { template: '<div>Dashboard</div>' } },
  ] })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentRegisterView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  if (availability === 'enabled') await wrapper.get('input[type="email"]').setValue('new@example.com')
  for (const input of wrapper.findAll('input[type="password"]')) await input.setValue('password123')
  return { wrapper, router, session: useAgentSession() }
}

describe('AgentRegisterView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('toggles each password field independently without registering', async () => {
    const register = vi.spyOn(agentAPI.auth, 'register')
    const { wrapper } = await setup()
    const password = wrapper.get('#agent-register-password')
    const confirmation = wrapper.get('#agent-register-confirm')
    await wrapper.get('button[aria-label="显示密码"]').trigger('click')
    expect(password.attributes('type')).toBe('text')
    expect(confirmation.attributes('type')).toBe('password')
    await wrapper.get('button[aria-label="显示确认密码"]').trigger('click')
    expect(confirmation.attributes('type')).toBe('text')
    expect(register).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not offer registration when the site disables it', async () => {
    const register = vi.spyOn(agentAPI.auth, 'register')
    const { wrapper } = await setup('disabled')
    expect(wrapper.text()).toContain('本站暂未开放注册')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('a[href="/login"]').exists()).toBe(true)
    expect(register).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('distinguishes settings failure from closure and supports retry', async () => {
    const register = vi.spyOn(agentAPI.auth, 'register')
    const { wrapper } = await setup('error')
    expect(wrapper.text()).toContain('无法确认本站注册状态')
    expect(wrapper.find('form').exists()).toBe(false)
    vi.mocked(agentAPI.getPublicSettings).mockResolvedValue({ registration_enabled: true, site_name: 'AgentAPI', site_logo: '' })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
    expect(register).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('completes verification without creating the main account again', async () => {
    const register = vi.spyOn(agentAPI.auth, 'register').mockResolvedValue(challenge)
    const verify = vi.spyOn(agentAPI.auth, 'login2FA')
      .mockRejectedValueOnce({ message: '验证码不正确' })
      .mockResolvedValueOnce(authenticated)
    const { wrapper, router, session } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(session.isAuthenticated).toBe(false)
    expect(router.currentRoute.value.path).toBe('/register')
    expect(wrapper.text()).toContain('完成双重验证')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('temporary-proof')
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('验证码不正确')
    expect(session.isAuthenticated).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(register).toHaveBeenCalledOnce()
    expect(verify).toHaveBeenLastCalledWith('temporary-proof', '123456')
    expect(session.user?.id).toBe('43')
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })

  it('does not authenticate or navigate when the challenge proof is missing', async () => {
    vi.spyOn(agentAPI.auth, 'register').mockResolvedValue({ ...challenge, temp_token: undefined })
    const { wrapper, router, session } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('账号已创建，请前往登录页面完成验证')
    expect(session.user).toBeNull()
    expect(router.currentRoute.value.path).toBe('/register')
    wrapper.unmount()
  })

  it('navigates to the dashboard only after an authenticated registration', async () => {
    vi.spyOn(agentAPI.auth, 'register').mockResolvedValue(authenticated)
    const { wrapper, router, session } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(session.user?.id).toBe('43')
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })

  it('stores a pending registration and defers main account creation until email verification', async () => {
    const register = vi.spyOn(agentAPI.auth, 'register')
    const { wrapper, router } = await setup('enabled', '/register?aff=invite-123', true)
    await wrapper.get('#agent-register-name').setValue('New User')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(register).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/email-verify')
    expect(JSON.parse(sessionStorage.getItem('agent_register_data') || '{}')).toEqual({
      email: 'new@example.com',
      password: 'password123',
      username: 'New User',
      affiliate_code: 'INVITE-123',
    })
    wrapper.unmount()
  })

  it('shows and submits the affiliate code from the tenant invite link', async () => {
	const register = vi.spyOn(agentAPI.auth, 'register').mockResolvedValue(authenticated)
	const { wrapper } = await setup('enabled', '/register?aff=inviter-123')
	expect(wrapper.text()).toContain('已应用邀请码')
	expect(wrapper.text()).toContain('INVITER-123')
	await wrapper.get('form').trigger('submit')
	await flushPromises()
	expect(register).toHaveBeenCalledWith('new@example.com', 'password123', undefined, 'INVITER-123')
	wrapper.unmount()
  })

  it.each(['REGISTERED_LOGIN_REQUIRED', 'USER_MAPPING_FAILED'])('offers login recovery instead of repeat registration for %s', async (code) => {
    const register = vi.spyOn(agentAPI.auth, 'register').mockRejectedValue({ status: 503, code })
    const { wrapper, router, session } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('账号已创建')
    expect(wrapper.text()).toContain('不要重复注册')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.get('a[href="/login"]').exists()).toBe(true)
    expect(register).toHaveBeenCalledOnce()
    expect(session.user).toBeNull()
    expect(router.currentRoute.value.path).toBe('/register')
    wrapper.unmount()
  })
})
