import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentAdminProvisioning, type AgentContextResponse } from '@/agent/api'
import AgentAdminProvisioningView from './AgentAdminProvisioningView.vue'

const context: AgentContextResponse = {
  agent: { agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active', billing_mode: 'user_upstream', owner_main_user_id: '42', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0 },
  authenticated: true,
  is_agent_admin: true,
}

const provisioning: AgentAdminProvisioning = {
  agent_id: 'agent-1',
  domain: 'agent.example.com',
  display_name: 'Example Agent',
  site_name: 'Example Site',
  owner_main_user_id: '42',
  configured_status: 'active',
  runtime_status: 'active',
  control_enabled: true,
  control_available: true,
  ready: true,
  last_checked_at: '2026-09-29T12:00:00Z',
  stale_after_seconds: 90,
  billing_mode: 'user_upstream',
  satellite_slug: 'agentapi',
  lifecycle_authority: 'sub2api_main',
  management_scope: 'current_agent_read_only',
}

describe('AgentAdminProvisioningView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before loading the current Agent lifecycle', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const reader = vi.spyOn(agentAPI.adminProvisioning, 'get').mockResolvedValue(provisioning)
    const wrapper = mount(AgentAdminProvisioningView)

    expect(reader).not.toHaveBeenCalled()
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(reader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain('agent.example.com')
    expect(wrapper.text()).toContain('当前代理站只读')
    expect(wrapper.text()).toContain('Sub2API 主站')
  })

  it('stops before the provisioning request when tenant context fails and retries safely', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext').mockRejectedValueOnce(new Error('internal URL')).mockResolvedValueOnce(context)
    const reader = vi.spyOn(agentAPI.adminProvisioning, 'get').mockResolvedValue(provisioning)
    const wrapper = mount(AgentAdminProvisioningView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载代理站开通状态')
    expect(wrapper.text()).not.toContain('internal URL')
    expect(reader).not.toHaveBeenCalled()

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(reader).toHaveBeenCalledOnce()
  })

  it('does not expose main-site global lifecycle mutations or credentials', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI.adminProvisioning, 'get').mockResolvedValue(provisioning)
    const wrapper = mount(AgentAdminProvisioningView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('创建代理站')
    expect(wrapper.text()).not.toContain('激活代理站')
    expect(wrapper.text()).not.toContain('暂停代理站')
    expect(wrapper.text()).not.toContain('撤销代理站')
    expect(wrapper.text()).not.toContain('runtime-control-secret')
    expect(wrapper.findAll('button')).toHaveLength(1)
    expect(wrapper.get('button').text()).toContain('刷新')
  })
})
