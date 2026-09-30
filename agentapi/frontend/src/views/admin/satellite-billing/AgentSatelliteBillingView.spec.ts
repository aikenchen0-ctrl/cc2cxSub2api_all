import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentUsageView, type SettlementView, type UsageInsights } from '@/agent/api'
import AgentSatelliteBillingView from './AgentSatelliteBillingView.vue'

const metrics = { requests: 4, measured: 3, missing_actual: 1, unobserved: 1, pending: 1, input_tokens: 100, output_tokens: 50, cache_read_tokens: 20, cache_creation_tokens: 10, standard_cost_usd_nanos: 900_000_000, actual_cost_usd_nanos: 650_000_000, route_observed: 2, route_mismatch: 0 }
const insights: UsageInsights = {
  source: 'local_main_usage_snapshots', status: 'partial', scope: 'agent',
  start: '2026-08-30T00:00:00Z', end: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
  ...metrics, models: [{ key: 'gpt-5.5', ...metrics }], trend: [{ key: '2026-09-28T00:00:00Z', ...metrics }],
}
const usage: AgentUsageView = {
  request_id: 'req-agent-billing', usage_id: 'usage-1', proxy_main_user_id: '42', billing_main_user_id: '42', reserved_cents: 1, actual_cents: 1,
  settlement_status: 'confirmed', model: 'gpt-5.5', usage_source: 'sub2api_user_usage', input_tokens: 100, output_tokens: 50, cache_creation_tokens: 10, cache_read_tokens: 20,
  cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0, input_cost_usd_nanos: 0, output_cost_usd_nanos: 0, cache_creation_cost_usd_nanos: 0, cache_read_cost_usd_nanos: 0,
  total_cost_usd_nanos: 900_000_000, actual_cost_usd_nanos: 650_000_000, actual_cost_reported: true, rate_multiplier: 1, long_context_billing_applied: false,
  image_count: 0, image_input_tokens: 0, image_input_cost_usd_nanos: 0, image_output_tokens: 0, image_output_cost_usd_nanos: 0, billing_type: 0,
  openai_ws_mode: false, native_compaction_v2: false, duration_ms: 300, first_token_ms: 80, stream: true, cache_ttl_overridden: false, created_at: '2026-09-28T12:00:00Z',
}
const pending: SettlementView = {
  request_id: 'req-pending', usage_id: 'usage-pending', proxy_main_user_id: '42', billing_main_user_id: '42', reserved_cents: 1, actual_cents: 0,
  status: 'pending', error: '', model: 'gpt-5.5', created_at: '2026-09-28T13:00:00Z', updated_at: '2026-09-28T13:00:00Z',
}

describe('AgentSatelliteBillingView', () => {
  afterEach(() => vi.restoreAllMocks())

  function mockLoad(): void {
    vi.spyOn(agentAPI, 'getUsageInsights').mockResolvedValue(insights)
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ items: [pending], total: 1 })
    vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({ items: [usage], total: 1, page: 1, page_size: 25 })
  }

  it('ports the main billing layout as a tenant-only read-only settlement center', async () => {
    mockLoad()
    const wrapper = mount(AgentSatelliteBillingView)
    await flushPromises()

    expect(agentAPI.getUsageInsights).toHaveBeenCalledWith('30d')
    expect(agentAPI.getUsage).toHaveBeenCalledWith(1, 25, { start_time: insights.start, end_time: insights.end })
    expect(wrapper.text()).toContain('主站模型计费')
    expect(wrapper.text()).toContain('代理站管理员可以核对本站记录')
    expect(wrapper.text()).toContain('req-agent-billing')
    expect(wrapper.text()).toContain('$0.6500')
    expect(wrapper.text()).toContain('66.7%')
    expect(wrapper.text()).not.toContain('保存配置')
    expect(wrapper.text()).not.toContain('aicut/')
    expect(wrapper.text()).not.toContain('satellite-app-secret')
    wrapper.unmount()
  })

  it('changes the exact window and reconciles only through the existing scoped endpoint', async () => {
    mockLoad()
    const reconcile = vi.spyOn(agentAPI, 'reconcileSettlements').mockResolvedValue({ items: [], total: 1 })
    const wrapper = mount(AgentSatelliteBillingView)
    await flushPromises()

    await wrapper.get('[aria-label="代理站计费时间范围"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('最近 7 天'))!.click()
    await flushPromises()
    expect(agentAPI.getUsageInsights).toHaveBeenLastCalledWith('7d')

    await wrapper.get('button.btn-secondary.btn-sm').trigger('click')
    await flushPromises()
    expect(reconcile).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已核对 1 条待确认记录')
    wrapper.unmount()
  })
})
