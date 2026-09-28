import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentRechargeView from './AgentRechargeView.vue'

describe('AgentRechargeView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('links only to the configured HTTP(S) main-site recharge page', async () => {
    vi.spyOn(agentAPI, 'getPublicSettings').mockResolvedValue({
      site_name: 'Demo API',
      site_logo: '/logo.svg',
      recharge_url: 'https://main.example.com/purchase',
    })

    const wrapper = mount(AgentRechargeView)
    await flushPromises()

    const link = wrapper.get('a')
    expect(link.attributes('href')).toBe('https://main.example.com/purchase')
    expect(wrapper.text()).toContain('自己的 Sub2API 主站账户直接计费')
    expect(wrapper.text()).not.toContain('本地充值订单')
  })

  it('rejects unsafe recharge schemes instead of rendering a link', async () => {
    vi.spyOn(agentAPI, 'getPublicSettings').mockResolvedValue({
      site_name: 'Demo API',
      site_logo: '/logo.svg',
      recharge_url: 'javascript:alert(1)',
    })

    const wrapper = mount(AgentRechargeView)
    await flushPromises()

    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toContain('主站充值地址暂不可用')
  })
})
