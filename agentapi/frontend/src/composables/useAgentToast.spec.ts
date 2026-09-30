import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetAgentToastsForTests, useAgentToast } from './useAgentToast'

describe('useAgentToast', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetAgentToastsForTests()
  })

  afterEach(() => {
    resetAgentToastsForTests()
    vi.useRealTimers()
  })

  it('copies the main-site typed toast defaults and auto-dismisses safely', () => {
    const toast = useAgentToast()
    const id = toast.showSuccess('保存成功')

    expect(id).toBe('agent-toast-1')
    expect(toast.toasts.value).toEqual([
      { id, type: 'success', message: '保存成功', title: undefined, duration: 3_000 },
    ])

    vi.advanceTimersByTime(2_999)
    expect(toast.toasts.value).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(toast.toasts.value).toHaveLength(0)
  })

  it('supports persistent notices, manual dismissal and bounded plain text', () => {
    const toast = useAgentToast()
    const id = toast.showToast('warning', `  ${'a'.repeat(1_100)}  `, {
      title: ` ${'b'.repeat(140)} `,
    })

    expect(toast.toasts.value[0]?.message).toHaveLength(1_000)
    expect(toast.toasts.value[0]?.title).toHaveLength(120)
    expect(toast.toasts.value[0]?.duration).toBeUndefined()
    vi.advanceTimersByTime(60_000)
    expect(toast.toasts.value).toHaveLength(1)

    toast.removeToast(id)
    expect(toast.toasts.value).toHaveLength(0)
  })
})
