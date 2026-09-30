import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentAdminOrdersPanel from './AgentAdminOrdersPanel.vue'
import { agentAPI, type AgentAdminOrder } from '@/agent/api'

const order: AgentAdminOrder = {
  id: 7,
  main_user_id: '84',
  user_email: 'second@example.com',
  user_display_name: 'Second User',
  amount: 12.5,
  pay_amount: 13,
  fee_rate: 4,
  currency: 'USD',
  payment_type: 'stripe',
  out_trade_no: 'agent-order-84',
  status: 'PENDING',
  order_type: 'balance',
  created_at: '2026-09-29T00:00:00Z',
  expires_at: '2026-09-30T00:00:00Z',
  refund_amount: 0,
}

describe('AgentAdminOrdersPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  function mockData() {
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({
      items: [{ agent_id: 'agent-1', main_user_id: '84', email: 'second@example.com', display_name: 'Second User', status: 'active', balance_cents: 0, created_at: '', updated_at: '' }],
      total: 1,
      page: 1,
      page_size: 500,
    })
    return vi.spyOn(agentAPI.adminOrders, 'list').mockResolvedValue({ items: [order], total: 1, page: 1, page_size: 20, pages: 1 })
  }

  it('shows only the tenant-enriched main-site order fields and filters by mapped user', async () => {
    const list = mockData()
    const wrapper = mount(AgentAdminOrdersPanel)
    await flushPromises()

    expect(wrapper.text()).toContain('Second User')
    expect(wrapper.text()).toContain('agent-order-84')
    expect(wrapper.text()).toContain('只聚合当前代理站已登记用户')
    expect(wrapper.text()).not.toContain('provider_secret')
    expect(wrapper.get('[data-status="pending"]').text()).toBe('待支付')
    expect(list).toHaveBeenCalledWith(1, 20, '', '')

    await wrapper.get('[aria-label="筛选本站用户"]').trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.includes('Second User'))!.click()
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(1, 20, '', '84')
    wrapper.unmount()
  })

  it('cancels a pending order with both mapped user and order identity', async () => {
    mockData()
    const cancel = vi.spyOn(agentAPI.adminOrders, 'cancel').mockResolvedValue({ message: 'order cancelled' })
    const wrapper = mount(AgentAdminOrdersPanel, { attachTo: document.body })
    await flushPromises()

    const cancelButton = wrapper.findAll('button').find(button => button.text() === '取消')
    expect(cancelButton).toBeDefined()
    await cancelButton!.trigger('click')
    await flushPromises()
    const confirm = document.body.querySelector<HTMLButtonElement>('[data-testid="confirm-admin-order-cancel"]')
    expect(confirm?.textContent).toContain('确认取消')
    confirm?.click()
    await flushPromises()

    expect(cancel).toHaveBeenCalledWith('84', 7)
    expect(wrapper.text()).toContain('订单 #7 已取消')
    wrapper.unmount()
  })
})
