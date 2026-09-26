import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentDashboardView from './AgentDashboardView.vue'
import { agentAPI, type AgentContextResponse, type AgentUsageView, type AgentUserView } from '@/agent/api'

type UsagePage = { items: AgentUsageView[]; total: number; page: number; page_size: number }

function usage(overrides: Partial<AgentUsageView> = {}): AgentUsageView {
  return {
    request_id: 'req-dashboard-001',
    proxy_main_user_id: 'user-1',
    billing_main_user_id: 'owner-1',
    reserved_cents: 50,
    actual_cents: 0,
    settlement_status: 'confirmed',
    model: 'gpt-5.5',
    usage_source: 'sub2api_owner_runtime_usage',
    input_tokens: 1000,
    output_tokens: 250,
    total_cost_usd_nanos: 150_000_000,
    actual_cost_usd_nanos: 0,
    actual_cost_reported: true,
    created_at: '2026-09-25T00:00:00Z',
    ...overrides,
  } as AgentUsageView
}

function mountDashboard() {
  return mount(AgentDashboardView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

function dashboardContext(siteName: string, balanceCents: number): AgentContextResponse {
  const user: AgentUserView = {
    agent_id: 'agt-demo', main_user_id: 'user-1', status: 'active', balance_cents: balanceCents,
    created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-25T00:00:00Z',
  }
  return {
    agent: {
      agent_id: 'agt-demo', domain: 'demo.cc2.cx', name: siteName, site_name: siteName,
      status: 'active', billing_mode: 'owner_upstream', main_balance_cents: 5000,
      billing_status: 'ok', wallet_available_cents: 2500, wallet_allocated_cents: 2500,
    },
    authenticated: true, is_agent_admin: false, main_user_id: 'user-1', user,
  } as AgentContextResponse
}

describe('AgentDashboardView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows recent per-user requests and preserves a reported zero actual cost', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(dashboardContext('Demo API', 2500))
    const getWallet = vi.spyOn(agentAPI, 'getWallet')
    const getUsage = vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({
      items: [usage()], total: 1, page: 1, page_size: 5,
    })

    const wrapper = mountDashboard()
    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(1, 5)
    expect(getWallet).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('25.00')
    expect(wrapper.text()).toContain('最近用量')
    expect(wrapper.text()).toContain('req-dashboard-001')
    expect(wrapper.text()).toContain('输入 1,000 · 输出 250')
    expect(wrapper.text()).toContain('Sub2API 实际费用 $0.00 · 标准费用 $0.15')
    expect(wrapper.text()).toContain('本地扣款 0.00')
    expect(wrapper.text()).toContain('查看全部用量')
  })

  it('keeps balance/dashboard content available when recent usage cannot be loaded', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(dashboardContext('Demo API', 2500))
    vi.spyOn(agentAPI, 'getUsage').mockRejectedValue(new Error('usage unavailable'))

    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.text()).toContain('可用余额')
    expect(wrapper.text()).toContain('25.00')
    expect(wrapper.text()).toContain('最近用量')
    expect(wrapper.text()).toContain('加载最近用量失败')
  })

  it('shows the dashboard while its independent usage request is still pending', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(dashboardContext('Ready Site', 8400))
    const pendingUsage = deferred<UsagePage>()
    vi.spyOn(agentAPI, 'getUsage').mockReturnValue(pendingUsage.promise)

    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.text()).toContain('Ready Site')
    expect(wrapper.text()).toContain('84.00')
    expect(wrapper.text()).toContain('正在加载最近用量…')

    pendingUsage.resolve({ items: [], total: 0, page: 1, page_size: 5 })
    await flushPromises()
    expect(wrapper.text()).toContain('暂无模型请求记录')
  })

  it('ignores stale context and usage responses after a newer refresh', async () => {
    const staleContext = deferred<AgentContextResponse>()
    const staleUsage = deferred<UsagePage>()
    vi.spyOn(agentAPI, 'getContext')
      .mockReturnValueOnce(staleContext.promise)
      .mockResolvedValue(dashboardContext('Latest Site', 9100))
    vi.spyOn(agentAPI, 'getUsage')
      .mockReturnValueOnce(staleUsage.promise)
      .mockResolvedValue({ items: [usage({ request_id: 'req-latest' })], total: 1, page: 1, page_size: 5 })

    const wrapper = mountDashboard()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Latest Site')
    expect(wrapper.text()).toContain('91.00')
    expect(wrapper.text()).toContain('req-latest')

    staleContext.resolve(dashboardContext('Stale Site', 100))
    staleUsage.resolve({ items: [usage({ request_id: 'req-stale' })], total: 1, page: 1, page_size: 5 })
    await flushPromises()

    expect(wrapper.text()).toContain('Latest Site')
    expect(wrapper.text()).not.toContain('Stale Site')
    expect(wrapper.text()).toContain('req-latest')
    expect(wrapper.text()).not.toContain('req-stale')
  })
})
