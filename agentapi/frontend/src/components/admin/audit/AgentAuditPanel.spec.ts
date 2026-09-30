import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import AgentAuditPanel from './AgentAuditPanel.vue'

describe('AgentAuditPanel', () => {
  afterEach(() => { document.body.innerHTML = '' })

  it('filters recent tenant events and opens the copied detail layout', async () => {
    const wrapper = mount(AgentAuditPanel, {
      props: {
        events: [
          { id: 1, actor_type: 'agent_admin', actor_id: 'owner-1', agent_id: 'agent-1', operation: 'branding.update', target_type: 'agent', target_id: 'agent-1', request_id: 'req-brand', result: 'success', created_at: '2026-09-29T00:00:00Z' },
          { id: 2, actor_type: 'system', actor_id: 'worker', agent_id: 'agent-1', operation: 'settlements.reconcile', target_type: 'settlement', target_id: 'req-usage', request_id: 'req-audit', result: 'failed', reason: '主站暂不可用', created_at: '2026-09-29T01:00:00Z' },
        ],
      },
      attachTo: document.body,
    })

    await wrapper.get('#audit-search').setValue('reconcile')
    expect(wrapper.text()).not.toContain('req-brand')
    expect(wrapper.text()).toContain('req-audit')

    await wrapper.findAll('button').find(button => button.text() === '查看')!.trigger('click')
    expect(document.body.textContent).toContain('操作日志详情')
    expect(document.body.textContent).toContain('主站暂不可用')
    wrapper.unmount()
  })
})
