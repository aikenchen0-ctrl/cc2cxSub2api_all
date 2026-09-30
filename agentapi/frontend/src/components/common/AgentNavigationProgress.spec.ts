import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetAgentNavigationLoadingForTests, useAgentNavigationLoadingState } from '@/composables/useAgentNavigationLoading'
import AgentNavigationProgress from './AgentNavigationProgress.vue'

describe('AgentNavigationProgress', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetAgentNavigationLoadingForTests()
  })

  afterEach(() => {
    resetAgentNavigationLoadingForTests()
    vi.useRealTimers()
  })

  it('copies the main-site delayed accessible route progress indicator', async () => {
    const state = useAgentNavigationLoadingState()
    const wrapper = mount(AgentNavigationProgress, { attachTo: document.body })
    const progress = wrapper.get('[role="progressbar"]')

    expect(progress.attributes('aria-label')).toBe('页面加载中')
    expect(progress.attributes('aria-hidden')).toBe('true')
    expect(progress.attributes('data-loading')).toBe('false')

    state.startNavigation()
    vi.advanceTimersByTime(state.ANTI_FLICKER_DELAY)
    await wrapper.vm.$nextTick()
    expect(progress.attributes('aria-hidden')).toBe('false')
    expect(progress.attributes('data-loading')).toBe('true')

    state.endNavigation()
    await wrapper.vm.$nextTick()
    expect(progress.attributes('aria-hidden')).toBe('true')
    expect(progress.attributes('data-loading')).toBe('false')
    wrapper.unmount()
  })
})
