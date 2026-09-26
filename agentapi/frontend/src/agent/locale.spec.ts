import { describe, expect, it } from 'vitest'
import { currencyLabel, enumLabel, errorMessage, operationLabel, providerLabel, statusLabel, targetLabel, taskTypeLabel } from './locale'

describe('Chinese interface labels', () => {
  it('translates common persisted states and audit labels', () => {
    expect(statusLabel('paid_pending_allocation')).toBe('已付款，待分配')
    expect(statusLabel('owner_upstream')).toBe('由代理站主账户计费')
    expect(enumLabel('unknown-billing-mode')).toBe('其他')
    expect(statusLabel('unrecognized-upstream-state')).toBe('其他状态')
    expect(operationLabel('wallet.sync')).toBe('同步主账户余额')
    expect(targetLabel('agent_wallet')).toBe('代理站钱包')
    expect(taskTypeLabel('image')).toBe('图片')
    expect(providerLabel('alipay')).toBe('支付宝')
    expect(currencyLabel('CNY')).toBe('人民币')
  })

  it('shows translated backend errors or a Chinese fallback', () => {
    expect(errorMessage({ code: 'AGENT_INSUFFICIENT_BALANCE' }, '请求失败，请稍后重试。'))
      .toBe('余额不足，请先充值或联系管理员分配余额。')
    expect(errorMessage(new Error('unknown English error'), '加载资料失败，请稍后重试。'))
      .toBe('加载资料失败，请稍后重试。')
    expect(errorMessage(new Error('请求失败：connection refused'), '加载资料失败，请稍后重试。'))
      .toBe('加载资料失败，请稍后重试。')
    expect(errorMessage(new Error('资料已更新'), '加载资料失败，请稍后重试。'))
      .toBe('资料已更新')
  })
})
