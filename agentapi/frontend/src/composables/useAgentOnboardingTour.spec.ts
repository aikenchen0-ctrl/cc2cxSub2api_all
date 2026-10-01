import { beforeEach, describe, expect, it, vi } from 'vitest'
import { agentOnboardingSteps, useAgentOnboardingTour } from './useAgentOnboardingTour'
import { agentOnboardingStorageKey } from '@/agent/onboarding'

const driverMock = vi.hoisted(() => vi.fn())

vi.mock('driver.js', () => ({ driver: driverMock }))

describe('useAgentOnboardingTour', () => {
  beforeEach(() => {
    localStorage.clear()
    document.body.innerHTML = [
      '<a data-tour="sidebar-dashboard"></a>',
      '<a data-tour="sidebar-api-keys"></a>',
      '<a data-tour="sidebar-usage"></a>',
      '<a data-tour="sidebar-profile"></a>',
      '<a data-tour="sidebar-admin-dashboard"></a>',
      '<button data-tour="header-user-menu"></button>',
    ].join('')
    driverMock.mockReset()
  })

  it('copies the main-site guide around real tenant-safe user and admin destinations', () => {
    const userSelectors = agentOnboardingSteps('user').map(step => step.element).filter(Boolean)
    const adminSelectors = agentOnboardingSteps('admin').map(step => step.element).filter(Boolean)

    expect(userSelectors).toContain('[data-tour="sidebar-api-keys"]')
    expect(userSelectors).toContain('[data-tour="sidebar-usage"]')
    expect(userSelectors).not.toContain('[data-tour="sidebar-admin-dashboard"]')
    expect(adminSelectors).toContain('[data-tour="sidebar-admin-dashboard"]')
    expect(JSON.stringify(agentOnboardingSteps('admin'))).not.toMatch(/SuperKey|admin[_ -]?key|credential/i)
  })

  it('starts once, records dismissal, and allows an explicit replay', async () => {
    let config: { onDestroyed?: () => void } | undefined
    const instance = {
      drive: vi.fn(),
      destroy: vi.fn(),
      isActive: vi.fn(() => false),
    }
    driverMock.mockImplementation((value) => { config = value; return instance })

    const tour = useAgentOnboardingTour('user')
    expect(await tour.startTour()).toBe(true)
    expect(instance.drive).toHaveBeenCalledTimes(1)
    config?.onDestroyed?.()
    expect(localStorage.getItem(agentOnboardingStorageKey('user'))).toBe('true')

    expect(await tour.startTour()).toBe(false)
    tour.replayTour()
    await Promise.resolve()
    await Promise.resolve()
    expect(instance.drive).toHaveBeenCalledTimes(2)
  })
})
