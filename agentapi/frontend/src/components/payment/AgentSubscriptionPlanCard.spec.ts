import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { AgentCheckoutPlan } from '@/agent/api'
import AgentSubscriptionPlanCard from './AgentSubscriptionPlanCard.vue'

const plan: AgentCheckoutPlan = {
  id: 9, group_id: 3, group_name: 'Gemini Pro', group_platform: 'antigravity',
  name: '专业订阅', description: '套餐说明', price: 20, original_price: 25, currency: 'USD',
  validity_days: 1, validity_unit: 'month', features: ['权益1', '权益2', '权益3', '权益4', '权益5'],
  product_name: 'Pro', rate_multiplier: 0.8, daily_limit_usd: 10, weekly_limit_usd: 50, monthly_limit_usd: 100,
  peak_rate_enabled: true, peak_start: '09:00', peak_end: '18:00', peak_rate_multiplier: 2,
  supported_model_scopes: ['gemini_text', 'gemini_image'],
}

describe('AgentSubscriptionPlanCard', () => {
  it('ports the complete main-site quota, scope, currency, discount and feature presentation', async () => {
    const wrapper = mount(AgentSubscriptionPlanCard, { props: { plan } })
    for (const text of ['USD', '-20%', '每日额度', '$10', '每周额度', '$50', '每月额度', '$100', '×0.8', '09:00–18:00 · ×2', 'Gemini', 'Imagen', '权益5']) {
      expect(wrapper.text()).toContain(text)
    }
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[plan]])
    wrapper.unmount()
  })

  it('preserves absent rate facts and disables selection while checkout is busy', async () => {
    const wrapper = mount(AgentSubscriptionPlanCard, {
      props: { plan: { ...plan, rate_multiplier: undefined }, disabled: true },
    })
    expect(wrapper.text()).toContain('暂不可用')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('select')).toBeUndefined()
    wrapper.unmount()
  })
})
