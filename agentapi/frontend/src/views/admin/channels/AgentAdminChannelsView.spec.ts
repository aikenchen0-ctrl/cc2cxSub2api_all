import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentAdminChannelsView from './AgentAdminChannelsView.vue'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

const policy = { catalog: ['gpt-5.5', 'gpt-image-2'], enabled: ['gpt-5.5'], customized: true }

const global = {
  stubs: {
    AgentChannelsPanel: {
      props: ['mode', 'policy', 'enabled', 'saving', 'loadFailed'],
      emits: ['update:enabled', 'save'],
      template: '<div data-testid="channels-panel" :data-mode="mode"><span>{{ enabled.join(\",\") }}</span><button data-testid="select-image" @click="$emit(\'update:enabled\', [\'gpt-5.5\', \'gpt-image-2\'])">select</button><button data-testid="save" @click="$emit(\'save\')">save</button></div>',
    },
  },
}

describe('AgentAdminChannelsView', () => {
  afterEach(() => vi.restoreAllMocks())

  it.each([
    ['channels', '渠道管理', '模型允许列表'],
    ['pricing', '模型定价', '只读价格事实'],
  ] as const)('loads tenant context and policy before mounting the %s page', async (mode, title, capability) => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const policyReader = vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue(policy)
    const wrapper = mount(AgentAdminChannelsView, { props: { mode }, global })

    expect(policyReader).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="channels-panel"]').exists()).toBe(false)
    await flushPromises()

    expect(policyReader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain(title)
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain(capability)
    expect(wrapper.text()).toContain('不能创建渠道、管理账号池或读取渠道凭据')
    expect(wrapper.get('[data-testid="channels-panel"]').attributes('data-mode')).toBe(mode)
  })

  it('does not read model policy before tenant context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const policyReader = vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue(policy)
    const wrapper = mount(AgentAdminChannelsView, { props: { mode: 'channels' }, global })
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载渠道管理')
    expect(policyReader).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(policyReader).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="channels-panel"]').exists()).toBe(true)
  })

  it('saves only the tenant model allowlist through the confirmed runtime endpoint', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue(policy)
    const update = vi.spyOn(agentAPI, 'updateAdminModelPolicy').mockResolvedValue({ ...policy, enabled: ['gpt-5.5', 'gpt-image-2'] })
    const wrapper = mount(AgentAdminChannelsView, { props: { mode: 'channels' }, global })
    await flushPromises()

    await wrapper.get('[data-testid="select-image"]').trigger('click')
    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(update).toHaveBeenCalledWith(['gpt-5.5', 'gpt-image-2'])
    expect(wrapper.text()).toContain('本站模型策略已保存')
  })

  it('never writes pricing facts from the read-only pricing route', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue(policy)
    const update = vi.spyOn(agentAPI, 'updateAdminModelPolicy')
    const wrapper = mount(AgentAdminChannelsView, { props: { mode: 'pricing' }, global })
    await flushPromises()

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(update).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('不提供本地价格覆盖或倍率编辑')
  })
})
