import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentAdminAuditView from './AgentAdminAuditView.vue'

const context = {
  agent: {
    agent_id: 'agent-1', domain: 'agent.example.com', name: 'Example Agent', site_name: 'Example Site', status: 'active',
    billing_mode: 'user_upstream', owner_main_user_id: 'owner-1', main_balance_cents: 0, billing_status: 'ok', wallet_available_cents: 0, wallet_allocated_cents: 0,
  },
  authenticated: true,
  is_agent_admin: true,
}

const events = [
  { id: 1, actor_type: 'agent_admin', actor_id: 'owner-1', agent_id: 'agent-1', operation: 'branding.update', target_type: 'agent', target_id: 'agent-1', request_id: 'req-brand', result: 'success', created_at: '2026-09-29T00:00:00Z' },
]

describe('AgentAdminAuditView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('confirms tenant context before loading current-agent audit events', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const auditReader = vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ items: events, total: 1 })
    const wrapper = mount(AgentAdminAuditView)

    expect(auditReader).not.toHaveBeenCalled()
    await flushPromises()

    expect(agentAPI.getContext).toHaveBeenCalledOnce()
    expect(auditReader).toHaveBeenCalledWith(500)
    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain('req-brand')
    expect(wrapper.text()).toContain('仅当前 agent_id')
    expect(wrapper.text()).toContain('不允许站长删除或篡改记录')
    expect(wrapper.text()).not.toContain('SuperKey:')
  })

  it('does not read audit events before context succeeds on retry', async () => {
    const contextReader = vi.spyOn(agentAPI, 'getContext')
      .mockRejectedValueOnce(new Error('context unavailable'))
      .mockResolvedValueOnce(context)
    const auditReader = vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ items: events, total: 1 })
    const wrapper = mount(AgentAdminAuditView)
    await flushPromises()

    expect(wrapper.text()).toContain('无法加载操作日志')
    expect(auditReader).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(contextReader).toHaveBeenCalledTimes(2)
    expect(auditReader).toHaveBeenCalledWith(500)
    expect(wrapper.text()).toContain('req-brand')
  })

  it('shows a recoverable tenant-safe error and refreshes the audit reader', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const auditReader = vi.spyOn(agentAPI, 'getAuditEvents')
      .mockRejectedValueOnce(new Error('upstream unavailable'))
      .mockResolvedValueOnce({ items: events, total: 1 })
    const wrapper = mount(AgentAdminAuditView)
    await flushPromises()

    expect(wrapper.text()).toContain('本站操作日志加载失败')
    expect(wrapper.text()).not.toContain('upstream unavailable')
    await wrapper.get('[aria-label="刷新操作日志"]').trigger('click')
    await flushPromises()

    expect(auditReader).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('req-brand')
  })
})
