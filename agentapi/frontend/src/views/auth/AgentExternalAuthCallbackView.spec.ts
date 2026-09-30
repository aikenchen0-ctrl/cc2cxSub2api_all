import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { agentAPI } from '@/agent/api'
import AgentExternalAuthCallbackView from './AgentExternalAuthCallbackView.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/auth/linuxdo/callback', component: AgentExternalAuthCallbackView, props: { provider: 'LinuxDo' } },
      { path: '/dashboard', component: { template: '<div>dashboard</div>' } },
      { path: '/usage', component: { template: '<div>usage</div>' } },
      { path: '/login', component: { template: '<div>login</div>' } },
    ],
  })
}

describe('AgentExternalAuthCallbackView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('does not render or forward OAuth secrets and restarts the established main-site SSO flow', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401 })
    const router = makeRouter()
    await router.push('/auth/linuxdo/callback?code=private-code&state=private-state&redirect=%2Fusage%3Fpage%3D2')
    await router.isReady()
    const wrapper = mount(AgentExternalAuthCallbackView, {
      props: { provider: 'LinuxDo' },
      global: { plugins: [router] },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('LinuxDo登录由主站完成')
    expect(wrapper.text()).not.toContain('private-code')
    expect(wrapper.text()).not.toContain('private-state')
    expect(wrapper.get('[data-testid="main-site-auth-restart"]').attributes('href')).toBe('/api/v1/auth/main-site/login?next=%2Fusage%3Fpage%3D2')
  })

  it('rejects external and callback-loop destinations', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401 })
    const router = makeRouter()
    await router.push('/auth/linuxdo/callback?redirect=https%3A%2F%2Fevil.example')
    await router.isReady()
    const wrapper = mount(AgentExternalAuthCallbackView, {
      props: { provider: 'LinuxDo' },
      global: { plugins: [router] },
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="main-site-auth-restart"]').attributes('href')).toBe('/api/v1/auth/main-site/login?next=%2Fdashboard')
  })

  it('returns an already authenticated user to the requested internal page', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    const router = makeRouter()
    await router.push('/auth/linuxdo/callback?redirect=%2Fusage')
    await router.isReady()
    mount(AgentExternalAuthCallbackView, {
      props: { provider: 'LinuxDo' },
      global: { plugins: [router] },
    })
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/usage')
  })
})
