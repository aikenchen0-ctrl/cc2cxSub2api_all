import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentModelDistributionChart from './AgentModelDistributionChart.vue'
import AgentTokenUsageTrend from './AgentTokenUsageTrend.vue'

vi.mock('vue-chartjs', () => ({
  Doughnut: { template: '<div data-testid="doughnut-chart" />' },
  Line: { template: '<div data-testid="line-chart" />' },
}))

const base = {
  requests: 1, measured: 1, missing_actual: 0, unobserved: 0, pending: 0,
  input_tokens: 1000, output_tokens: 200, cache_read_tokens: 50, cache_creation_tokens: 25,
  standard_cost_usd_nanos: 30_000_000, actual_cost_usd_nanos: 0,
  route_observed: 1, route_mismatch: 0,
}

describe('copied usage charts', () => {
  it('treats a reported zero actual cost as data and switches metric', async () => {
    const wrapper = mount(AgentModelDistributionChart, { props: { modelStats: [{ ...base, key: 'gpt-5.5' }], metric: 'tokens' } })
    expect(wrapper.text()).toContain('$0.0000')
    expect(wrapper.find('[data-testid="doughnut-chart"]').exists()).toBe(true)
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('update:metric')).toEqual([['actual_cost']])
  })

  it('does not draw a trend when every exact-window bucket is unobserved', () => {
    const wrapper = mount(AgentTokenUsageTrend, { props: { trendData: [{ ...base, key: '2026-09-29T00:00:00Z', measured: 0, unobserved: 1, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0 }] } })
    expect(wrapper.find('[data-testid="line-chart"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无已观测令牌数据')
  })
})
