import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentAffiliateRecordsPanel from './AgentAffiliateRecordsPanel.vue'

describe('AgentAffiliateRecordsPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('renders tenant-scoped invitation records and forwards filters', async () => {
    const list = vi.spyOn(agentAPI.adminAffiliates, 'invites').mockResolvedValue({
      items: [{
        inviter_id: 42, inviter_email: 'owner@example.com', inviter_username: 'Owner',
        invitee_id: 84, invitee_email: 'user@example.com', invitee_username: 'User',
        aff_code: 'TENANT', total_rebate: 2.5, created_at: '2026-09-28T00:00:00Z',
      }],
      total: 1, page: 1, page_size: 20, pages: 1,
    })
    const wrapper = mount(AgentAffiliateRecordsPanel, { props: { type: 'invites' } })
    await flushPromises()
    expect(wrapper.text()).toContain('owner@example.com')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('TENANT')
    expect(wrapper.text()).toContain('$2.50')

    vi.useFakeTimers()
    await wrapper.get('[aria-label="搜索推广记录"]').setValue('user@example.com')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(1, 20, expect.objectContaining({ search: 'user@example.com', sort_by: 'created_at', sort_order: 'desc' }))

    await wrapper.get('[aria-label="推广记录日期范围"]').trigger('click')
    await wrapper.get('[aria-label="日期范围开始日期"]').setValue('2026-09-01')
    await wrapper.get('[aria-label="日期范围结束日期"]').setValue('2026-09-29')
    await wrapper.findAll('button').find((button) => button.text() === '应用')!.trigger('click')
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(1, 20, expect.objectContaining({
      search: 'user@example.com',
      start_at: '2026-09-01',
      end_at: '2026-09-29',
    }))
  })

  it('renders rebate and transfer columns without management actions', async () => {
    vi.spyOn(agentAPI.adminAffiliates, 'rebates').mockResolvedValue({
      items: [{
        order_id: 9, out_trade_no: 'ORD-9', inviter_id: 42, inviter_email: 'owner@example.com', inviter_username: 'Owner',
        invitee_id: 84, invitee_email: 'user@example.com', invitee_username: 'User', order_amount: 10, pay_amount: 70,
        rebate_amount: 2, payment_type: 'alipay', order_status: 'COMPLETED', created_at: '2026-09-28T00:00:00Z',
      }], total: 1, page: 1, page_size: 20, pages: 1,
    })
    const rebate = mount(AgentAffiliateRecordsPanel, { props: { type: 'rebates' } })
    await flushPromises()
    expect(rebate.text()).toContain('ORD-9')
    expect(rebate.text()).toContain('COMPLETED')
    expect(rebate.get('[data-status="completed"]').text()).toBe('COMPLETED')
    expect(rebate.text()).not.toContain('赠送')
    rebate.unmount()

    vi.spyOn(agentAPI.adminAffiliates, 'transfers').mockResolvedValue({
      items: [{
        ledger_id: 3, user_id: 84, user_email: 'user@example.com', username: 'User', amount: 3,
        balance_after: 12, available_quota_after: 0, frozen_quota_after: 1, history_quota_after: 6,
        snapshot_available: true, created_at: '2026-09-29T00:00:00Z',
      }], total: 1, page: 1, page_size: 20, pages: 1,
    })
    const transfer = mount(AgentAffiliateRecordsPanel, { props: { type: 'transfers' } })
    await flushPromises()
    expect(transfer.text()).toContain('user@example.com')
    expect(transfer.text()).toContain('$12.00')
    expect(transfer.text()).not.toContain('重置')
  })

  it('shows an empty state and upstream error safely', async () => {
    vi.spyOn(agentAPI.adminAffiliates, 'invites').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    const empty = mount(AgentAffiliateRecordsPanel, { props: { type: 'invites' } })
    await flushPromises()
    expect(empty.text()).toContain('暂无本站用户邀请记录')
    empty.unmount()

    vi.spyOn(agentAPI.adminAffiliates, 'transfers').mockRejectedValue(new Error('主站暂不可用'))
    const failed = mount(AgentAffiliateRecordsPanel, { props: { type: 'transfers' } })
    await flushPromises()
    expect(failed.get('[role="alert"]').text()).toContain('主站暂不可用')
  })
})
