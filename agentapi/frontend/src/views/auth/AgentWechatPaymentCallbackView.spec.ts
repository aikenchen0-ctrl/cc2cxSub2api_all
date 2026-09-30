import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import AgentWechatPaymentCallbackView from './AgentWechatPaymentCallbackView.vue'

async function mountCallback(path: string, hash = '') {
  window.location.hash = hash
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/auth/wechat/payment/callback', component: AgentWechatPaymentCallbackView },
      { path: '/purchase', component: { template: '<div>purchase</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentWechatPaymentCallbackView, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('AgentWechatPaymentCallbackView', () => {
  afterEach(() => {
    window.location.hash = ''
  })

  it('copies the main-site fragment callback into a short-lived purchase resume query', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/wechat/payment/callback',
      '#wechat_resume_token=resume-token&redirect=%2Fpurchase',
    )

    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query).toEqual({ wechat_resume: '1', wechat_resume_token: 'resume-token' })
    expect(window.location.hash).toBe('')
    wrapper.unmount()
  })

  it('preserves the legacy OpenID payment fields needed to resume the order', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/wechat/payment/callback?openid=wx-user&payment_type=wxpay&amount=20&order_type=subscription&plan_id=9',
    )

    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query).toMatchObject({
      wechat_resume: '1',
      openid: 'wx-user',
      payment_type: 'wxpay',
      amount: '20',
      order_type: 'subscription',
      plan_id: '9',
    })
    wrapper.unmount()
  })

  it('rejects an external redirect and falls back to the local purchase page', async () => {
    const { wrapper, router } = await mountCallback(
      '/auth/wechat/payment/callback?wechat_resume_token=resume-token&redirect=https%3A%2F%2Fevil.example',
    )

    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query.wechat_resume_token).toBe('resume-token')
    wrapper.unmount()
  })

  it('shows a recoverable error when the callback has no resume identity', async () => {
    const { wrapper, router } = await mountCallback('/auth/wechat/payment/callback')

    expect(router.currentRoute.value.path).toBe('/auth/wechat/payment/callback')
    expect(wrapper.get('[role="alert"]').text()).toContain('缺少支付恢复信息')
    expect(wrapper.text()).toContain('返回购买页')
    wrapper.unmount()
  })
})
