import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentOrdersView from './AgentOrdersView.vue'
import { agentAPI } from '@/agent/api'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/agent/api', async () => {
  const actual = await vi.importActual<typeof import('@/agent/api')>('@/agent/api')
  return { ...actual, agentAPI: { ...actual.agentAPI, billing: { ...actual.agentAPI.billing, myOrders: vi.fn(), cancelMyOrder: vi.fn(), requestMyRefund: vi.fn(), refundEligibleProviders: vi.fn() } } }
})

const completed = { id: 7, amount: 12, pay_amount: 12, fee_rate: 0, currency: 'USD', payment_type: 'stripe', out_trade_no: 'ORDER-7', status: 'COMPLETED', order_type: 'balance', created_at: '2026-09-29T00:00:00Z', expires_at: '2026-09-30T00:00:00Z', refund_amount: 0, provider_instance_id: 'stripe-1' }

describe('AgentOrdersView', () => {
  it('admin refunds use the local provider action and display pending confirmation honestly', async () => {
    vi.spyOn(agentAPI.billing, 'orders').mockResolvedValue({ items: [completed], total: 1, page: 1, page_size: 20, pages: 1 })
    vi.spyOn(agentAPI.billing, 'providers').mockResolvedValue({ items: [{ id: 1, refund_enabled: true }] } as Awaited<ReturnType<typeof agentAPI.billing.providers>>)
    vi.mocked(agentAPI.billing.orders).mockResolvedValue({ items: [{ ...completed, provider_instance_id: '1' }], total: 1, page: 1, page_size: 20, pages: 1 })
    const refund = vi.spyOn(agentAPI.billing, 'refundOrder').mockResolvedValue({ message: 'refund processed', status: 'REFUND_PENDING' })
    const wrapper = mount(AgentOrdersView, { props: { admin: true }, attachTo: document.body, global: { stubs: { DataTable: { props: ['data'], template: '<div><slot name="cell-actions" v-for="row in data" :row="row" /></div>' }, AgentPagination: true, Icon: true } } })
    await flushPromises()
    expect(agentAPI.billing.orders).toHaveBeenCalledWith(1, 20, '')
    expect(wrapper.text()).toContain('本站订单')
    await wrapper.get('button.text-primary-600').trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('[data-testid="confirm-refund"]')!.click()
    await flushPromises()
    expect(refund).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('退款结果待渠道确认')
    expect(wrapper.text()).not.toContain('渠道已确认退款完成')
    expect(agentAPI.billing.requestMyRefund).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(agentAPI.billing.myOrders).mockResolvedValue({ items: [completed], total: 1, page: 1, page_size: 20, pages: 1 })
    vi.mocked(agentAPI.billing.refundEligibleProviders).mockResolvedValue({ provider_instance_ids: ['stripe-1'] })
    vi.mocked(agentAPI.billing.cancelMyOrder).mockResolvedValue({ message: 'ok' })
    vi.mocked(agentAPI.billing.requestMyRefund).mockResolvedValue({ message: 'ok' })
  })
  it('loads tenant-local orders and filters them', async () => {
    const wrapper = mount(AgentOrdersView, { global: { stubs: { DataTable: { props: ['data'], template: '<div><slot name="cell-actions" v-for="row in data" :row="row" /></div>' }, AgentPagination: true, BaseDialog: true, Icon: true } } })
    await flushPromises()
    expect(agentAPI.billing.myOrders).toHaveBeenCalledWith(1, 20, '')
    await wrapper.get('[aria-label="订单状态"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('已完成'))!.click()
    await flushPromises()
    expect(agentAPI.billing.myOrders).toHaveBeenLastCalledWith(1, 20, 'COMPLETED')
    expect(wrapper.text()).toContain('查看当前账户的订单、余额与退款状态')
  })
  it('submits refund through the mapped order API then reloads', async () => {
    const wrapper = mount(AgentOrdersView, { attachTo: document.body, global: { stubs: { DataTable: { props: ['data'], template: '<div><slot name="cell-actions" v-for="row in data" :row="row" /></div>' }, AgentPagination: true, Icon: true } } })
    await flushPromises()
    await wrapper.get('button.text-primary-600').trigger('click')
    await flushPromises()
    const textarea = document.body.querySelector<HTMLTextAreaElement>('#refund-reason')!
    textarea.value = 'duplicate charge'; textarea.dispatchEvent(new Event('input'))
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('[data-testid="confirm-refund"]')!.click()
    await flushPromises()
    expect(agentAPI.billing.requestMyRefund).toHaveBeenCalledWith(7, 'duplicate charge')
    expect(agentAPI.billing.myOrders).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
