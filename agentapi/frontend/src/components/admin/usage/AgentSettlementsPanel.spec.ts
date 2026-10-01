import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentSettlementsPanel from './AgentSettlementsPanel.vue'

describe('AgentSettlementsPanel', () => {
  const items = [
    { request_id: 'req-pending', proxy_main_user_id: 'user-1', billing_main_user_id: 'user-1', reserved_cents: 20, actual_cents: 0, status: 'pending', model: 'gpt-5.5' },
    { request_id: 'req-confirmed', proxy_main_user_id: 'user-2', billing_main_user_id: 'user-2', reserved_cents: 20, actual_cents: 18, status: 'confirmed', model: 'gpt-image-2' },
  ]

  it('filters tenant settlement records and emits reconciliation', async () => {
    const wrapper = mount(AgentSettlementsPanel, { props: { items } })
    expect(wrapper.text()).toContain('req-pending')
    expect(wrapper.text()).toContain('req-confirmed')
    expect(wrapper.text()).toContain('待确认')

    await wrapper.get('#settlement-status').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('已同步'))!.click()
    await flushPromises()
    expect(wrapper.text()).not.toContain('req-pending')
    expect(wrapper.text()).toContain('req-confirmed')
    expect(wrapper.text()).toContain('¥0.18')

    await wrapper.findAll('button').find(button => button.text() === '核对待结算记录')!.trigger('click')
    expect(wrapper.emitted('reconcile')).toHaveLength(1)
  })
})
