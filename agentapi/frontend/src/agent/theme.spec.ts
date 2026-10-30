import { beforeEach, describe, expect, it, vi } from 'vitest'
import { initializeTheme, isDark, toggleTheme } from './theme'

describe('main-site compatible theme preference', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    document.documentElement.classList.remove('dark')
    isDark.value = false
    vi.restoreAllMocks()
  })

  it('restores the persistent main-site theme preference', () => {
    localStorage.setItem('theme', 'dark')
    initializeTheme()
    expect(isDark.value).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('migrates the previous tab-scoped AgentAPI preference once', () => {
    sessionStorage.setItem('agentapi_theme', 'light')
    document.documentElement.classList.add('dark')
    initializeTheme()
    expect(isDark.value).toBe(false)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
    expect(sessionStorage.getItem('agentapi_theme')).toBeNull()
  })

  it('migrates the retired landing-page preference before the tab preference', () => {
    localStorage.setItem('agentapi_landing_theme', 'dark')
    sessionStorage.setItem('agentapi_theme', 'light')
    initializeTheme()
    expect(isDark.value).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(localStorage.getItem('agentapi_landing_theme')).toBeNull()
    expect(sessionStorage.getItem('agentapi_theme')).toBeNull()
  })

  it('clears retired preferences without overriding the shared theme', () => {
    localStorage.setItem('theme', 'light')
    localStorage.setItem('agentapi_landing_theme', 'dark')
    sessionStorage.setItem('agentapi_theme', 'dark')
    initializeTheme()
    expect(isDark.value).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
    expect(localStorage.getItem('agentapi_landing_theme')).toBeNull()
    expect(sessionStorage.getItem('agentapi_theme')).toBeNull()
  })

  it('uses the operating system preference when no explicit choice exists', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: true } as MediaQueryList)
    initializeTheme()
    expect(isDark.value).toBe(true)
    expect(localStorage.getItem('theme')).toBeNull()
  })

  it('persists user changes across browser sessions', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
    initializeTheme()
    toggleTheme()
    expect(isDark.value).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    toggleTheme()
    expect(isDark.value).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
  })

  it('keeps mobile browser chrome in sync with the active theme', () => {
    const meta = document.createElement('meta')
    meta.name = 'theme-color'
    document.head.appendChild(meta)

    localStorage.setItem('theme', 'dark')
    initializeTheme()
    expect(meta.content).toBe('#0e0c09')

    toggleTheme()
    expect(meta.content).toBe('#fbfaf7')

    meta.remove()
  })
})
