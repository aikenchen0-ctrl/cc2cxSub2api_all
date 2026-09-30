import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import AgentAutoRefreshButton from './AgentAutoRefreshButton.vue'

describe('AgentAutoRefreshButton', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('copies the main-site countdown menu and emits explicit settings', async () => {
    const wrapper = mount(AgentAutoRefreshButton, {
      attachTo: document.body,
      props: { enabled: true, intervalSeconds: 30, countdown: 18, intervals: [15, 30, 60] },
    })

    expect(wrapper.get('[aria-label="自动刷新设置"]').text()).toContain('自动刷新：18 秒')
    await wrapper.get('[aria-label="自动刷新设置"]').trigger('click')
    expect(wrapper.get('[role="menu"]').text()).toContain('启用自动刷新')
    expect(wrapper.findAll('[role="menuitemradio"]')).toHaveLength(3)

    await wrapper.get('[role="menuitemcheckbox"]').trigger('click')
    expect(wrapper.emitted('update:enabled')).toEqual([[false]])
    await wrapper.findAll('[role="menuitemradio"]')[2].trigger('click')
    expect(wrapper.emitted('update:interval')).toEqual([[60]])
    wrapper.unmount()
  })

  it('closes on Escape and outside clicks', async () => {
    const wrapper = mount(AgentAutoRefreshButton, {
      attachTo: document.body,
      props: { enabled: false, intervalSeconds: 60, countdown: 60, intervals: [30, 60] },
    })

    await wrapper.get('[aria-label="自动刷新设置"]').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)

    await wrapper.get('[aria-label="自动刷新设置"]').trigger('click')
    document.body.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
