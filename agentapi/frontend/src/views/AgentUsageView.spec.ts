import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentUsageView from './AgentUsageView.vue'
import { agentAPI, type AgentUsageView as UsageItem } from '@/agent/api'

describe('AgentUsageView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows Sub2API token/cost provenance separately from local settlement and labels legacy fallback', async () => {
    const usage: UsageItem[] = [{
      request_id: 'req-authoritative',
      usage_id: 'usage-123',
      proxy_main_user_id: 'user-1',
      billing_main_user_id: 'owner-1',
      reserved_cents: 50,
      actual_cents: 12,
      settlement_status: 'confirmed',
      model: 'gpt-5.5',
      usage_source: 'sub2api_owner_runtime_usage',
      input_tokens: 1000,
      output_tokens: 250,
      cache_creation_tokens: 40,
      cache_read_tokens: 120,
      cache_creation_5m_tokens: 30,
      cache_creation_1h_tokens: 10,
      input_cost_usd_nanos: 1234,
      output_cost_usd_nanos: 5678,
      cache_creation_cost_usd_nanos: 910,
      cache_read_cost_usd_nanos: 1112,
      total_cost_usd_nanos: 150_000_000,
      actual_cost_usd_nanos: 120_000_000,
      actual_cost_reported: true,
      rate_multiplier: 1,
      long_context_billing_applied: false,
      image_count: 0,
      image_input_tokens: 0,
      image_input_cost_usd_nanos: 0,
      image_output_tokens: 0,
      image_output_cost_usd_nanos: 0,
      upstream_model: 'provider-gpt-5.5',
      upstream_response_model: 'provider-gpt-5.5-2026-01',
      upstream_model_mismatch: true,
      service_tier: 'priority',
      reasoning_effort: 'high',
      inbound_endpoint: '/v1/responses',
      request_type: 'text',
      billing_mode: 'token',
      billing_type: 0,
      openai_ws_mode: false,
      native_compaction_v2: false,
      duration_ms: 900,
      first_token_ms: 120,
      stream: false,
      cache_ttl_overridden: false,
      created_at: '2026-09-22T00:00:00Z',
    }, {
      request_id: 'req-legacy-admin-usage-label',
      proxy_main_user_id: 'user-1',
      billing_main_user_id: 'owner-1',
      reserved_cents: 50,
      actual_cents: 4,
      settlement_status: 'confirmed',
      usage_source: 'sub2api_admin_usage',
      input_tokens: 2,
      output_tokens: 3,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_5m_tokens: 0,
      cache_creation_1h_tokens: 0,
      input_cost_usd_nanos: 0,
      output_cost_usd_nanos: 0,
      cache_creation_cost_usd_nanos: 0,
      cache_read_cost_usd_nanos: 0,
      total_cost_usd_nanos: 4_000_000,
      actual_cost_usd_nanos: 4_000_000,
      actual_cost_reported: true,
      rate_multiplier: 1,
      long_context_billing_applied: false,
      image_count: 0,
      image_input_tokens: 0,
      image_input_cost_usd_nanos: 0,
      image_output_tokens: 0,
      image_output_cost_usd_nanos: 0,
      billing_type: 0,
      openai_ws_mode: false,
      native_compaction_v2: false,
      duration_ms: 10,
      first_token_ms: 2,
      stream: false,
      cache_ttl_overridden: false,
      created_at: '2026-09-22T00:00:30Z',
    }, {
      request_id: 'req-fallback',
      proxy_main_user_id: 'user-1',
      billing_main_user_id: 'owner-1',
      reserved_cents: 50,
      actual_cents: 20,
      settlement_status: 'confirmed',
      usage_source: 'owner_balance_delta_fallback',
      input_tokens: 0,
      output_tokens: 0,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_5m_tokens: 0,
      cache_creation_1h_tokens: 0,
      input_cost_usd_nanos: 0,
      output_cost_usd_nanos: 0,
      cache_creation_cost_usd_nanos: 0,
      cache_read_cost_usd_nanos: 0,
      total_cost_usd_nanos: 0,
      actual_cost_usd_nanos: 0,
      actual_cost_reported: false,
      rate_multiplier: 0,
      long_context_billing_applied: false,
      image_count: 0,
      image_input_tokens: 0,
      image_input_cost_usd_nanos: 0,
      image_output_tokens: 0,
      image_output_cost_usd_nanos: 0,
      billing_type: 0,
      openai_ws_mode: false,
      native_compaction_v2: false,
      duration_ms: 0,
      first_token_ms: 0,
      stream: false,
      cache_ttl_overridden: false,
      created_at: '2026-09-22T00:01:00Z',
    }]
    const getUsage = vi.spyOn(agentAPI, 'getUsage').mockResolvedValue({ items: usage, total: 52, page: 1, page_size: 25 })

    const wrapper = mount(AgentUsageView)
    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(1, 25)
    expect(wrapper.text()).toContain('Sub2API 用量记录')
    expect(wrapper.text()).toContain('令牌 · 输入 1000 / 输出 250 / 缓存读取 120 / 缓存写入 40（5 分钟 30 / 1 小时 10）')
    expect(wrapper.text()).toContain('$0.12')
    expect(wrapper.text()).toContain('$0.15')
    expect(wrapper.text()).toContain('provider-gpt-5.5-2026-01')
    expect(wrapper.text()).toContain('/v1/responses')
    expect(wrapper.text()).toContain('推理强度 高')
    expect(wrapper.text()).toContain('上游模型与请求的模型不一致')
    expect(wrapper.text()).toContain('req-legacy-admin-usage-label')
    expect(wrapper.text()).toContain('主账户余额差额估算')
    expect(wrapper.text()).not.toContain('令牌 · 输入 0 / 输出 0 / 缓存读取 0 / 缓存写入 0')

    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(2, 25)

    await wrapper.get('[aria-label="每页显示条数"]').setValue('50')
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(1, 50)
  })
})
