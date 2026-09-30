import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentInput from './AgentInput.vue'

describe('AgentInput', () => {
  it('emits text and change values from the native input', async () => {
    const wrapper = mount(AgentInput, { props: { modelValue: 'before' } })
    await wrapper.get('input').setValue('after')
    expect(wrapper.emitted('update:modelValue')).toEqual([['after']])
    expect(wrapper.emitted('change')).toEqual([['after']])
  })

  it('preserves trim and number v-model modifiers', async () => {
    const wrapper = mount(AgentInput, {
      props: { modelValue: 0, modelModifiers: { trim: true, number: true } },
    })
    await wrapper.get('input').setValue(' 42 ')
    expect(wrapper.emitted('update:modelValue')).toEqual([[42]])
  })

  it('forwards native attributes, classes, and keyboard listeners', async () => {
    const onKeydown = vi.fn()
    const wrapper = mount(AgentInput, {
      props: { modelValue: '' },
      attrs: {
        id: 'request-id',
        maxlength: '256',
        'aria-label': '请求编号',
        class: 'font-mono min-w-48',
        onKeydown,
      },
    })
    const input = wrapper.get('input')
    expect(input.attributes('id')).toBe('request-id')
    expect(input.attributes('maxlength')).toBe('256')
    expect(input.attributes('aria-label')).toBe('请求编号')
    expect(input.classes()).toEqual(expect.arrayContaining(['input', 'font-mono', 'min-w-48']))
    await input.trigger('keydown', { key: 'Enter' })
    expect(onKeydown).toHaveBeenCalledOnce()
  })

  it('renders slots and main-site field state', () => {
    const wrapper = mount(AgentInput, {
      props: { modelValue: '', label: '邮箱', required: true, disabled: true, error: '请输入邮箱', id: 'email' },
      slots: { prefix: '<span data-testid="prefix">@</span>', suffix: '<span data-testid="suffix">清除</span>' },
    })
    expect(wrapper.get('label').attributes('for')).toBe('email')
    expect(wrapper.get('input').attributes()).toMatchObject({ disabled: '', required: '' })
    expect(wrapper.get('input').classes()).toEqual(expect.arrayContaining(['pl-11', 'pr-11', 'input-error']))
    expect(wrapper.get('[data-testid="prefix"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="suffix"]').exists()).toBe(true)
    expect(wrapper.get('.input-error-text').text()).toBe('请输入邮箱')
  })

  it('exposes native email type validation without leaking the input element', async () => {
    const wrapper = mount(AgentInput, {
      props: { modelValue: '', type: 'email' },
    })
    await wrapper.get('input').setValue('not-an-email')
    expect((wrapper.vm as unknown as { hasTypeMismatch: () => boolean }).hasTypeMismatch()).toBe(true)
  })
})
