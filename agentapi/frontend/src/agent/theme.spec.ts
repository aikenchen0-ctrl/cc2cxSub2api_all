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
})
