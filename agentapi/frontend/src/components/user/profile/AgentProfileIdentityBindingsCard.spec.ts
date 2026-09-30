import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AgentIdentityBindingsResponse, type AgentProfile } from '@/agent/api'
import { useAgentSession } from '@/agent/session'
import AgentProfileIdentityBindingsCard from './AgentProfileIdentityBindingsCard.vue'

const profile: AgentProfile = {
  id: '42',
  email: 'old@example.com',
  username: 'Example User',
  can_edit: true,
}

const bindingResponse: AgentIdentityBindingsResponse = {
  can_edit: true,
  oauth_binding_supported: true,
  items: [
    { provider: 'email', bound: true, bound_count: 1, can_bind: false, can_unbind: false },
    { provider: 'linuxdo', bound: true, bound_count: 2, can_bind: false, can_unbind: true, display_name: 'Linux User', subject_hint: 'lin***42', note: '可以解除此登录方式。' },
    { provider: 'oidc', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
    { provider: 'wechat', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
    { provider: 'dingtalk', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
  ],
}

function mountCard() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/profile', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(AgentProfileIdentityBindingsCard, {
    props: { profile },
    global: { plugins: [pinia, router] },
  })
  return { wrapper, router }
}

describe('AgentProfileIdentityBindingsCard', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI.profile.bindings, 'get').mockResolvedValue(bindingResponse)
  })

  afterEach(() => vi.restoreAllMocks())

  it('renders the main-site binding states and obtains bind URLs only after a server request', async () => {
    const { wrapper } = mountCard()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('已绑定')
    expect(wrapper.get('[data-testid="profile-binding-linuxdo-status"]').text()).toBe('已绑定')
    expect(wrapper.get('[data-testid="profile-binding-email-status"]').attributes('data-status')).toBe('bound')
    expect(wrapper.get('[data-testid="profile-binding-oidc-status"]').attributes('data-status')).toBe('unbound')
    expect(wrapper.text()).toContain('Linux User')
    expect(wrapper.text()).toContain('lin***42')
    expect(wrapper.text()).toContain('已绑定 2 个身份')
    expect(wrapper.get('[data-testid="profile-binding-oidc-bind"]').text()).toBe('绑定')
    expect(wrapper.find('a[href*="/api/v1/auth/"]').exists()).toBe(false)
  })

  it('starts an OAuth binding through AgentAPI before navigating to the signed main-site URL', async () => {
    const start = vi.spyOn(agentAPI.profile.bindings, 'start').mockResolvedValue({
      provider: 'oidc', method: 'GET', authorize_url: 'https://main.example/api/v1/auth/oauth/oidc/bind/start?satellite_handoff=opaque',
    })
    const { wrapper } = mountCard()
    await flushPromises()

    await wrapper.get('[data-testid="profile-binding-oidc-bind"]').trigger('click')
    await flushPromises()

    expect(start).toHaveBeenCalledWith('oidc')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('sends an email code and applies the authoritative updated profile', async () => {
    const sendCode = vi.spyOn(agentAPI.profile.bindings, 'sendEmailCode').mockResolvedValue({ success: true })
    const updatedProfile = { ...profile, email: 'new@example.com' }
    const bindEmail = vi.spyOn(agentAPI.profile.bindings, 'bindEmail').mockResolvedValue({
      ...bindingResponse,
      profile: updatedProfile,
    })
    const { wrapper } = mountCard()
    await flushPromises()

    await wrapper.get('[data-testid="profile-binding-email-input"]').setValue('new@example.com')
    await wrapper.get('[data-testid="profile-binding-email-send-code"]').trigger('click')
    await flushPromises()
    expect(sendCode).toHaveBeenCalledWith('new@example.com')
    expect(wrapper.text()).toContain('验证码已发送至 new@example.com')

    await wrapper.get('[data-testid="profile-binding-email-code-input"]').setValue('123456')
    await wrapper.get('[data-testid="profile-binding-email-password-input"]').setValue('current-password')
    await wrapper.get('[data-testid="profile-binding-email-submit"]').trigger('click')
    await flushPromises()

    expect(bindEmail).toHaveBeenCalledWith({
      email: 'new@example.com',
      verify_code: '123456',
      password: 'current-password',
    })
    expect(wrapper.emitted('updated')?.[0]).toEqual([updatedProfile])
    expect(wrapper.text()).not.toContain('current-password')
  })

  it('clears the local session and redirects after a provider unbind', async () => {
    const unbind = vi.spyOn(agentAPI.profile.bindings, 'unbind').mockResolvedValue({ success: true, reauthenticate: true })
    const { wrapper, router } = mountCard()
    const auth = useAgentSession()
    auth.user = { id: '42', email: 'old@example.com' }
    await flushPromises()

    await wrapper.get('[data-testid="profile-binding-linuxdo-unbind"]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('解除 LinuxDo 登录方式')
    expect(unbind).not.toHaveBeenCalled()
    ;(dialog?.querySelector('[data-testid="agent-confirm-action"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(unbind).toHaveBeenCalledWith('linuxdo')
    expect(auth.user).toBeNull()
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.identityChanged).toBe('1')
  })

  it('shows a retry state when the binding status cannot be loaded', async () => {
    vi.mocked(agentAPI.profile.bindings.get).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(bindingResponse)
    const { wrapper } = mountCard()
    await flushPromises()
    expect(wrapper.text()).toContain('加载登录方式失败')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(agentAPI.profile.bindings.get).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="profile-binding-email"]').exists()).toBe(true)
  })
})
