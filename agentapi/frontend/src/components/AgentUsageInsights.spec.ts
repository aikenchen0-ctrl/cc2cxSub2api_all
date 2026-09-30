import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentUsageInsights from './AgentUsageInsights.vue'
const mocks = vi.hoisted(() => ({ getUsageInsights: vi.fn() }))
vi.mock('@/agent/api', () => ({ agentAPI: mocks }))
vi.mock('vue-chartjs', () => ({
  Doughnut: { template: '<div data-testid="doughnut-chart" />' },
  Line: { template: '<div data-testid="line-chart" />' },
}))

const result = { status: 'partial', scope: 'user', start: '2026-09-27T00:00:00Z', end: '2026-09-28T00:00:00Z', requests: 2, measured: 1, missing_actual: 1, unobserved: 1, pending: 1, actual_cost_usd_nanos: 0, standard_cost_usd_nanos: 123, route_observed: 0, route_mismatch: 0, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, models: [], trend: [] }
describe('AgentUsageInsights', () => {
  beforeEach(() => vi.clearAllMocks())
  it('labels incomplete amounts and missing actual cost', async () => {
    mocks.getUsageInsights.mockResolvedValue(result)
    const wrapper = mount(AgentUsageInsights)
    await flushPromises()
    expect(wrapper.get('[data-status="partial"]').text()).toBe('部分可用')
    expect(wrapper.text()).toContain('部分可用')
    expect(wrapper.text()).toContain('不代表完整总额')
    expect(wrapper.text()).toContain('无数据')
    expect(wrapper.text()).toContain('请求总数')
    expect(wrapper.text()).toContain('模型分布')
    expect(wrapper.text()).toContain('令牌用量趋势')
    expect(mocks.getUsageInsights).toHaveBeenCalledWith('24h')
  })
  it('does not display zero totals after failed reads', async () => {
    mocks.getUsageInsights.mockRejectedValue(new Error('offline'))
    const wrapper = mount(AgentUsageInsights)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('不以零代替缺失数据')
    expect(wrapper.text()).not.toContain('请求总数')
  })
  it('ignores a stale response from the previous window', async () => {
    let resolveOld!: (value: unknown) => void
    mocks.getUsageInsights.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    mocks.getUsageInsights.mockResolvedValueOnce({ ...result, requests: 77 })
    const wrapper = mount(AgentUsageInsights)
    await wrapper.get('[aria-label="统计窗口"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('最近 7 天'))!.click()
    await flushPromises()
    resolveOld({ ...result, requests: 99 })
    await flushPromises()
    expect(wrapper.text()).toContain('77')
    expect(wrapper.text()).not.toContain('99')
  })

  it('renders model and trend charts from measured snapshot facts', async () => {
    const row = {
      key: 'gpt-5.5', requests: 1, measured: 1, missing_actual: 0, unobserved: 0, pending: 0,
      input_tokens: 1200, output_tokens: 300, cache_read_tokens: 100, cache_creation_tokens: 50,
      standard_cost_usd_nanos: 30_000_000, actual_cost_usd_nanos: 0,
      route_observed: 1, route_mismatch: 0,
    }
    mocks.getUsageInsights.mockResolvedValue({
      ...result,
      status: 'measured', requests: 1, measured: 1, missing_actual: 0, unobserved: 0, pending: 0,
      input_tokens: 1200, output_tokens: 300, cache_read_tokens: 100, cache_creation_tokens: 50,
      standard_cost_usd_nanos: 30_000_000, actual_cost_usd_nanos: 0,
      route_observed: 1, route_mismatch: 0,
      models: [row],
      trend: [{ ...row, key: '2026-09-27T00:00:00Z' }],
    })
    const wrapper = mount(AgentUsageInsights)
    await flushPromises()
    expect(wrapper.find('[data-testid="doughnut-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="line-chart"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('gpt-5.5')
    expect(wrapper.text()).toContain('$0.0000')
    expect(wrapper.text()).not.toContain('实际费用\n          无数据')
  })
})
