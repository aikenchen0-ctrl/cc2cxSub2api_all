import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentModelPolicyPanel from './AgentModelPolicyPanel.vue'

describe('AgentModelPolicyPanel', () => {
  it('filters the public catalog and emits a tenant-local selection', async () => {
    const wrapper = mount(AgentModelPolicyPanel, {
      props: {
        policy: { catalog: ['gpt-5.5', 'gpt-image-2', 'claude-sonnet-4'], enabled: ['gpt-5.5'], customized: true },
        enabled: ['gpt-5.5'],
      },
    })

    await wrapper.get('#model-policy-search').setValue('image')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).not.toContain('claude-sonnet-4')

    const selectVisible = wrapper.findAll('button').find(button => button.text() === '选择当前筛选')
    await selectVisible!.trigger('click')
    expect(wrapper.emitted('update:enabled')?.at(-1)?.[0]).toEqual(['gpt-5.5', 'gpt-image-2'])

    await wrapper.findAll('button').find(button => button.text() === '保存模型权限')!.trigger('click')
    expect(wrapper.emitted('save')).toHaveLength(1)
  })
})
