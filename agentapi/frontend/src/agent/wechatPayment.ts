export interface WeixinJSBridgeLike {
  invoke(
    action: 'getBrandWCPayRequest',
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

type WindowWithWechatBridge = Window & { WeixinJSBridge?: WeixinJSBridgeLike }

export type WechatPaymentOutcome = 'success' | 'cancelled' | 'failed'

export function isWechatBrowser(userAgent = window.navigator.userAgent): boolean {
  return /MicroMessenger/i.test(userAgent)
}

function currentBridge(): WeixinJSBridgeLike | undefined {
  return (window as WindowWithWechatBridge).WeixinJSBridge
}

export function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = currentBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null): void => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = (): void => finish(currentBridge() || null)
    const timer = window.setTimeout(() => finish(currentBridge() || null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

export async function invokeWechatJsapiPayment(
  payload: Record<string, unknown>,
  timeoutMs = 4000,
): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge(timeoutMs)
  if (!bridge) throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, result => resolve(result || {}))
  })
}

export function classifyWechatPaymentResult(result: Record<string, unknown>): WechatPaymentOutcome {
  const message = String(result.err_msg || result.errMsg || '').trim().toLowerCase()
  if (message.includes('cancel')) return 'cancelled'
  if (!message || message.includes('ok')) return 'success'
  return 'failed'
}
