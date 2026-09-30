import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminSettingsView from './AgentAdminSettingsView.vue'
import { agentAPI } from '@/agent/api'
import { applyHomeSettings, compactHomeEnabled, homeContent, siteSubtitle } from '@/agent/branding'

function createContext() {
  return {
    agent: {
      agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', site_logo: '/old.svg', doc_url: 'https://docs.old.example.com/', contact_info: 'old-support@example.com', status: 'active',
      billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
    },
    authenticated: true,
    is_agent_admin: true,
  }
}

describe('AgentAdminSettingsView', () => {
  afterEach(() => { vi.restoreAllMocks(); applyHomeSettings({}) })

  it('loads tenant branding and saves only the current agent branding fields', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({ name: 'New Agent', site_name: 'New Site', site_logo: '/new.svg', doc_url: 'https://docs.new.example.com/', contact_info: 'new-support@example.com' })
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    expect(wrapper.text()).toContain('系统设置')
    expect(wrapper.get<HTMLInputElement>('#agent-name').element.value).toBe('Example Agent')
    expect(wrapper.get<HTMLInputElement>('#site-name').element.value).toBe('Example Site')
    expect(wrapper.get<HTMLInputElement>('#site-logo').element.value).toBe('/old.svg')
    expect(wrapper.get<HTMLInputElement>('#doc-url').element.value).toBe('https://docs.old.example.com/')
    expect(wrapper.get<HTMLInputElement>('#contact-info').element.value).toBe('old-support@example.com')

    await wrapper.get('#agent-name').setValue(' New Agent ')
    await wrapper.get('#site-name').setValue(' New Site ')
    await wrapper.get('#site-logo').setValue(' /new.svg ')
    await wrapper.get('#doc-url').setValue(' https://docs.new.example.com/ ')
    await wrapper.get('#contact-info').setValue(' new-support@example.com ')
    await wrapper.findAll('button').find(button => button.text().includes('保存品牌信息'))!.trigger('click')
    await flushPromises()

    expect(update).toHaveBeenCalledWith({ name: 'New Agent', site_name: 'New Site', site_logo: '/new.svg', doc_url: 'https://docs.new.example.com/', contact_info: 'new-support@example.com', site_subtitle: '', compact_home_enabled: false, home_content: '' })
    expect(wrapper.text()).toContain('本站品牌信息已保存')
    expect(wrapper.text()).not.toContain('Admin API Key')
  })

  it('does not mount settings until tenant context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(createContext())
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载系统设置')
    expect(wrapper.find('#agent-name').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(wrapper.get<HTMLInputElement>('#agent-name').element.value).toBe('Example Agent')
  })

  it('saves homepage controls to tenant branding and immediately applies the returned settings', async () => {
    const settings = { site_subtitle: '本站副标题', compact_home_enabled: true, home_content: '<h1>本站首页</h1>' }
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({
      name: 'Example Agent', site_name: 'Example Site', site_logo: '/old.svg', doc_url: '', contact_info: '', ...settings,
    })
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()
    await wrapper.get('#site-subtitle').setValue('本站副标题')
    await wrapper.get('#compact-home-enabled').setValue(true)
    await wrapper.get('#home-content').setValue('<h1>本站首页</h1>')
    await wrapper.findAll('button').find(button => button.text().includes('保存品牌信息'))!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(expect.objectContaining(settings))
    expect(siteSubtitle.value).toBe(settings.site_subtitle)
    expect(compactHomeEnabled.value).toBe(true)
    expect(homeContent.value).toBe(settings.home_content)
    wrapper.unmount()
  })

  it('keeps draft homepage settings on a failed save without applying them publicly', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    vi.spyOn(agentAPI, 'updateBranding').mockRejectedValue(new Error('save unavailable'))
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()
    await wrapper.get('#home-content').setValue('<h1>尚未保存</h1>')
    await wrapper.findAll('button').find(button => button.text().includes('保存品牌信息'))!.trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLTextAreaElement>('#home-content').element.value).toBe('<h1>尚未保存</h1>')
    expect(homeContent.value).toBe('')
    expect(wrapper.get('[role="alert"]').text()).toContain('更新本站品牌信息失败')
    wrapper.unmount()
  })

  it('states that main-site global settings and credentials remain isolated', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text().includes('数据边界'))!.trigger('click')
    expect(wrapper.text()).toContain('不能修改主站全局设置或其他代理站配置')
    expect(wrapper.text()).toContain('不能读取主站管理员 Key、JWT 或内部凭证')
    expect(wrapper.text()).toContain('用户余额与实际计费记录始终以主站为准')
  })
})
