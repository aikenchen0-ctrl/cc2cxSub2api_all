import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminSettingsView from './AgentAdminSettingsView.vue'
import { agentAPI } from '@/agent/api'
import { createMemoryHistory, createRouter } from 'vue-router'
import { applyBranding, applyHomeSettings, applyPageTitle, compactHomeEnabled, homeContent, siteLogo, siteName, siteSubtitle } from '@/agent/branding'

function createContext(domain = 'agent.example.com') {
  return {
    agent: {
      agent_id: 'agent-1', domain, api_base_url: 'https://agent.example.com/v1', name: 'Example Agent', site_name: 'Example Site', site_logo: '/old.svg', doc_url: 'https://docs.old.example.com/', contact_info: 'old-support@example.com', status: 'active',
      billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
    },
    authenticated: true,
    is_agent_admin: true,
  }
}

describe('AgentAdminSettingsView', () => {
  it('opens payment inside system settings and keeps the selected tab in the URL', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const legacy = vi.spyOn(agentAPI, 'paymentConfig')
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/settings', component: AgentAdminSettingsView }] })
    await router.push('/admin/settings?tab=payment')
    await router.isReady()
    const wrapper = mount(AgentAdminSettingsView, { global: { plugins: [router], stubs: { AgentPaymentSettings: { template: '<div data-testid="payment-panel">Payment panel</div>' } } } })
    await flushPromises()
    expect(wrapper.get('#settings-tab-payment').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="payment-panel"]').exists()).toBe(true)
    expect(legacy).not.toHaveBeenCalled()
    await wrapper.get('#settings-tab-branding').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('branding')
    expect(wrapper.find('#agent-name').exists()).toBe(true)
    await wrapper.get('#settings-tab-payment').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('payment')
    expect(wrapper.find('[data-testid="payment-panel"]').exists()).toBe(true)
    wrapper.unmount()
  })
  afterEach(() => { vi.restoreAllMocks(); applyPageTitle(''); applyBranding('AgentAPI', '/logo.svg'); applyHomeSettings({}) })
  beforeEach(() => {
    vi.spyOn(agentAPI, 'paymentConfig').mockResolvedValue({ enabled: false, provider: 'manual', currency: 'CNY', webhook_secret_configured: false, min_amount_cents: 100, max_amount_cents: 1000000, order_ttl_seconds: 1800 })
  })

  it('loads tenant branding and saves only the current agent branding fields', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({ name: 'New Agent', site_name: 'New Site', site_logo: '/new.svg', doc_url: 'https://docs.new.example.com/', contact_info: 'new-support@example.com', api_base_url: 'https://api.new.example.com/v1' })
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    expect(wrapper.text()).toContain('系统设置')
    expect(wrapper.get<HTMLInputElement>('#agent-name').element.value).toBe('Example Agent')
    expect(wrapper.get<HTMLInputElement>('#site-name').element.value).toBe('Example Site')
    expect(wrapper.get<HTMLInputElement>('#site-logo').element.value).toBe('/old.svg')
    expect(wrapper.get<HTMLInputElement>('#doc-url').element.value).toBe('https://docs.old.example.com/')
    expect(wrapper.get<HTMLInputElement>('#contact-info').element.value).toBe('old-support@example.com')
    expect(wrapper.get<HTMLInputElement>('#api-base-url').element.value).toBe('https://agent.example.com/v1')
    expect(wrapper.text()).toContain('站点名称')
    expect(wrapper.text()).toContain('后台备注名称')
    expect(wrapper.get('aside').text()).toContain('Example Site')
    expect(wrapper.get('aside').text()).not.toContain('Example Agent')
    expect(siteName.value).toBe('Example Site')
    expect(siteLogo.value).toBe('/old.svg')

    await wrapper.get('#agent-name').setValue(' New Agent ')
    await wrapper.get('#site-name').setValue(' New Site ')
    await wrapper.get('#site-logo').setValue(' /new.svg ')
    await wrapper.get('#doc-url').setValue(' https://docs.new.example.com/ ')
    await wrapper.get('#contact-info').setValue(' new-support@example.com ')
    await wrapper.get('#api-base-url').setValue(' https://api.new.example.com/v1 ')
    await wrapper.findAll('button').find(button => button.text().includes('保存品牌信息'))!.trigger('click')
    await flushPromises()

    expect(update).toHaveBeenCalledWith({ name: 'New Agent', site_name: 'New Site', site_logo: '/new.svg', doc_url: 'https://docs.new.example.com/', contact_info: 'new-support@example.com', api_base_url: 'https://api.new.example.com/v1', site_subtitle: '', compact_home_enabled: false, home_content: '' })
    expect(wrapper.text()).toContain('本站品牌信息已保存')
    expect(wrapper.text()).not.toContain('Admin API Key')
  })

  it('uses the public site name for branding and falls back to it for the admin note', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({
      name: '德雄专属', site_name: '德雄专属', site_logo: '', doc_url: '', contact_info: '', api_base_url: '', site_subtitle: '新副标题', compact_home_enabled: false, home_content: '',
    })
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    await wrapper.get('#site-name').setValue(' 德雄专属 ')
    await wrapper.get('#agent-name').setValue('')
    await wrapper.get('#site-subtitle').setValue('新副标题')
    await wrapper.findAll('button').find(button => button.text().includes('保存品牌信息'))!.trigger('click')
    await flushPromises()

    expect(update).toHaveBeenCalledWith(expect.objectContaining({ name: '德雄专属', site_name: '德雄专属', site_subtitle: '新副标题' }))
    expect(siteName.value).toBe('德雄专属')
    expect(siteSubtitle.value).toBe('新副标题')
    expect(wrapper.get('aside').text()).toContain('德雄专属')
    wrapper.unmount()
  })

  it('keeps an uploaded base64 logo in settings without showing the raw string', async () => {
    const logo = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB'
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const upload = vi.spyOn(agentAPI, 'uploadLogo').mockResolvedValue({
      name: 'Example Agent', site_name: 'Example Site', site_logo: logo, doc_url: '', contact_info: '', api_base_url: '', site_subtitle: '', compact_home_enabled: false, home_content: '',
    })
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    const input = wrapper.get('#site-logo-file')
    Object.defineProperty(input.element, 'files', { value: [new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], 'logo.png', { type: 'image/png' })] })
    await input.trigger('change')
    await flushPromises()

    expect(upload).toHaveBeenCalledOnce()
    expect(siteLogo.value).toBe(logo)
    expect(wrapper.text()).toContain('图片已内嵌为 Base64')
    expect(wrapper.find('#site-logo').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(logo)
    wrapper.unmount()
  })

  it('explains why anonymous shared-host visitors do not see a tenant brand', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext(''))
    const wrapper = mount(AgentAdminSettingsView)
    await flushPromises()

    expect(wrapper.text()).toContain('当前是共享入口')
    expect(wrapper.text()).toContain('登录后会按当前会话显示本站品牌')
    wrapper.unmount()
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
      name: 'Example Agent', site_name: 'Example Site', site_logo: '/old.svg', doc_url: '', contact_info: '', api_base_url: 'https://agent.example.com/v1', ...settings,
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

  it('shows only branding and payment tabs and falls back from the removed tab', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(createContext())
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/settings', component: AgentAdminSettingsView }] })
    await router.push('/admin/settings?tab=boundary')
    await router.isReady()
    const wrapper = mount(AgentAdminSettingsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.findAll('[role="tab"]').map(tab => tab.text())).toEqual(['站点设置', '支付配置'])
    expect(wrapper.text()).not.toContain('数据与权限边界')
    expect(wrapper.get('#settings-tab-branding').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('#agent-name').exists()).toBe(true)
    wrapper.unmount()
  })
})
