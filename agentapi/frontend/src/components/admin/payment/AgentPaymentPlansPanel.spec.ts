import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentAdminCheckoutPlan } from '@/agent/api'
import AgentPaymentPlansPanel from './AgentPaymentPlansPanel.vue'

const plan: AgentAdminCheckoutPlan = {
  id: 7,
  group_id: 3,
  group_name: 'OpenAI Pro',
  group_platform: 'openai',
  name: '本站专业版',
  description: '本站说明',
  price: 29.9,
  currency: 'USD',
  validity_days: 30,
  validity_unit: 'days',
  features: ['高优先级', '更多额度'],
  product_name: 'subscription',
  enabled: true,
  sort_order: 10,
  customized: true,
  source_name: '主站专业版',
  source_description: '主站说明',
  source_features: ['主站权益'],
}

afterEach(() => vi.restoreAllMocks())

describe('AgentPaymentPlansPanel', () => {
  it('renders main-site plan facts and tenant-only presentation controls', async () => {
    const list = vi.spyOn(agentAPI.adminPaymentPlans, 'list').mockResolvedValue({ items: [plan], total: 1 })
    const wrapper = mount(AgentPaymentPlansPanel)
    await flushPromises()

    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('本站专业版')
    expect(wrapper.text()).toContain('OpenAI Pro')
    expect(wrapper.get('[data-group-id="3"]').text()).toContain('OpenAI Pro')
    expect(wrapper.get('[data-group-id="3"]').attributes('data-platform')).toBe('openai')
    expect(wrapper.text()).toContain('$29.90')
    expect(wrapper.text()).toContain('价格、币种、有效期、订阅分组和真实销售状态由 Sub2API 主站统一管理')
    expect(wrapper.text()).not.toContain('管理员 Key')
    wrapper.unmount()
  })

  it('edits only the current agent display policy while preserving authoritative price fields', async () => {
    vi.spyOn(agentAPI.adminPaymentPlans, 'list').mockResolvedValue({ items: [plan], total: 1 })
    const update = vi.spyOn(agentAPI.adminPaymentPlans, 'update').mockResolvedValue({ ...plan, name: '代理站旗舰版', sort_order: 2 })
    const wrapper = mount(AgentPaymentPlansPanel)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '编辑')!.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('主站专业版')
    expect(document.body.textContent).toContain('$29.90')
    const sort = document.body.querySelector<HTMLInputElement>('#plan-sort')!
    const name = document.body.querySelector<HTMLInputElement>('#plan-name')!
    const description = document.body.querySelector<HTMLTextAreaElement>('#plan-description')!
    const features = document.body.querySelector<HTMLTextAreaElement>('#plan-features')!
    sort.value = '2'; sort.dispatchEvent(new Event('input'))
    name.value = '代理站旗舰版'; name.dispatchEvent(new Event('input'))
    description.value = '代理站专属说明'; description.dispatchEvent(new Event('input'))
    features.value = '权益 A\n权益 B'; features.dispatchEvent(new Event('input'))
    document.body.querySelector<HTMLButtonElement>('[data-testid="save-plan-policy"]')!.click()
    await flushPromises()

    expect(update).toHaveBeenCalledWith(7, {
      enabled: true,
      sort_order: 2,
      display_name: '代理站旗舰版',
      description: '代理站专属说明',
      features: ['权益 A', '权益 B'],
    })
    const payload = update.mock.calls[0][1] as Record<string, unknown>
    expect(payload).not.toHaveProperty('price')
    expect(payload).not.toHaveProperty('group_id')
    expect(payload).not.toHaveProperty('validity_days')
    expect(wrapper.text()).toContain('代理站旗舰版')
    wrapper.unmount()
  })

  it('can hide a plan through the local policy endpoint', async () => {
    vi.spyOn(agentAPI.adminPaymentPlans, 'list').mockResolvedValue({ items: [plan], total: 1 })
    const update = vi.spyOn(agentAPI.adminPaymentPlans, 'update').mockResolvedValue({ ...plan, enabled: false })
    const wrapper = mount(AgentPaymentPlansPanel)
    await flushPromises()

    await wrapper.get('[aria-label="隐藏套餐"]').trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ enabled: false, sort_order: 10 }))
    expect(wrapper.text()).toContain('套餐已从本站购买页面隐藏')
    wrapper.unmount()
  })
})
