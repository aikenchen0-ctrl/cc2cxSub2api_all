import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import AgentOAuthBindingCallbackView from './AgentOAuthBindingCallbackView.vue'

async function mountCallback(path: string, hash = '') {
  window.location.hash = hash
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/auth/oauth/binding/callback', component: AgentOAuthBindingCallbackView },
      { path: '/profile', component: { template: '<div>profile</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentOAuthBindingCallbackView, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentOAuthBindingCallbackView', () => {
  afterEach(() => {
    window.location.hash = ''
  })

  it('returns a successful provider binding to the local profile page', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/oauth/binding/callback',
      '#status=success&provider=linuxdo&redirect=%2Fprofile',
    )

    expect(router.currentRoute.value.path).toBe('/profile')
    expect(router.currentRoute.value.query).toEqual({ oauth_binding: 'success', provider: 'linuxdo' })
    expect(window.location.hash).toBe('')
    wrapper.unmount()
  })

  it('rejects an external redirect and stays inside the Agent station', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/oauth/binding/callback',
      '#status=success&provider=oidc&redirect=https%3A%2F%2Fevil.example',
    )

    expect(router.currentRoute.value.path).toBe('/profile')
    expect(router.currentRoute.value.query.provider).toBe('oidc')
    wrapper.unmount()
  })

  it('shows a generic recoverable error without rendering provider text', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/oauth/binding/callback',
      '#provider=wechat&error=access_denied&error_description=provider-secret-detail',
    )

    expect(router.currentRoute.value.path).toBe('/auth/oauth/binding/callback')
    expect(wrapper.get('[role="alert"]').text()).toContain('绑定未完成')
    expect(wrapper.text()).not.toContain('provider-secret-detail')
    expect(window.location.hash).toBe('')
    wrapper.unmount()
  })

  it('rejects an unknown provider instead of accepting arbitrary callback data', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/oauth/binding/callback?status=success&provider=unknown',
    )

    expect(router.currentRoute.value.path).toBe('/auth/oauth/binding/callback')
    expect(wrapper.get('[role="alert"]').text()).toContain('无法确认第三方账号类型')
    wrapper.unmount()
  })
})
