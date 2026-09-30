import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentGroupBadge from './AgentGroupBadge.vue'

describe('AgentGroupBadge', () => {
  it('renders a main-style standard group with a user-specific rate', () => {
    const wrapper = mount(AgentGroupBadge, {
      props: { groupId: 7, name: '标准组', platform: 'openai', rateMultiplier: 1, userRateMultiplier: 0.8 },
    })

    const badge = wrapper.get('[data-group-id="7"]')
    expect(badge.attributes('data-group-name')).toBe('标准组')
    expect(badge.attributes('data-platform')).toBe('openai')
    expect(badge.text()).toContain('标准组')
    expect(badge.text()).toContain('1x')
    expect(badge.text()).toContain('0.8x')
    expect(badge.get('.line-through').text()).toBe('1x')
  })

  it('renders subscription and peak-rate semantics without guessing a timezone', () => {
    const wrapper = mount(AgentGroupBadge, {
      props: {
        name: 'Gemini Pro', platform: 'gemini', subscriptionType: 'subscription', rateMultiplier: 1.5,
        peakRateEnabled: true, peakStart: '09:00', peakEnd: '18:00', peakRateMultiplier: 2,
      },
    })

    expect(wrapper.text()).toContain('订阅')
    expect(wrapper.text()).toContain('高峰 09:00–18:00 · 2x')
    expect(wrapper.text()).not.toContain('UTC')
  })

  it('marks exclusive and unknown-platform groups safely', () => {
    const exclusive = mount(AgentGroupBadge, { props: { name: '专属组', platform: 'vendor-x', exclusive: true, showRate: false } })
    expect(exclusive.get('[data-exclusive="true"]').text()).toBe('专属组')

    const fallback = mount(AgentGroupBadge, { props: { name: '未知组', platform: 'vendor-x', showRate: false } })
    expect(fallback.get('[data-platform="vendor-x"]').classes()).toContain('bg-gray-100')
  })
})
