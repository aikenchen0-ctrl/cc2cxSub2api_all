import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  agentOnboardingReplayAvailable,
  agentOnboardingStorageKey,
  clearAgentOnboardingComplete,
  isAgentOnboardingComplete,
  markAgentOnboardingComplete,
  replayAgentOnboarding,
  setAgentOnboardingReplay,
} from './onboarding'

describe('agent onboarding state', () => {
  beforeEach(() => {
    localStorage.clear()
    setAgentOnboardingReplay(null)
  })

  it('keeps user and tenant-admin completion state separate', () => {
    expect(agentOnboardingStorageKey('user')).not.toBe(agentOnboardingStorageKey('admin'))
    expect(isAgentOnboardingComplete('user')).toBe(false)
    expect(isAgentOnboardingComplete('admin')).toBe(false)

    markAgentOnboardingComplete('user')
    expect(isAgentOnboardingComplete('user')).toBe(true)
    expect(isAgentOnboardingComplete('admin')).toBe(false)

    clearAgentOnboardingComplete('user')
    expect(isAgentOnboardingComplete('user')).toBe(false)
  })

  it('exposes replay only while the copied layout has registered a safe handler', () => {
    const replay = vi.fn()
    expect(agentOnboardingReplayAvailable.value).toBe(false)
    expect(replayAgentOnboarding()).toBe(false)

    setAgentOnboardingReplay(replay)
    expect(agentOnboardingReplayAvailable.value).toBe(true)
    expect(replayAgentOnboarding()).toBe(true)
    expect(replay).toHaveBeenCalledTimes(1)

    setAgentOnboardingReplay(null)
    expect(agentOnboardingReplayAvailable.value).toBe(false)
    expect(replayAgentOnboarding()).toBe(false)
  })
})
