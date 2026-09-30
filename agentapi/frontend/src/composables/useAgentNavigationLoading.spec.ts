import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAgentNavigationLoading } from './useAgentNavigationLoading'

describe('agent navigation loading state', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('suppresses fast route changes to avoid a progress flash', () => {
    vi.useFakeTimers()
    const state = createAgentNavigationLoading()

    state.startNavigation()
    expect(state.isNavigating.value).toBe(true)
    expect(state.isLoading.value).toBe(false)
    vi.advanceTimersByTime(99)
    state.endNavigation()
    vi.advanceTimersByTime(1)

    expect(state.isNavigating.value).toBe(false)
    expect(state.isLoading.value).toBe(false)
  })

  it('shows after the anti-flicker delay and resets on completion', () => {
    vi.useFakeTimers()
    const state = createAgentNavigationLoading()

    state.startNavigation()
    vi.advanceTimersByTime(state.ANTI_FLICKER_DELAY)
    expect(state.isLoading.value).toBe(true)

    state.endNavigation()
    expect(state.isLoading.value).toBe(false)
  })

  it('restarts the delay for consecutive navigations', () => {
    vi.useFakeTimers()
    const state = createAgentNavigationLoading()

    state.startNavigation()
    vi.advanceTimersByTime(80)
    state.startNavigation()
    vi.advanceTimersByTime(20)
    expect(state.isLoading.value).toBe(false)
    vi.advanceTimersByTime(80)
    expect(state.isLoading.value).toBe(true)
  })
})
