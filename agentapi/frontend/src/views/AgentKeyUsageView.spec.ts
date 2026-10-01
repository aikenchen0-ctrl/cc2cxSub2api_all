import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentKeyUsageView from './AgentKeyUsageView.vue'

const result = {
  mode: 'unrestricted', isValid: true, planName: '主站计费', balance: 12.34, remaining: 12.34,
  key: { name: '开发 Key', prefix: 'sk-example' }, status_detail: 'measured',
  usage: {
    today: { requests: 1, input_tokens: 100, output_tokens: 20, total_tokens: 120, cache_creation_tokens: 0, cache_read_tokens: 0, actual_cost: 0.01 },
    total: { requests: 2, input_tokens: 200, output_tokens: 40, total_tokens: 240, cache_creation_tokens: 0, cache_read_tokens: 0, actual_cost: 0.02 }, rpm: 0, tpm: 0,
  },
  daily_usage: [{ date: '2026-09-29', requests: 1, input_tokens: 100, output_tokens: 20, total_tokens: 120, cache_creation_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, cost: 0.01, actual_cost: 0.01 }],
  model_stats: [{ model: 'gpt-5.5', requests: 2, input_tokens: 200, output_tokens: 40, total_tokens: 240, cache_creation_tokens: 0, cache_read_tokens: 0, actual_cost: 0.02 }],
}

function render() {
  return mount(AgentKeyUsageView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
}

afterEach(() => vi.restoreAllMocks())

describe('AgentKeyUsageView', () => {
  it('queries the same-origin usage route with the AgentAPI key and renders main-style details', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(result), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const wrapper = render()
    await wrapper.get('input[placeholder="sk-..."]').setValue('sk-secret-value')
    await wrapper.get('button.bg-primary-600').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, options] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/v1/usage?')
    expect((options?.headers as Record<string, string>).Authorization).toBe('Bearer sk-secret-value')
    expect(wrapper.text()).toContain('账户余额')
    expect(wrapper.text()).toContain('$12.3400')
    expect(wrapper.text()).toContain('gpt-5.5')
    expect(wrapper.text()).not.toContain('sk-secret-value')

    const customRange = wrapper.findAll('button').find(button => button.text() === '自定义')
    expect(customRange).toBeDefined()
    await customRange!.trigger('click')
    await wrapper.get('[aria-label="Key 用量自定义日期范围"]').trigger('click')
    await wrapper.get('[aria-label="日期范围开始日期"]').setValue('2026-09-01')
    await wrapper.get('[aria-label="日期范围结束日期"]').setValue('2026-09-29')
    const applyRange = wrapper.findAll('[role="dialog"] button').find(button => button.text() === '应用')
    expect(applyRange).toBeDefined()
    await applyRange!.trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(String(fetchMock.mock.calls[1][0])).toContain('start_date=2026-09-01')
    expect(String(fetchMock.mock.calls[1][0])).toContain('end_date=2026-09-29')
  })

  it('keeps failed responses out of the result area', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ message: 'AgentAPI API key is invalid or revoked' }), { status: 401 }))
    const wrapper = render()
    await wrapper.get('input[placeholder="sk-..."]').setValue('sk-bad')
    await wrapper.get('button.bg-primary-600').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalid or revoked')
    expect(wrapper.text()).not.toContain('账户余额')
  })
})
