import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentChannelsPanel from './AgentChannelsPanel.vue'

const channelPayload = {
  channels: [{
    name: 'Primary',
    description: '主站公开渠道',
    platforms: [{
      platform: 'openai',
      groups: [{
        id: 7,
        name: 'standard',
        platform: 'openai',
        subscription_type: 'standard',
        rate_multiplier: 1,
        peak_rate_enabled: false,
        peak_start: '',
        peak_end: '',
        peak_rate_multiplier: 1,
        is_exclusive: false,
      }],
      supported_models: [
        { name: 'gpt-5.5', platform: 'openai', pricing: { billing_mode: 'token', input_price: 0.000001, output_price: 0.000004 } },
        { name: 'gpt-image-2', platform: 'openai', pricing: { billing_mode: 'per_request', per_request_price: 0.08 } },
      ],
    }],
  }],
  user_group_rates: { 7: 1 },
}

afterEach(() => vi.restoreAllMocks())

describe('AgentChannelsPanel', () => {
  it('copies the channel table while editing only the tenant model allowlist', async () => {
    vi.spyOn(agentAPI, 'getAdminChannels').mockResolvedValue(channelPayload)
    const wrapper = mount(AgentChannelsPanel, {
      props: {
        mode: 'channels',
        policy: { catalog: ['gpt-5.5', 'gpt-image-2'], enabled: ['gpt-5.5'], customized: true },
        enabled: ['gpt-5.5'],
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-channels-table"]').text()).toContain('Primary')
    expect(wrapper.findAll('[data-platform="openai"]').length).toBeGreaterThan(0)
    expect(wrapper.text()).toContain('standard')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).toContain('不会修改主站全局渠道、账号或定价')
    expect(wrapper.text()).not.toContain('创建渠道')
    expect(wrapper.text()).not.toContain('删除渠道')

    await wrapper.get('[data-testid="toggle-gpt-image-2"]').trigger('click')
    expect(wrapper.emitted('update:enabled')?.at(-1)?.[0]).toEqual(['gpt-5.5', 'gpt-image-2'])
    await wrapper.get('[data-testid="save-channel-policy"]').trigger('click')
    expect(wrapper.emitted('save')).toHaveLength(1)

    await wrapper.get('[aria-label="搜索管理渠道"]').setValue('image')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).not.toContain('gpt-5.5')
    await wrapper.get('[aria-label="筛选本站状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('本站未启用'))!.click()
    await flushPromises()
    expect(wrapper.text()).toContain('gpt-image-2')
  })

  it('renders main-site prices as read-only facts', async () => {
    vi.spyOn(agentAPI, 'getAdminChannels').mockResolvedValue(channelPayload)
    const wrapper = mount(AgentChannelsPanel, {
      props: {
        mode: 'pricing',
        policy: { catalog: ['gpt-5.5', 'gpt-image-2'], enabled: ['gpt-5.5'], customized: false },
        enabled: ['gpt-5.5'],
      },
    })
    await flushPromises()

    const table = wrapper.get('[data-testid="admin-pricing-table"]')
    expect(table.findAll('[data-platform="openai"]').length).toBeGreaterThan(0)
    expect(table.text()).toContain('$1')
    expect(table.text()).toContain('$4')
    expect(table.text()).toContain('$0.08')
    expect(table.text()).toContain('按次')
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('编辑价格')
    expect(wrapper.find('[data-testid="save-channel-policy"]').exists()).toBe(false)
  })

  it('shows a retry state when the read-only main-site lookup fails', async () => {
    const request = vi.spyOn(agentAPI, 'getAdminChannels')
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(channelPayload)
    const wrapper = mount(AgentChannelsPanel, {
      props: {
        mode: 'channels',
        policy: { catalog: [], enabled: [], customized: false },
        enabled: [],
      },
    })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('加载主站渠道资料失败')
    await wrapper.findAll('button').find(button => button.text() === '重新加载')!.trigger('click')
    await flushPromises()
    expect(request).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('gpt-5.5')
  })
})
