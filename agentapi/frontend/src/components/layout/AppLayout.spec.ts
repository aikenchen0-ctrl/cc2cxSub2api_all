import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppLayout from './AppLayout.vue'
import Home from '@/views/AgentHomeView.vue'
import { useAgentSession } from '@/agent/session'
import { notifyAgentAccountFactsChanged } from '@/agent/accountFacts'
import { applyBranding } from '@/agent/branding'
import { agentAPI } from '@/agent/api'

const onboardingMocks = vi.hoisted(() => ({
  startTour: vi.fn(async () => true),
  replayTour: vi.fn(),
  disposeTour: vi.fn(),
}))

vi.mock('@/composables/useAgentOnboardingTour', () => ({
  useAgentOnboardingTour: () => onboardingMocks,
}))

beforeEach(() => {
  setActivePinia(createPinia())
  sessionStorage.clear()
  localStorage.clear()
  Object.values(onboardingMocks).forEach(mock => mock.mockClear())
  applyBranding('本站 A', '/a.svg')
  vi.spyOn(agentAPI.contentPages, 'list').mockResolvedValue({ items: [], total: 0 })
  vi.spyOn(agentAPI.announcements, 'list').mockResolvedValue({ items: [], total: 0, unread: 0 })
  vi.spyOn(agentAPI, 'getContext').mockRejectedValue(new Error('No balance fixture'))
})
afterEach(() => vi.restoreAllMocks())
async function render(admin = false, home = false) {
  const session = useAgentSession()
  session.initialized = true
  session.user = { id: '42', email: 'a@example.test', agent_admin: admin }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' }, meta: { title: '仪表盘' } }] })
  await router.push('/dashboard')
  return { router, wrapper: mount(home ? Home : AppLayout, { global: { plugins: [router], stubs: { LzParticleScene: true } }, slots: home ? {} : { default: '<div data-testid="panel">业务面板</div>' } }) }
}
describe('Sub2API-style tenant layout', () => {
  it('uses sidebar and panel; ordinary users see no admin menus', async () => {
    const { wrapper } = await render()
    expect(wrapper.find('aside.sidebar').exists()).toBe(true)
    expect(wrapper.find('[data-testid="panel"]').text()).toBe('业务面板')
    expect(wrapper.get('[data-testid="header-page-description"]').text()).toContain('主站余额')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/model-plaza')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/batch-image')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).not.toContain('/available-channels')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).not.toContain('/monitor')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).not.toContain('/redeem')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).not.toContain('/subscriptions')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/orders')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/affiliate')
    expect(wrapper.findAll('a').some(a => a.attributes('href')?.startsWith('/admin/'))).toBe(false)
    wrapper.unmount()
  })
  it('offers tenant admin sections, including safe main-path runtime resource views', async () => {
    const { wrapper } = await render(true)
    await wrapper.get('[data-testid="sidebar-group-admin-orders"]').trigger('click')
    await wrapper.get('[data-testid="sidebar-group-admin-affiliates"]').trigger('click')
    await wrapper.get('[data-testid="sidebar-group-admin-channels"]').trigger('click')
    const paths = wrapper.findAll('a').map(a => a.attributes('href'))
    expect(paths).toContain('/admin/users')
    expect(paths).toContain('/admin/agent-provisioning')
    expect(paths).toContain('/admin/ops')
    expect(paths).toContain('/admin/orders/dashboard')
    expect(paths).toContain('/admin/orders/plans')
    expect(paths).toContain('/admin/orders')
    expect(paths).toContain('/admin/subscriptions')
    expect(paths).toContain('/admin/affiliates/invites')
    expect(paths).toContain('/admin/affiliates/rebates')
    expect(paths).toContain('/admin/affiliates/transfers')
    expect(paths).toContain('/admin/channels')
    expect(paths).toContain('/admin/channels/pricing')
    expect(paths).not.toContain('/admin/channels/monitor')
    expect(paths).not.toContain('/admin/models')
    expect(paths).toContain('/admin/audit-logs')
    expect(paths).not.toContain('/admin/audit')
    expect(paths).toContain('/admin/announcements')
    expect(paths).toContain('/admin/content')
    expect(paths).toContain('/admin/settings')
    expect(paths).toContain('/admin/promo-codes')
    for (const removed of ['/admin/groups', '/admin/accounts', '/admin/plugins', '/admin/proxies', '/admin/redeem', '/admin/risk-control', '/admin/prompt-audit', '/admin/upstream-audit']) {
      expect(paths).not.toContain(removed)
    }
    expect(paths.every(path => !path?.startsWith('/api/'))).toBe(true)
    wrapper.unmount()
  })
  it('uses the main-site collapsible payment, affiliate, and channel menu groups', async () => {
    const { wrapper } = await render(true)
    const payment = wrapper.get('[data-testid="sidebar-group-admin-orders"]')
    const affiliates = wrapper.get('[data-testid="sidebar-group-admin-affiliates"]')
    const channels = wrapper.get('[data-testid="sidebar-group-admin-channels"]')
    expect(payment.attributes('aria-expanded')).toBe('false')
    expect(affiliates.attributes('aria-expanded')).toBe('false')
    expect(channels.attributes('aria-expanded')).toBe('false')
    await payment.trigger('click')
    expect(payment.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/admin/orders/dashboard')
    await affiliates.trigger('click')
    expect(affiliates.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/admin/affiliates/invites')
    await channels.trigger('click')
    expect(channels.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/admin/channels/pricing')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).not.toContain('/admin/channels/monitor')
    wrapper.unmount()
  })
  it('reflects tenant branding updates immediately without a main API request', async () => {
    const { wrapper } = await render(true)
    applyBranding('本站 B', '/b.svg')
    await flushPromises()
    expect(wrapper.text()).toContain('本站 B')
    expect(wrapper.text()).not.toContain('本站 A')
    expect(wrapper.find('aside img').attributes('src')).toBe('/b.svg')
    wrapper.unmount()
  })
  it('ports the main header balance and user dropdown using tenant-safe APIs', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue({
      authenticated: true,
      is_agent_admin: true,
      main_user_id: '42',
      agent: { agent_id: 'agent-1', domain: 'agent.test', name: '本站 A', site_name: '本站 A', status: 'active', billing_mode: 'user_upstream', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0 },
      user: { agent_id: 'agent-1', main_user_id: '42', email: 'a@example.test', status: 'active', balance_cents: 12345, frozen_balance_cents: 250, created_at: '', updated_at: '' },
    })
    const { wrapper } = await render(true)
    await flushPromises()
    expect(wrapper.get('[data-testid="header-balance"]').text()).toContain('$123.45')
    expect(wrapper.get('[data-testid="header-balance"]').text()).toContain('冻结 $2.50')
    expect(wrapper.get('[data-testid="header-balance-details"]').text()).toContain('总余额$125.95')
    await wrapper.get('[aria-label="用户菜单"]').trigger('click')
    expect(wrapper.text()).toContain('代理站管理员')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('/admin/dashboard')
    expect(wrapper.text()).toContain('退出登录')
    wrapper.unmount()
  })
  it('ports the main-site onboarding entry and only targets routes available in this tenant console', async () => {
    vi.spyOn(agentAPI, 'getContext').mockRejectedValue(new Error('balance unavailable'))
    const { wrapper } = await render(true)
    await flushPromises()

    expect(wrapper.get('[data-tour="sidebar-dashboard"]').attributes('href')).toBe('/dashboard')
    expect(wrapper.get('[data-tour="sidebar-model-plaza"]').attributes('href')).toBe('/model-plaza')
    expect(wrapper.get('[data-tour="sidebar-api-keys"]').attributes('href')).toBe('/keys')
    expect(wrapper.get('[data-tour="sidebar-usage"]').attributes('href')).toBe('/usage')
    expect(wrapper.get('[data-tour="sidebar-profile"]').attributes('href')).toBe('/profile')
    expect(wrapper.get('[data-tour="sidebar-admin-dashboard"]').attributes('href')).toBe('/admin/dashboard')

    await wrapper.get('[aria-label="用户菜单"]').trigger('click')
    await wrapper.get('[data-testid="replay-onboarding"]').trigger('click')
    expect(onboardingMocks.replayTour).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    expect(onboardingMocks.disposeTour).toHaveBeenCalledTimes(1)
  })
  it('refreshes the copied header balance after an authoritative account change', async () => {
    const context = vi.spyOn(agentAPI, 'getContext')
      .mockResolvedValueOnce({
        authenticated: true, is_agent_admin: false, main_user_id: '42', agent: {} as never,
        user: { balance_cents: 100 } as never,
      })
      .mockResolvedValue({
        authenticated: true, is_agent_admin: false, main_user_id: '42', agent: {} as never,
        user: { balance_cents: 1250 } as never,
      })
    const { wrapper } = await render(true)
    await flushPromises()
    expect(wrapper.get('[data-testid="header-balance"]').text()).toContain('$1.00')
    const callsBeforeRefresh = context.mock.calls.length

    notifyAgentAccountFactsChanged({ balanceCents: 1250, refreshSubscriptions: false })
    await flushPromises()

    expect(context.mock.calls.length).toBeGreaterThan(callsBeforeRefresh)
    expect(wrapper.get('[data-testid="header-balance"]').text()).toContain('$12.50')
    wrapper.unmount()
  })
  it('shows the current tenant documentation link in the copied main header', async () => {
    applyBranding('本站 A', '/a.svg', 'https://docs.agent-a.example.com/start')
    vi.spyOn(agentAPI, 'getContext').mockRejectedValue(new Error('balance unavailable'))
    const { wrapper } = await render(true)
    await flushPromises()
    const docs = wrapper.findAll('a').find(link => link.text().includes('文档'))
    expect(docs?.attributes('href')).toBe('https://docs.agent-a.example.com/start')
    expect(docs?.attributes('target')).toBe('_blank')
    expect(docs?.attributes('rel')).toContain('noopener')
    wrapper.unmount()
  })
  it('shows tenant contact information in the copied user menu as plain text', async () => {
    applyBranding('本站 A', '/a.svg', '', 'support@agent-a.example.com')
    vi.spyOn(agentAPI, 'getContext').mockRejectedValue(new Error('balance unavailable'))
    const { wrapper } = await render(true)
    await flushPromises()
    await wrapper.get('[aria-label="用户菜单"]').trigger('click')
    const contact = wrapper.get('[data-testid="header-contact-info"]')
    expect(contact.text()).toContain('support@agent-a.example.com')
    expect(contact.find('a').exists()).toBe(false)
    wrapper.unmount()
  })
  it('collapses desktop sidebar and closes mobile menu on navigation', async () => {
    const { wrapper, router } = await render()
    await wrapper.get('[aria-label="收起侧栏"]').trigger('click')
    expect(sessionStorage.getItem('agentapi_sidebar_collapsed')).toBe('true')
    await wrapper.get('[aria-label="打开菜单"]').trigger('click')
    expect(wrapper.find('[aria-label="关闭菜单遮罩"]').exists()).toBe(true)
    await router.push('/keys')
    await flushPromises()
    expect(wrapper.find('[aria-label="关闭菜单遮罩"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens the full mobile menu while retaining the desktop collapsed preference', async () => {
    const { wrapper, router } = await render(true)
    await wrapper.get('[aria-label="收起侧栏"]').trigger('click')
    expect(wrapper.get('#agent-sidebar').classes()).toContain('w-[72px]')
    expect(wrapper.get('[aria-label="支付管理"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('[aria-label="打开菜单"]').trigger('click')
    expect(wrapper.get('#agent-sidebar').classes()).toContain('w-64')
    expect(wrapper.get('[aria-label="支付管理"] .sidebar-label').attributes('aria-hidden')).toBeUndefined()
    await wrapper.get('[aria-label="支付管理"]').trigger('click')
    expect(wrapper.get('#agent-sidebar a[href="/admin/orders/plans"]').exists()).toBe(true)
    expect(sessionStorage.getItem('agentapi_sidebar_collapsed')).toBe('true')
    await router.push('/admin/orders/plans')
    await flushPromises()
    expect(wrapper.find('[aria-label="关闭菜单遮罩"]').exists()).toBe(false)
    expect(wrapper.get('#agent-sidebar').classes()).toContain('w-[72px]')
    wrapper.unmount()
  })

  it('keeps collapsed navigation accessible and restores expanded group selection', async () => {
    const { wrapper, router } = await render(true)
    await router.push('/admin/orders/plans')
    await flushPromises()
    expect(wrapper.get('#agent-sidebar a[href="/admin/orders/plans"]').attributes('aria-current')).toBe('page')
    expect(wrapper.get('[aria-label="支付管理"]').classes()).not.toContain('sidebar-link-active')
    await wrapper.get('[aria-label="收起侧栏"]').trigger('click')
    expect(wrapper.get('#agent-sidebar a[href="/profile"]').attributes('aria-label')).toBe('个人资料')
    expect(wrapper.get('[aria-label="支付管理"]').classes()).toContain('sidebar-link-active')
    await wrapper.get('[aria-label="支付管理"]').trigger('click')
    expect(wrapper.get('[aria-label="支付管理"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('[aria-label="展开侧栏"]').trigger('click')
    expect(wrapper.get('[aria-label="支付管理"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#agent-sidebar a[href="/admin/orders/plans"]').attributes('aria-current')).toBe('page')
    wrapper.unmount()
  })
  it('ports the native particle home and replaces hardcoded main-site branding', async () => {
    applyBranding('本站 A', '/a.svg', 'https://docs.agent-a.example.com/')
    const { wrapper } = await render(true, true)
    expect(wrapper.find('.lz-home').exists()).toBe(true)
    expect(wrapper.find('.lz-particle-stage').exists()).toBe(true)
    expect(wrapper.find('.lz-brand-name').text()).toBe('本站 A')
    expect(wrapper.find('.lz-brand-mark img').attributes('src')).toBe('/a.svg')
    expect(wrapper.find('.lz-start').attributes('href')).toBe('/admin/dashboard')
    expect(wrapper.findAll('a').map(link => link.attributes('href'))).toContain('/downloads')
    expect(wrapper.findAll('a').map(link => link.attributes('href'))).toContain('https://docs.agent-a.example.com/')
    wrapper.unmount()
  })
})
