import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { agentAPI, type AgentProfile } from '@/agent/api'
import AgentProfileView from './AgentProfileView.vue'

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
    expect(update).not.toHaveBeenCalled()
    expect(changePassword).not.toHaveBeenCalled()
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
