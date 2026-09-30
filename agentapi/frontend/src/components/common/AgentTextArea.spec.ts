import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentTextArea from './AgentTextArea.vue'

describe('AgentTextArea', () => {
  it('emits model and change values from the native textarea', async () => {
    const wrapper = mount(AgentTextArea, { props: { modelValue: 'before' } })

    await wrapper.get('textarea').setValue('after')

    expect(wrapper.emitted('update:modelValue')).toEqual([['after']])
    expect(wrapper.emitted('change')).toEqual([['after']])
  })

  it('forwards native attributes, classes, and keyboard listeners', async () => {
    const onKeydown = vi.fn()
    const wrapper = mount(AgentTextArea, {
      props: { modelValue: '' },
      attrs: {
        id: 'prompt-input',
        maxlength: '500',
        'aria-label': '提示词',
        class: 'font-mono min-h-48',
        onKeydown,
      },
    })

    const textarea = wrapper.get('textarea')
    expect(textarea.attributes('id')).toBe('prompt-input')
    expect(textarea.attributes('maxlength')).toBe('500')
    expect(textarea.attributes('aria-label')).toBe('提示词')
    expect(textarea.classes()).toEqual(expect.arrayContaining(['input', 'font-mono', 'min-h-48']))

    await textarea.trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(onKeydown).toHaveBeenCalledOnce()
  })

  it('renders main-site field state and helper text', () => {
    const wrapper = mount(AgentTextArea, {
      props: {
        modelValue: '',
        label: '正文',
        required: true,
        disabled: true,
        error: '请输入正文',
        hint: '不会显示',
        id: 'body',
      },
    })

    expect(wrapper.get('label').attributes('for')).toBe('body')
    expect(wrapper.get('textarea').attributes()).toMatchObject({ disabled: '', required: '' })
    expect(wrapper.get('.input-error-text').text()).toBe('请输入正文')
    expect(wrapper.text()).not.toContain('不会显示')
  })
})
