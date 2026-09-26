import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { activate, create, list, resume, retry, revoke, stream, suspend } = vi.hoisted(() => ({
  activate: vi.fn(),
  create: vi.fn(),
  list: vi.fn(),
  resume: vi.fn(),
  retry: vi.fn(),
  revoke: vi.fn(),
  stream: vi.fn(),
  suspend: vi.fn()
}))

vi.mock('@/api/admin/agentProvisioning', () => ({
  activateAgentProvisioningAgent: activate,
  createAgentProvisioningAgent: create,
  listAgentProvisioningAgents: list,
  resumeAgentProvisioningAgent: resume,
  retryAgentProvisioningAgent: retry,
  revokeAgentProvisioningAgent: revoke,
  streamAgentProvisioningUpdates: stream,
  suspendAgentProvisioningAgent: suspend,
  default: { activate, create, list, resume, retry, revoke, stream, suspend }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import AgentProvisioningView from '@/views/admin/AgentProvisioningView.vue'

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})
const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] }, columns: { type: Array, default: () => [] }, loading: Boolean },
  template: '<div><div v-for="row in data" :key="row.agent_id" class="agent-row"><slot name="cell-actions" :row="row" /></div><slot v-if="data.length === 0" name="empty" /></div>'
})
const ConfirmDialogStub = defineComponent({
  props: { show: Boolean, title: String, message: String },
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" class="confirm-dialog"><p>{{ title }} {{ message }}</p><slot /><button data-test="confirm" @click="$emit(\'confirm\')">confirm</button><button data-test="cancel" @click="$emit(\'cancel\')">cancel</button></div>'
})

function makeAgent(overrides: Record<string, unknown> = {}) {
  return {
    agent_id: 'agt_01234567890123456789012345678901',
    slug: 'alpha',
    domain: 'alpha.cc2.cx',
    display_name: 'Alpha',
    owner_main_user_id: 42,
    plan_id: 'starter',
    brand_name: 'Alpha',
    status: 'active',
    current_step: 'ready',
    can_retry: false,
    domain_status: 'ready',
    request_id: 'req-1',
    created_at: '2026-09-24T00:00:00Z',
    updated_at: '2026-09-24T00:00:00Z',
    ...overrides
  }
}

function mountView() {
  return mount(AgentProvisioningView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        ConfirmDialog: ConfirmDialogStub,
        Pagination: true,
        Icon: true
      }
    }
  })
}

describe('AgentProvisioningView', () => {
  beforeEach(() => {
    activate.mockReset().mockResolvedValue(makeAgent({ status: 'active' }))
    create.mockReset().mockResolvedValue(makeAgent({ status: 'pending' }))
    list.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    resume.mockReset()
    retry.mockReset().mockResolvedValue(makeAgent({ status: 'pending' }))
    revoke.mockReset().mockResolvedValue(makeAgent({ status: 'revoked' }))
    suspend.mockReset().mockResolvedValue(makeAgent({ status: 'suspended' }))
    stream.mockReset().mockImplementation(({ signal }: { signal: AbortSignal }) => new Promise<void>(resolve => {
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
  })

  it('clearly labels the control-only limitation and creates a station record', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('agentProvisioning.controlPlaneNoticeTitle')

    await wrapper.get('button.btn-primary').trigger('click')
    await wrapper.get('input[placeholder="team-name"]').setValue('alpha')
    const inputs = wrapper.findAll('input')
    await inputs.find(input => input.attributes('inputmode') === 'numeric')!.setValue('42')
    const namedInputs = wrapper.findAll('input')
    await namedInputs.find(input => input.attributes('maxlength') === '100' && input.attributes('required') !== undefined)!.setValue('Alpha Station')
    await namedInputs.find(input => input.attributes('maxlength') === '64')!.setValue('starter')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      requested_slug: 'alpha',
      display_name: 'Alpha Station',
      owner_main_user_id: '42',
      plan_id: 'starter',
      domain_mode: 'platform_subdomain'
    }))
    wrapper.unmount()
  })

  it('requires typing the exact agent ID before sending a revoke request', async () => {
    list.mockResolvedValue({ items: [makeAgent()], total: 1, page: 1, page_size: 20 })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.agent-row .btn-danger').trigger('click')
    await wrapper.get('.confirm-dialog input').setValue('wrong-id')
    await wrapper.get('[data-test="confirm"]').trigger('click')
    expect(revoke).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('agentProvisioning.confirmationMismatch')

    await wrapper.get('.confirm-dialog input').setValue('agt_01234567890123456789012345678901')
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()
    expect(revoke).toHaveBeenCalledWith('agt_01234567890123456789012345678901', 'agt_01234567890123456789012345678901')
    wrapper.unmount()
  })

  it('offers an explicit retry only for retryable failed deployments', async () => {
    const agentId = 'agt_01234567890123456789012345678901'
    list.mockResolvedValue({ items: [makeAgent({ status: 'failed', current_step: 'failed', can_retry: true })], total: 1, page: 1, page_size: 20 })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.agent-row .btn-secondary').trigger('click')
    expect(wrapper.text()).toContain('agentProvisioning.retryTitle')
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()
    expect(retry).toHaveBeenCalledWith(agentId)
    wrapper.unmount()
  })

  it('requires an explicit readiness confirmation before activating a pending station', async () => {
    const agentId = 'agt_01234567890123456789012345678901'
    list.mockResolvedValue({ items: [makeAgent({ status: 'pending' })], total: 1, page: 1, page_size: 20 })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.agent-row .btn-primary').trigger('click')
    await wrapper.get('[data-test="confirm"]').trigger('click')
    expect(activate).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('agentProvisioning.readinessConfirmationRequired')

    await wrapper.get('.confirm-dialog input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()
    expect(activate).toHaveBeenCalledWith(agentId)
    wrapper.unmount()
  })
})
