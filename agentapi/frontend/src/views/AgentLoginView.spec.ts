import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type LoginResponse } from '@/agent/api'
import { useAgentSession } from '@/agent/session'
import AgentLoginView from './AgentLoginView.vue'
import { agentPasskeyAPI } from '@/agent/passkeys'

const success: LoginResponse = { access_token: '', refresh_token: '', expires_in: 259200, token_type: 'Cookie', user: { id: '43', agent_admin: false } }
async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/login', component: AgentLoginView },
    { path: '/dashboard', component: { template: '<div>Dashboard</div>' } },
    { path: '/register', component: { template: '<div>Register</div>' } },
    { path: '/forgot-password', component: { template: '<div>Forgot</div>' } },
  ] })
  await router.push('/login')
  await router.isReady()
  const wrapper = mount(AgentLoginView, { global: { plugins: [pinia, router] } })
  await wrapper.get('input[type="email"]').setValue('user@example.com')
  await wrapper.get('input[type="password"]').setValue('password123')
  return { wrapper, router, session: useAgentSession() }
}

describe('AgentLoginView', () => {
  afterEach(() => vi.restoreAllMocks())
  it('starts the registered Sub2API satellite SSO flow without browser credentials', async () => {
    const { wrapper, router } = await setup()
    await router.replace({ path: '/login', query: { redirect: '/usage?page=2' } })
    await flushPromises()
    const link = wrapper.get('[data-testid="main-site-login"]')
    expect(link.attributes('href')).toBe('/api/v1/auth/main-site/login?next=%2Fusage%3Fpage%3D2')
    expect(link.attributes('href')).not.toContain('key=')
    expect(link.attributes('href')).not.toContain('token=')
    wrapper.unmount()
  })
  it('shows passkey login only for a supported configured Agent origin', async () => {
    vi.spyOn(agentPasskeyAPI, 'isSupported').mockReturnValue(true)
    vi.spyOn(agentPasskeyAPI, 'config').mockResolvedValue({ enabled: true, configured: true, supported_origin: true, rp_id: 'example.test' })
    const { wrapper, router, session } = await setup()
    await flushPromises()
    const loginWithPasskey = vi.spyOn(session, 'loginWithPasskey').mockResolvedValue({ id: '43', agent_admin: false })
    await wrapper.get('[data-testid="passkey-login"]').trigger('click')
    await flushPromises()
    expect(loginWithPasskey).toHaveBeenCalledOnce()
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })
  it('toggles password visibility without submitting credentials', async () => {
    const login = vi.spyOn(agentAPI.auth, 'login')
    const { wrapper } = await setup()
    const input = wrapper.get('#agent-login-password')
    expect(input.attributes('type')).toBe('password')
    await wrapper.get('button[aria-label="显示密码"]').trigger('click')
    expect(input.attributes('type')).toBe('text')
    await wrapper.get('button[aria-label="隐藏密码"]').trigger('click')
    expect(input.attributes('type')).toBe('password')
    expect(login).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('rejects an incomplete verification challenge and clears the password', async () => {
    vi.spyOn(agentAPI.auth, 'login').mockResolvedValue({ ...success, user: undefined, requires_2fa: true })
    const { wrapper, router, session } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('登录验证信息不完整')
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(session.isAuthenticated).toBe(false)
    expect(router.currentRoute.value.path).toBe('/login')
    wrapper.unmount()
  })
  it.each([undefined, { id: '', agent_admin: false }])('does not navigate on an invalid user response: %j', async (user) => {
    vi.spyOn(agentAPI.auth, 'login').mockResolvedValue({ ...success, user })
    const { wrapper, router } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('登录响应缺少用户信息')
    expect(router.currentRoute.value.path).toBe('/login')
    wrapper.unmount()
  })
  it('prevents a second submit while login is pending', async () => {
    let resolve!: (response: LoginResponse) => void
    const login = vi.spyOn(agentAPI.auth, 'login').mockImplementation(() => new Promise(r => { resolve = r }))
    const { wrapper, router } = await setup()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(login).toHaveBeenCalledOnce()
    resolve(success)
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })
  it('validates numeric OTP and permits retry after verification failure', async () => {
    const login = vi.spyOn(agentAPI.auth, 'login').mockResolvedValue({ ...success, user: undefined, requires_2fa: true, temp_token: 'private-proof' })
    const verify = vi.spyOn(agentAPI.auth, 'login2FA').mockRejectedValueOnce({ message: '验证码错误' }).mockResolvedValueOnce(success as Required<LoginResponse>)
    const { wrapper, router } = await setup()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.html()).not.toContain('private-proof')
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('abcdef')
    await wrapper.get('form').trigger('submit')
    expect(verify).not.toHaveBeenCalled()
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('验证码错误')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(login).toHaveBeenCalledOnce()
    expect(verify).toHaveBeenCalledTimes(2)
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })
})
