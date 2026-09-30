import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AgentProfile } from '@/agent/api'
import AgentProfileView from './AgentProfileView.vue'
import { useAgentSession } from '@/agent/session'

const editableProfile: AgentProfile = {
  id: '42',
  email: 'user@example.com',
  username: 'Example User',
  can_edit: true,
}

function mountProfileView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/login', component: { template: '<div>Login</div>' } },
      { path: '/home', component: { template: '<div>Home</div>' } },
      { path: '/:pathMatch(.*)*', component: { template: '<div>Fallback</div>' } },
    ],
  })
  const wrapper = mount(AgentProfileView, { global: { plugins: [pinia, router] } })
  return { wrapper, router }
}

describe('AgentProfileView', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI.profile.bindings, 'get').mockResolvedValue({
      can_edit: true,
      oauth_binding_supported: false,
      items: [
        { provider: 'email', bound: true, bound_count: 1, can_bind: false, can_unbind: false },
        { provider: 'linuxdo', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
        { provider: 'oidc', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
        { provider: 'wechat', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
        { provider: 'dingtalk', bound: false, bound_count: 0, can_bind: true, can_unbind: false },
      ],
    })
    vi.spyOn(agentAPI.profile.totp, 'getStatus').mockResolvedValue({ enabled: false, feature_enabled: true })
    vi.spyOn(agentAPI.profile.balanceNotify, 'get').mockResolvedValue({
      feature_enabled: false,
      system_default_threshold: 0,
      enabled: false,
      threshold: null,
      extra_emails: [],
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads profile details and updates only the username through AgentAPI', async () => {
    vi.spyOn(agentAPI.profile, 'get').mockResolvedValue(editableProfile)
    const update = vi.spyOn(agentAPI.profile, 'update').mockResolvedValue({
      ...editableProfile,
      username: 'Updated User',
    })
    const { wrapper } = mountProfileView()
    await flushPromises()

    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('主站用户编号')
    await wrapper.get('input[name="username"]').setValue('Updated User')
    await wrapper.findAll('form')[0].trigger('submit')
    await flushPromises()

    expect(update).toHaveBeenCalledWith('Updated User')
    expect(wrapper.text()).toContain('个人资料已更新')
    expect((wrapper.get('input[name="username"]').element as HTMLInputElement).value).toBe('Updated User')
  })

  it('keeps single-sign-on identity sessions read-only', async () => {
    vi.spyOn(agentAPI.profile, 'get').mockResolvedValue({
      ...editableProfile,
      can_edit: false,
    })
    const update = vi.spyOn(agentAPI.profile, 'update')
    const changePassword = vi.spyOn(agentAPI.profile, 'changePassword')
    const { wrapper } = mountProfileView()
    await flushPromises()

    expect((wrapper.get('input[name="username"]').element as HTMLInputElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('仅用于身份识别')
    expect(wrapper.find('[data-testid="profile-totp-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-balance-notify-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-avatar-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-bindings-card"]').exists()).toBe(false)
    expect(update).not.toHaveBeenCalled()
    expect(changePassword).not.toHaveBeenCalled()
    await wrapper.get('form').trigger('submit')
    expect(update).not.toHaveBeenCalled()
  })

  it('does not promote a main-site administrator to an agent administrator', async () => {
    vi.spyOn(agentAPI.profile, 'get').mockResolvedValue(editableProfile)
    const { wrapper } = mountProfileView()
    useAgentSession().user = { id: '42', role: 'admin', agent_admin: false }
    await flushPromises()
    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).not.toContain('本站管理员')
    expect(wrapper.get('[data-testid="profile-shell"]').classes()).toContain('max-w-[950px]')
    expect(wrapper.get('[data-testid="profile-basics-panel"]').exists()).toBe(true)
    useAgentSession().user = { id: '42', role: 'user', agent_admin: true }
    await flushPromises()
    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('本站管理员')
  })

  it('offers retry after profile retrieval fails without rendering editable forms', async () => {
    const get = vi.spyOn(agentAPI.profile, 'get').mockRejectedValueOnce(new Error('offline')).mockResolvedValue(editableProfile)
    const { wrapper } = mountProfileView()
    await flushPromises()
    expect(wrapper.findAll('form')).toHaveLength(0)
    const retry = wrapper.findAll('button').find(button => button.text() === '重新加载')!
    await retry.trigger('click')
    await flushPromises()
    expect(get).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('form')).toHaveLength(2)
  })

  it('prevents duplicate profile submissions while saving', async () => {
    vi.spyOn(agentAPI.profile, 'get').mockResolvedValue(editableProfile)
    let finish!: (value: AgentProfile) => void
    const update = vi.spyOn(agentAPI.profile, 'update').mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const { wrapper } = mountProfileView()
    await flushPromises()
    await wrapper.get('input[name="username"]').setValue('Updated User')
    await wrapper.findAll('form')[0].trigger('submit')
    await wrapper.findAll('form')[0].trigger('submit')
    expect(update).toHaveBeenCalledTimes(1)
    finish({ ...editableProfile, username: 'Updated User' })
    await flushPromises()
    expect(wrapper.text()).toContain('个人资料已更新')
  })

  it('clears the local session and returns to sign-in after password change', async () => {
    vi.spyOn(agentAPI.profile, 'get').mockResolvedValue(editableProfile)
    const changePassword = vi.spyOn(agentAPI.profile, 'changePassword').mockResolvedValue({ message: 'password changed' })
    const { wrapper, router } = mountProfileView()
    await flushPromises()

    const passwordForm = wrapper.findAll('form')[1]
    expect(passwordForm.attributes('novalidate')).toBeDefined()
    await passwordForm.trigger('submit')
    expect(wrapper.text()).toContain('请输入当前密码。')
    expect(changePassword).not.toHaveBeenCalled()

    await wrapper.get('input[name="old_password"]').setValue('current-password')
    await wrapper.get('input[name="new_password"]').setValue('a-new-password')
    await wrapper.get('input[name="confirm_password"]').setValue('a-new-password')
    await passwordForm.trigger('submit')
    await flushPromises()

    expect(changePassword).toHaveBeenCalledWith('current-password', 'a-new-password')
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.passwordChanged).toBe('1')
  })
})
