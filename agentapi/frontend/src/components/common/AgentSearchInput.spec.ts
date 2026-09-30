import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentSearchInput from './AgentSearchInput.vue'

describe('AgentSearchInput', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('updates the model immediately and emits one debounced search', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AgentSearchInput, {
      props: { modelValue: '', placeholder: '搜索用户', debounceMs: 300 },
    })

    const input = wrapper.get('input')
    await input.setValue('a')
    await input.setValue('alice')

    expect(wrapper.emitted('update:modelValue')).toEqual([['a'], ['alice']])
    expect(wrapper.emitted('search')).toBeUndefined()

    vi.advanceTimersByTime(299)
    expect(wrapper.emitted('search')).toBeUndefined()
    vi.advanceTimersByTime(1)
    expect(wrapper.emitted('search')).toEqual([['alice']])
  })

  it('supports immediate searches for client-side filters', async () => {
    const wrapper = mount(AgentSearchInput, {
      props: { modelValue: '', debounceMs: 0, ariaLabel: '搜索记录' },
    })

    await wrapper.get('input').setValue('request-42')

    expect(wrapper.emitted('search')).toEqual([['request-42']])
    expect(wrapper.get('input').attributes('aria-label')).toBe('搜索记录')
  })

  it('forwards native input attributes and styling', () => {
    const wrapper = mount(AgentSearchInput, {
      props: { modelValue: '', ariaLabel: '搜索关联用户' },
      attrs: { id: 'user-search', maxlength: '128', class: 'tenant-search' },
    })

    const input = wrapper.get('input')
    expect(input.attributes('id')).toBe('user-search')
    expect(input.attributes('maxlength')).toBe('128')
    expect(input.classes()).toContain('tenant-search')
  })

  it('supports an accessible clear action without waiting for debounce', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AgentSearchInput, {
      props: {
        modelValue: 'gpt-image',
        clearable: true,
        clearAriaLabel: '清空模型搜索',
      },
    })

    await wrapper.get('button[aria-label="清空模型搜索"]').trigger('click')
    vi.runAllTimers()

    expect(wrapper.emitted('update:modelValue')).toEqual([['']])
    expect(wrapper.emitted('search')).toEqual([['']])
  })

  it('cancels a pending search when the parent resets the value', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AgentSearchInput, {
      props: { modelValue: '', debounceMs: 300 },
    })

    await wrapper.get('input').setValue('stale')
    await wrapper.setProps({ modelValue: 'stale' })
    await wrapper.setProps({ modelValue: '' })
    vi.runAllTimers()

    expect(wrapper.emitted('search')).toBeUndefined()
  })

  it('clears pending timers when unmounted', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AgentSearchInput, {
      props: { modelValue: '', debounceMs: 300 },
    })

    await wrapper.get('input').setValue('tenant-safe')
    wrapper.unmount()
    vi.runAllTimers()

    expect(wrapper.emitted('search')).toBeUndefined()
  })
})
