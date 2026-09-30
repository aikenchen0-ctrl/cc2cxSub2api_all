import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentStatusBadge from './AgentStatusBadge.vue'

describe('AgentStatusBadge', () => {
  it.each([
    ['COMPLETED', '已完成', 'bg-green-500'],
    ['operational', '运行正常', 'bg-green-500'],
    ['measured', '实测', 'bg-green-500'],
    ['bound', '已绑定', 'bg-green-500'],
    ['done', '已完成', 'bg-green-500'],
    ['pending', '待处理', 'bg-yellow-500'],
    ['queued', '排队中', 'bg-yellow-500'],
    ['archived', '已归档', 'bg-yellow-500'],
    ['unobserved', '未观测', 'bg-yellow-500'],
    ['mismatch', '模型不一致', 'bg-red-500'],
    ['degraded', '降级', 'bg-yellow-500'],
    ['REFUND_PENDING', '退款处理中', 'bg-yellow-500'],
    ['refund_failed', '退款失败', 'bg-red-500'],
    ['revoked', '已撤销', 'bg-red-500'],
    ['rejected', '已拒绝', 'bg-red-500'],
    ['unknown', '未知', 'bg-gray-400'],
  ])('renders %s using the main-site status treatment', (status, label, colorClass) => {
    const wrapper = mount(AgentStatusBadge, { props: { status, label } })

    expect(wrapper.attributes('data-status')).toBe(status.toLowerCase())
    expect(wrapper.text()).toBe(label)
    expect(wrapper.get('span > span').classes()).toContain(colorClass)
  })
})
