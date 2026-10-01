import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import UserDashboardQuickActions from './UserDashboardQuickActions.vue'

describe('copied dashboard actions', () => {
  it.each([['创建 API 密钥', '/keys'], ['查看用量', '/usage'], ['查看订单', '/orders'], ['账户充值', '/purchase']])('routes %s to the tenant page', async (label, target) => {
    const router = createRouter({ history: createMemoryHistory(), routes: ['/', '/keys', '/usage', '/orders', '/purchase'].map(path => ({ path, component: { template: '<div />' } })) })
    await router.push('/')
    const wrapper = mount(UserDashboardQuickActions, { global: { plugins: [router] } })
    await wrapper.findAll('button').find(button => button.text().includes(label))!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe(target)
    expect(wrapper.text()).not.toContain('兑换码')
    wrapper.unmount()
  })
})
