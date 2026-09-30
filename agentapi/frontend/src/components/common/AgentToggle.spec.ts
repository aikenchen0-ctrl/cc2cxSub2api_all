import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentToggle from './AgentToggle.vue'

describe('AgentToggle', () => {
  it('exposes switch semantics and emits the next value', async () => {
    const wrapper = mount(AgentToggle, {
      props: { modelValue: false },
      attrs: { 'aria-label': '启用功能' },
    })

    const toggle = wrapper.get('[role="switch"]')
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(toggle.attributes('aria-label')).toBe('启用功能')
    await toggle.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
    expect(wrapper.emitted('change')).toEqual([[true]])
  })

  it('forwards native attributes and custom classes', () => {
    const wrapper = mount(AgentToggle, {
      props: { modelValue: true, id: 'feature-toggle' },
      attrs: { class: 'mt-1', 'data-testid': 'feature-toggle' },
    })

    const toggle = wrapper.get('button')
    expect(toggle.attributes('id')).toBe('feature-toggle')
    expect(toggle.attributes('data-testid')).toBe('feature-toggle')
    expect(toggle.classes()).toEqual(expect.arrayContaining(['mt-1', 'bg-primary-600']))
    expect(toggle.attributes('aria-checked')).toBe('true')
  })

  it('does not emit while disabled', async () => {
    const onClick = vi.fn()
    const wrapper = mount(AgentToggle, {
      props: { modelValue: false, disabled: true },
      attrs: { onClick },
    })

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('change')).toBeUndefined()
    expect(onClick).not.toHaveBeenCalled()
  })
})
