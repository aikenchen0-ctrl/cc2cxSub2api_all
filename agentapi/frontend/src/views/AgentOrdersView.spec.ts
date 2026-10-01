import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentOrdersView from './AgentOrdersView.vue'
import { agentAPI } from '@/agent/api'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/agent/api', async () => {
  const actual = await vi.importActual<typeof import('@/agent/api')>('@/agent/api')
  return { ...actual, agentAPI: { ...actual.agentAPI, orders: { list: vi.fn(), cancel: vi.fn(), requestRefund: vi.fn(), refundEligibleProviders: vi.fn() } } }
})

const completed = { id: 7, amount: 12, pay_amount: 12, fee_rate: 0, currency: 'USD', payment_type: 'stripe', out_trade_no: 'ORDER-7', status: 'COMPLETED', order_type: 'balance', created_at: '2026-09-29T00:00:00Z', expires_at: '2026-09-30T00:00:00Z', refund_amount: 0, provider_instance_id: 'stripe-1' }

describe('AgentOrdersView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(agentAPI.orders.list).mockResolvedValue({ items: [completed], total: 1, page: 1, page_size: 20, pages: 1 })
    vi.mocked(agentAPI.orders.refundEligibleProviders).mockResolvedValue({ provider_instance_ids: ['stripe-1'] })
    vi.mocked(agentAPI.orders.cancel).mockResolvedValue({ message: 'ok' })
    vi.mocked(agentAPI.orders.requestRefund).mockResolvedValue({ message: 'ok' })
  })
  it('loads authoritative main-site orders and filters them', async () => {
    const wrapper = mount(AgentOrdersView, { global: { stubs: { DataTable: { props: ['data'], template: '<div><slot name="cell-actions" v-for="row in data" :row="row" /></div>' }, AgentPagination: true, BaseDialog: true, Icon: true } } })
    await flushPromises()
    expect(agentAPI.orders.list).toHaveBeenCalledWith(1, 20, '')
    await wrapper.get('[aria-label="订单状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('已完成'))!.click()
    await flushPromises()
    expect(agentAPI.orders.list).toHaveBeenLastCalledWith(1, 20, 'COMPLETED')
    expect(wrapper.text()).toContain('查看当前账户的订单、余额与退款状态')
  })
  it('submits refund through the mapped order API then reloads', async () => {
    const wrapper = mount(AgentOrdersView, { attachTo: document.body, global: { stubs: { DataTable: { props: ['data'], template: '<div><slot name="cell-actions" v-for="row in data" :row="row" /></div>' }, AgentPagination: true, Icon: true } } })
    await flushPromises()
    await wrapper.get('button.text-purple-600').trigger('click')
    await flushPromises()
    const textarea = document.body.querySelector<HTMLTextAreaElement>('#refund-reason')!
    textarea.value = 'duplicate charge'; textarea.dispatchEvent(new Event('input'))
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('[data-testid="confirm-refund"]')!.click()
    await flushPromises()
    expect(agentAPI.orders.requestRefund).toHaveBeenCalledWith(7, 'duplicate charge')
    expect(agentAPI.orders.list).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
