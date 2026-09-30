import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentDateRangePicker from './AgentDateRangePicker.vue'

describe('AgentDateRangePicker', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('copies the main-site presets and applies one range atomically', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-29T12:00:00+08:00'))
    const wrapper = mount(AgentDateRangePicker, {
      props: { startDate: '', endDate: '', ariaLabel: '推广记录日期范围' },
      attachTo: document.body,
    })

    await wrapper.get('[aria-label="推广记录日期范围"]').trigger('click')
    expect(wrapper.get('[role="dialog"]').attributes('aria-label')).toBe('推广记录日期范围面板')
    await wrapper.findAll('button').find((button) => button.text() === '近 7 天')!.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === '应用')!.trigger('click')

    expect(wrapper.emitted('update:startDate')?.at(-1)).toEqual(['2026-09-23'])
    expect(wrapper.emitted('update:endDate')?.at(-1)).toEqual(['2026-09-29'])
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ startDate: '2026-09-23', endDate: '2026-09-29', preset: '7days' }])
    expect(wrapper.get('[aria-label="推广记录日期范围"]').attributes('aria-expanded')).toBe('false')
    wrapper.unmount()
  })

  it('validates custom ranges, clears them and closes on Escape', async () => {
    const wrapper = mount(AgentDateRangePicker, {
      props: { startDate: '2026-09-20', endDate: '2026-09-29' },
      attachTo: document.body,
    })
    await wrapper.get('[aria-label="选择日期范围"]').trigger('click')
    await wrapper.get('[aria-label="日期范围开始日期"]').setValue('2026-09-30')
    await wrapper.get('[aria-label="日期范围结束日期"]').setValue('2026-09-29')
    expect(wrapper.get('[role="alert"]').text()).toContain('开始日期不能晚于结束日期')
    expect(wrapper.findAll('button').find((button) => button.text() === '应用')!.attributes('disabled')).toBeDefined()

    await wrapper.findAll('button').find((button) => button.text() === '清除')!.trigger('click')
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ startDate: '', endDate: '', preset: null }])
    await wrapper.setProps({ startDate: '', endDate: '' })
    await wrapper.get('[aria-label="选择日期范围"]').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.get('[aria-label="选择日期范围"]').attributes('aria-expanded')).toBe('false')
    wrapper.unmount()
  })
})
