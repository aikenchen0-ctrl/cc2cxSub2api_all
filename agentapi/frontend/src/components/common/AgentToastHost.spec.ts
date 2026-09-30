import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetAgentToastsForTests, useAgentToast } from '@/composables/useAgentToast'
import AgentToastHost from './AgentToastHost.vue'

describe('AgentToastHost', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetAgentToastsForTests()
  })

  afterEach(() => {
    resetAgentToastsForTests()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('renders main-site style typed notifications as text and allows dismissal', async () => {
    const wrapper = mount(AgentToastHost, { attachTo: document.body })
    const toast = useAgentToast()
    toast.showError('<img src=x onerror=alert(1)>', { title: '请求失败', duration: 5_000 })
    await flushPromises()

    const item = document.body.querySelector<HTMLElement>('[data-toast-type="error"]')
    expect(item?.textContent).toContain('请求失败')
    expect(item?.textContent).toContain('<img src=x onerror=alert(1)>')
    expect(item?.querySelector('img')).toBeNull()
    expect(document.body.querySelector('[aria-live="polite"]')).not.toBeNull()

    document.body.querySelector<HTMLButtonElement>('[aria-label^="关闭通知"]')?.click()
    await flushPromises()
    expect(toast.toasts.value).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows a duration progress bar and removes the notice after its timer', async () => {
    const wrapper = mount(AgentToastHost, { attachTo: document.body })
    const toast = useAgentToast()
    toast.showInfo('正在同步', { duration: 1_000 })
    await flushPromises()

    expect(document.body.querySelector('.agent-toast-progress')).not.toBeNull()
    vi.advanceTimersByTime(1_000)
    await flushPromises()
    expect(toast.toasts.value).toHaveLength(0)
    wrapper.unmount()
  })
})
