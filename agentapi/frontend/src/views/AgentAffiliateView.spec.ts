import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentAffiliateView from './AgentAffiliateView.vue'

afterEach(() => vi.restoreAllMocks())

const detail = {
  aff_code: 'ABC123', aff_count: 1, aff_quota: 2.5, aff_frozen_quota: 0.5,
  aff_history_quota: 8, effective_rebate_rate_percent: 12.5,
  invitees: [{ email: 'm***@example.com', username: 'm***', total_rebate: 2.5, created_at: '2026-09-29T00:00:00Z' }],
}

describe('AgentAffiliateView', () => {
  it('ports the main-site affiliate page using only AgentAPI user endpoints', async () => {
    const get = vi.spyOn(agentAPI.affiliate, 'get').mockResolvedValue(detail)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } })
    const wrapper = mount(AgentAffiliateView, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(get).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('12.5%')
    expect(wrapper.text()).toContain('$2.50')
    expect(wrapper.text()).toContain('ABC123')
    expect(wrapper.text()).toContain('/register?aff=ABC123')
    expect(wrapper.text()).toContain('m***@example.com')
    const copyButtons = wrapper.findAll('button').filter(button => button.text().includes('复制'))
    await copyButtons[1].trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('/register?aff=ABC123'))
  })

  it('confirms the authoritative transfer and reloads main-site detail', async () => {
    const accountChanged = vi.fn()
    window.addEventListener('agentapi:account-facts-changed', accountChanged)
    vi.spyOn(agentAPI.affiliate, 'get').mockResolvedValueOnce(detail).mockResolvedValueOnce({ ...detail, aff_quota: 0, aff_history_quota: 8 })
    const transfer = vi.spyOn(agentAPI.affiliate, 'transfer').mockResolvedValue({ transferred_quota: 2.5, balance: 12.5 })
    const wrapper = mount(AgentAffiliateView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    const confirm = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('确认转入')) as HTMLButtonElement
    confirm.click()
    await flushPromises()
    expect(transfer).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('当前主站余额 $12.50')
    expect(wrapper.text()).toContain('当前没有可转入的返利')
    expect(accountChanged).toHaveBeenCalledTimes(1)
    expect((accountChanged.mock.calls[0][0] as CustomEvent).detail.balanceCents).toBe(1250)
    window.removeEventListener('agentapi:account-facts-changed', accountChanged)
    wrapper.unmount()
  })

  it('shows upstream load failures without fabricated data', async () => {
    vi.spyOn(agentAPI.affiliate, 'get').mockRejectedValue({ status: 503, message: '主站暂不可用' })
    const wrapper = mount(AgentAffiliateView)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('主站暂不可用')
    expect(wrapper.text()).not.toContain('ABC123')
  })
})
