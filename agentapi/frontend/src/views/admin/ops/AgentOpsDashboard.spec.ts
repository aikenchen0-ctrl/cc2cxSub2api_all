import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentUsageView, type UsageInsights } from '@/agent/api'
import AgentOpsDashboard from './AgentOpsDashboard.vue'

const insights: UsageInsights = {
  source: 'local_main_usage_snapshots', status: 'partial', scope: 'agent',
  start: '2026-09-28T00:00:00Z', end: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
  requests: 3, measured: 2, missing_actual: 1, unobserved: 1, pending: 1,
  input_tokens: 100, output_tokens: 50, cache_read_tokens: 20, cache_creation_tokens: 10,
  standard_cost_usd_nanos: 500, actual_cost_usd_nanos: 400, route_observed: 2, route_mismatch: 1,
  models: [{ key: 'gpt-5.5', requests: 3, measured: 2, missing_actual: 1, unobserved: 1, pending: 1, input_tokens: 100, output_tokens: 50, cache_read_tokens: 20, cache_creation_tokens: 10, standard_cost_usd_nanos: 500, actual_cost_usd_nanos: 400, route_observed: 2, route_mismatch: 1 }],
  trend: [{ key: '2026-09-28T00:00:00Z', requests: 3, measured: 2, missing_actual: 1, unobserved: 1, pending: 1, input_tokens: 100, output_tokens: 50, cache_read_tokens: 20, cache_creation_tokens: 10, standard_cost_usd_nanos: 500, actual_cost_usd_nanos: 400, route_observed: 2, route_mismatch: 1 }],
}

const usage: AgentUsageView = {
  request_id: 'req-agent-only', usage_id: 'usage-1', proxy_main_user_id: '42', billing_main_user_id: '42',
  reserved_cents: 1, actual_cents: 1, settlement_status: 'confirmed', model: 'gpt-5.5', usage_source: 'sub2api_user_usage',
  input_tokens: 100, output_tokens: 50, cache_creation_tokens: 10, cache_read_tokens: 20,
  cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0, input_cost_usd_nanos: 0, output_cost_usd_nanos: 0,
  cache_creation_cost_usd_nanos: 0, cache_read_cost_usd_nanos: 0, total_cost_usd_nanos: 500,
  actual_cost_usd_nanos: 400, actual_cost_reported: true, rate_multiplier: 1, long_context_billing_applied: false,
  image_count: 0, image_input_tokens: 0, image_input_cost_usd_nanos: 0, image_output_tokens: 0, image_output_cost_usd_nanos: 0,
  billing_type: 0, openai_ws_mode: false, native_compaction_v2: false, duration_ms: 325, first_token_ms: 100,
  stream: true, cache_ttl_overridden: false, created_at: '2026-09-28T12:00:00Z',
}

describe('AgentOpsDashboard', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('ports the main ops layout using only agent-scoped usage facts', async () => {
    const getInsights = vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue(insights)
    const getUsage = vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({ items: [usage], total: 1, page: 1, page_size: 50 })
    const wrapper = mount(AgentOpsDashboard, {
      global: { stubs: { AgentTokenUsageTrend: true, AgentModelDistributionChart: true } },
    })
    await flushPromises()

    expect(getInsights).toHaveBeenCalledWith('24h')
    expect(getUsage).toHaveBeenCalledWith(1, 50, { start_time: insights.start, end_time: insights.end })
    expect(wrapper.text()).toContain('运营监控')
    expect(wrapper.text()).toContain('当前代理站范围')
    expect(wrapper.text()).toContain('req-agent-only')
    expect(wrapper.text()).toContain('50.0%')
    expect(wrapper.findAll('button').map(button => button.text())).not.toContain('告警规则')
    expect(wrapper.findAll('button').map(button => button.text())).not.toContain('运行参数')
    wrapper.unmount()
  })

  it('changes the exact window and keeps missing data explicit', async () => {
    vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue({ ...insights, status: 'empty', requests: 0, measured: 0, models: [], trend: [] })
    vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 50 })
    const wrapper = mount(AgentOpsDashboard, {
      global: { stubs: { AgentTokenUsageTrend: true, AgentModelDistributionChart: true } },
    })
    await flushPromises()
    await wrapper.get('[aria-label="运营监控时间范围"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('最近 7 天'))!.click()
    await flushPromises()
    expect(agentAPI.getUsageInsights).toHaveBeenLastCalledWith('7d')
    expect(wrapper.text()).toContain('暂无本站请求')
    wrapper.unmount()
  })

  it('uses the main-site auto-refresh menu for interval changes and pausing', async () => {
    vi.useFakeTimers()
    const getInsights = vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue(insights)
    vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({ items: [usage], total: 1, page: 1, page_size: 50 })
    const wrapper = mount(AgentOpsDashboard, {
      global: { stubs: { AgentTokenUsageTrend: true, AgentModelDistributionChart: true } },
    })
    await flushPromises()

    await wrapper.get('[aria-label="自动刷新设置"]').trigger('click')
    await wrapper.findAll('[role="menuitemradio"]')[0].trigger('click')
    expect(wrapper.get('[aria-label="自动刷新设置"]').text()).toContain('自动刷新：15 秒')
    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()
    expect(getInsights).toHaveBeenCalledTimes(2)

    await wrapper.get('[role="menuitemcheckbox"]').trigger('click')
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(getInsights).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
