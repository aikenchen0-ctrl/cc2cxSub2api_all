import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI } from '@/agent/api'
import AgentResetPasswordView from './AgentResetPasswordView.vue'

async function setup(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/reset-password', component: AgentResetPasswordView },
    { path: '/forgot-password', component: { template: '<div>Forgot</div>' } },
    { path: '/login', component: { template: '<div>Login</div>' } },
  ] })
  await router.push(path)
  await router.isReady()
  return mount(AgentResetPasswordView, { global: { plugins: [pinia, router] } })
}

describe('AgentResetPasswordView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('consumes a valid main-site token through the AgentAPI boundary', async () => {
    const reset = vi.spyOn(agentAPI.auth, 'resetPassword').mockResolvedValue({ message: 'ok' })
    const wrapper = await setup('/reset-password?email=user%40example.com&token=one-time-token')
    const passwords = wrapper.findAll('input[type="password"]')
    await passwords[0].setValue('secret123')
    await passwords[1].setValue('secret123')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(reset).toHaveBeenCalledWith('user@example.com', 'one-time-token', 'secret123')
    expect(wrapper.text()).toContain('密码重置成功')
    wrapper.unmount()
  })

  it('rejects an incomplete link before contacting the backend', async () => {
    const reset = vi.spyOn(agentAPI.auth, 'resetPassword')
    const wrapper = await setup('/reset-password?email=user%40example.com')
    expect(wrapper.text()).toContain('重置链接无效')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(reset).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
