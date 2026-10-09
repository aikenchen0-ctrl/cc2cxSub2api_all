import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PaymentProviderDialog from './PaymentProviderDialog.vue'
import ImageUpload from '../common/ImageUpload.vue'
import { stationCallbackPaths, stationWebhookPath } from './providerConfig'
import type { ProviderInstance } from '@/types/payment'

const keys = ['easypay', 'alipay', 'wxpay', 'stripe', 'airwallex']
const props = {
  show: true, saving: false, agentId: 'station-a', editing: null as ProviderInstance | null,
  allKeyOptions: keys.map(value => ({ value, label: value })),
  enabledKeyOptions: keys.map(value => ({ value, label: value })),
  allPaymentTypes: ['alipay', 'wxpay', 'stripe', 'airwallex'].map(value => ({ value, label: value })),
  redirectLabel: '跳转',
}
const configs: Record<string, Record<string, string>> = {
  easypay: { pid: '1001', apiBase: 'https://pay.example', customMethods: '[{"type":"coin","upstreamType":"usdt","displayName":"USDT"}]' },
  alipay: { appId: 'app-a' },
  wxpay: { appId: 'app-w', mchId: 'merchant-w', certSerial: 'cert-w', publicKeyId: 'pub-w' },
  stripe: { publishableKey: 'pk_a', currency: 'JPY' },
  airwallex: { clientId: 'client-a', currency: 'USD', countryCode: 'US', apiBase: 'https://api.airwallex.com/api/v1', accountId: '' },
}
function instance(key: string): ProviderInstance {
  return { id: 1, provider_key: key, name: `${key} station A`, config: configs[key]!, enabled: true,
    payment_mode: key === 'easypay' ? 'popup' : '', sort_order: 1,
    supported_types: key === 'easypay' ? ['alipay', 'coin'] : key === 'stripe' ? ['card', 'link'] : [key],
    limits: JSON.stringify({ [key === 'easypay' ? 'alipay' : key]: { singleMin: 1, singleMax: 999, dailyLimit: 5000 } }),
    refund_enabled: true, allow_user_refund: true }
}
const global = { stubs: { Teleport: { template: '<div><slot /></div>' } } }

describe('station payment provider settings', () => {
  it('derives independent callback paths without mutable shared state', () => {
    const a = stationCallbackPaths('station-a', 'easypay')
    const b = stationCallbackPaths('station-b', 'easypay')
    expect(a?.notifyUrl).toBe('/api/v1/agent/billing/webhook/station-a/easypay')
    expect(b?.notifyUrl).toBe('/api/v1/agent/billing/webhook/station-b/easypay')
    expect(a?.notifyUrl).not.toContain('station-b')
    expect(stationWebhookPath('', 'stripe')).toBe('')
    expect(stationWebhookPath('station-a', 'unknown')).toBe('')
  })

  it.each(keys)('preserves %s settings and omitted secrets when editing', async key => {
    const provider = instance(key)
    const wrapper = mount(PaymentProviderDialog, { props: { ...props, editing: provider }, global })
    wrapper.vm.loadProvider(provider)
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    const payload = wrapper.emitted('save')?.[0]?.[0] as Record<string, any>
    expect(payload).toBeDefined()
    expect(payload.name).toBe(provider.name)
    expect(payload.payment_mode).toBe(provider.payment_mode)
    expect(payload.supported_types).toEqual(provider.supported_types)
    expect(payload.config).toMatchObject(configs[key]!)
    for (const secret of ['pkey', 'privateKey', 'apiV3Key', 'publicKey', 'secretKey', 'apiKey', 'webhookSecret']) {
      expect(payload.config[secret]).toBeUndefined()
    }
    expect(JSON.parse(payload.limits)).toEqual(JSON.parse(provider.limits))
    if (['easypay', 'alipay', 'wxpay'].includes(key)) {
      expect(payload.config.notifyUrl).toContain(`/webhook/station-a/${key}`)
    } else {
      expect(wrapper.text()).toContain(`/webhook/station-a/${key}`)
    }
    wrapper.unmount()
  })

  it('shows validation and server failures inside the dialog', async () => {
    const wrapper = mount(PaymentProviderDialog, { props, global })
    wrapper.vm.reset('stripe')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).not.toBe('')
    expect(wrapper.emitted('save')).toBeUndefined()
    wrapper.vm.reset('stripe')
    await wrapper.setProps({ serverError: '存在未完成订单，不能修改商户密钥' })
    expect(wrapper.get('[role="alert"]').text()).toContain('未完成订单')
    wrapper.unmount()
  })

  it('sanitizes uploaded SVG markup', () => {
    const wrapper = mount(ImageUpload, { props: { mode: 'svg', modelValue: '<svg onload="alert(1)"><script>alert(1)</script><circle r="3"/></svg>' } })
    expect(wrapper.find('circle').exists()).toBe(true)
    expect(wrapper.html()).not.toContain('onload')
    expect(wrapper.find('script').exists()).toBe(false)
    wrapper.unmount()
  })
})
