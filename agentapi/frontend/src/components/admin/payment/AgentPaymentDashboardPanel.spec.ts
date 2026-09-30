import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentAdminPaymentDashboard } from '@/agent/api'
import AgentPaymentDashboardPanel from './AgentPaymentDashboardPanel.vue'

vi.mock('vue-chartjs', () => ({
  Line: { template: '<div data-testid="payment-line-chart" />' },
}))

const dashboard: AgentAdminPaymentDashboard = {
  today_amount: { USD: 12.5 },
  total_amount: { USD: 20 },
  today_count: 1,
  total_count: 2,
  avg_amount: { USD: 10 },
  pending_orders: 3,
  daily_series: [
    { date: '2026-09-28', amount: { USD: 7.5 }, count: 1 },
    { date: '2026-09-29', amount: { USD: 12.5 }, count: 1 },
  ],
  payment_methods: [
    { type: 'stripe', amount: { USD: 12.5 }, count: 1 },
    { type: 'wxpay', amount: { USD: 7.5 }, count: 1 },
  ],
  top_users: {
    USD: [
      { main_user_id: '42', email: 'owner@example.com', name: 'Owner', amount: 12.5 },
      { main_user_id: '84', email: 'member@example.com', name: 'Member', amount: 7.5 },
    ],
  },
}

afterEach(() => vi.restoreAllMocks())

describe('AgentPaymentDashboardPanel', () => {
  it('renders tenant-only payment statistics copied from the main dashboard layout', async () => {
    const load = vi.spyOn(agentAPI.adminOrders, 'dashboard').mockResolvedValue(dashboard)
    const wrapper = mount(AgentPaymentDashboardPanel)
    await flushPromises()

    expect(load).toHaveBeenCalledWith(30)
    expect(wrapper.text()).toContain('今日收入')
    expect(wrapper.text()).toContain('区间收入')
    expect(wrapper.text()).toContain('待支付订单')
    expect(wrapper.text()).toContain('Stripe')
    expect(wrapper.text()).toContain('微信支付')
    expect(wrapper.text()).toContain('Owner')
    expect(wrapper.text()).toContain('Member')
    expect(wrapper.find('[data-testid="payment-line-chart"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('main_admin_secret')
    wrapper.unmount()
  })

  it('reloads the authoritative dashboard for 7/30/90-day windows and manual refresh', async () => {
    const load = vi.spyOn(agentAPI.adminOrders, 'dashboard').mockResolvedValue(dashboard)
    const wrapper = mount(AgentPaymentDashboardPanel)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '7 天')!.trigger('click')
    await flushPromises()
    expect(load).toHaveBeenLastCalledWith(7)

    await wrapper.get('[aria-label="刷新支付统计"]').trigger('click')
    await flushPromises()
    expect(load).toHaveBeenLastCalledWith(7)
    expect(load).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('shows a recoverable error without inventing zero-valued main-site facts', async () => {
    vi.spyOn(agentAPI.adminOrders, 'dashboard').mockRejectedValue(new Error('主站暂不可用'))
    const wrapper = mount(AgentPaymentDashboardPanel)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('主站暂不可用')
    expect(wrapper.text()).not.toContain('¥0.00')
    wrapper.unmount()
  })
})
