import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import AgentAdminDashboardView from './AgentAdminDashboardView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Agent', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

const insights = {
  source: 'sub2api_usage_snapshots', status: 'partial' as const, start: '2026-09-28T00:00:00Z', end: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z', scope: 'agent' as const,
  requests: 12, measured: 10, missing_actual: 2, unobserved: 2, pending: 1,
  input_tokens: 1000, output_tokens: 500, cache_read_tokens: 200, cache_creation_tokens: 100,
  standard_cost_usd_nanos: 2_000_000_000, actual_cost_usd_nanos: 1_500_000_000, route_observed: 10, route_mismatch: 1,
  models: [], trend: [],
}

function mockSuccessfulLoad(): void {
  vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
  vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue(insights)
  vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: [], total: 3 })
}

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/dashboard', component: AgentAdminDashboardView },
      { path: '/admin/orders', component: { template: '<div />' } },
      { path: '/admin/usage', component: { template: '<div />' } },
      { path: '/admin/announcements', component: { template: '<div />' } },
      { path: '/admin/settings', component: { template: '<div />' } },
    ],
  })
  await router.push('/admin/dashboard')
  await router.isReady()
  const wrapper = mount(AgentAdminDashboardView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('AgentAdminDashboardView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('shows authoritative tenant usage without exposing a user directory', async () => {
    mockSuccessfulLoad()
    const wrapper = await mountView()

    expect(agentAPI.getUsageInsights).toHaveBeenCalledWith('24h')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('1.80K')
    expect(wrapper.text()).toContain('$1.5000')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('10 / 12')
    expect(wrapper.text()).toContain('用户余额计费')
    expect(wrapper.get('[data-status="active"]').text()).toBe('运行中')
    expect(wrapper.text()).toContain('不会越过本站权限边界')
    expect(wrapper.text()).not.toContain('服务账号')
  })

  it('keeps healthy tenant facts visible when one optional source fails', async () => {
    mockSuccessfulLoad()
    vi.mocked(agentAPI.getSettlements).mockRejectedValue(new Error('unavailable'))
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('部分数据暂不可用：待确认结算')
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('12')
  })

  it('does not render dashboard facts until the tenant context succeeds', async () => {
    vi.spyOn(agentAPI, 'getContext').mockRejectedValue(new Error('context unavailable'))
    vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue(insights)
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: [], total: 99 })
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('无法加载站长仪表盘')
    expect(wrapper.find('[aria-label="核心统计"]').exists()).toBe(false)
    expect(agentAPI.getUsageInsights).not.toHaveBeenCalled()
    expect(agentAPI.getSettlements).not.toHaveBeenCalled()
  })

  it('keeps quick actions on tenant-safe management routes', async () => {
    mockSuccessfulLoad()
    const wrapper = await mountView()
    const links = wrapper.findAll('a').map(item => item.attributes('href'))

    expect(links).toEqual(expect.arrayContaining(['/admin/users', '/admin/usage', '/admin/announcements', '/admin/settings']))
    expect(links.some(path => path?.includes('sub2api'))).toBe(false)
  })
})
