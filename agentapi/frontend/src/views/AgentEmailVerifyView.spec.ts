import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AuthenticatedResponse } from '@/agent/api'
import AgentEmailVerifyView from './AgentEmailVerifyView.vue'

const pending = {
  email: 'newuser@example.com',
  password: 'password123',
  username: 'New User',
  affiliate_code: 'INVITE-123',
}
const authenticated: AuthenticatedResponse = {
  access_token: '', refresh_token: '', expires_in: 259200, token_type: 'Cookie',
  user: { id: '43', agent_admin: false },
}

async function setup(withPending = true) {
  if (withPending) sessionStorage.setItem('agent_register_data', JSON.stringify(pending))
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/email-verify', component: AgentEmailVerifyView },
    { path: '/register', component: { template: '<div>Register</div>' } },
    { path: '/login', component: { template: '<div>Login</div>' } },
    { path: '/dashboard', component: { template: '<div>Dashboard</div>' } },
  ] })
  await router.push('/email-verify')
  await router.isReady()
  const wrapper = mount(AgentEmailVerifyView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentEmailVerifyView', () => {
  beforeEach(() => sessionStorage.clear())
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it('auto-sends a code without exposing the password and completes registration with the code', async () => {
    const send = vi.spyOn(agentAPI.auth, 'sendVerifyCode').mockResolvedValue({ message: 'ok', countdown: 60 })
    const register = vi.spyOn(agentAPI.auth, 'register').mockResolvedValue(authenticated)
    const { wrapper, router } = await setup()

    expect(send).toHaveBeenCalledWith('newuser@example.com')
    expect(wrapper.text()).toContain('ne***@example.com')
    expect(wrapper.html()).not.toContain('password123')
    expect(wrapper.text()).toContain('验证码已发送')
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(register).toHaveBeenCalledWith('newuser@example.com', 'password123', 'New User', 'INVITE-123', '123456')
    expect(sessionStorage.getItem('agent_register_data')).toBeNull()
    expect(router.currentRoute.value.path).toBe('/dashboard')
    wrapper.unmount()
  })

  it('does not contact the main site when the pending registration is missing', async () => {
    const send = vi.spyOn(agentAPI.auth, 'sendVerifyCode')
    const register = vi.spyOn(agentAPI.auth, 'register')
    const { wrapper } = await setup(false)
    expect(wrapper.text()).toContain('注册会话已失效')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(send).not.toHaveBeenCalled()
    expect(register).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps pending registration after an invalid upstream code so the user can retry', async () => {
    vi.spyOn(agentAPI.auth, 'sendVerifyCode').mockResolvedValue({ message: 'ok', countdown: 60 })
    vi.spyOn(agentAPI.auth, 'register').mockRejectedValue({ message: '验证码不正确' })
    const { wrapper, router } = await setup()
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('验证码不正确')
    expect(sessionStorage.getItem('agent_register_data')).not.toBeNull()
    expect(router.currentRoute.value.path).toBe('/email-verify')
    wrapper.unmount()
  })
})
