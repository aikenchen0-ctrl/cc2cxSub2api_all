import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  classifyWechatPaymentResult,
  invokeWechatJsapiPayment,
  isWechatBrowser,
  waitForWeixinJSBridge,
  type WeixinJSBridgeLike,
} from './wechatPayment'

type WindowWithWechatBridge = Window & { WeixinJSBridge?: WeixinJSBridgeLike }

describe('wechat payment bridge', () => {
  afterEach(() => {
    delete (window as WindowWithWechatBridge).WeixinJSBridge
    vi.useRealTimers()
  })

  it('detects the embedded WeChat browser without treating normal mobile browsers as WeChat', () => {
    expect(isWechatBrowser('Mozilla/5.0 MicroMessenger/8.0.50')).toBe(true)
    expect(isWechatBrowser('Mozilla/5.0 iPhone Mobile Safari')).toBe(false)
  })

  it('invokes the official JSAPI bridge with the main-site payload', async () => {
    const invoke = vi.fn((_action, _payload, callback) => callback({ err_msg: 'get_brand_wcpay_request:ok' }))
    ;(window as WindowWithWechatBridge).WeixinJSBridge = { invoke }

    const payload = { appId: 'wx-app', timeStamp: '123', nonceStr: 'nonce', package: 'prepay_id=1', signType: 'RSA', paySign: 'signed' }
    await expect(invokeWechatJsapiPayment(payload)).resolves.toMatchObject({ err_msg: 'get_brand_wcpay_request:ok' })
    expect(invoke).toHaveBeenCalledWith('getBrandWCPayRequest', payload, expect.any(Function))
  })

  it('waits for the bridge-ready event and times out safely when unavailable', async () => {
    vi.useFakeTimers()
    const waiting = waitForWeixinJSBridge(20)
    await vi.advanceTimersByTimeAsync(20)
    await expect(waiting).resolves.toBeNull()
  })

  it('classifies success, cancellation and provider failure responses', () => {
    expect(classifyWechatPaymentResult({ err_msg: 'get_brand_wcpay_request:ok' })).toBe('success')
    expect(classifyWechatPaymentResult({ err_msg: 'get_brand_wcpay_request:cancel' })).toBe('cancelled')
    expect(classifyWechatPaymentResult({ err_msg: 'get_brand_wcpay_request:fail' })).toBe('failed')
  })
})
