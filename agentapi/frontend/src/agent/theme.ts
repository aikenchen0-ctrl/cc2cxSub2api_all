import { ref } from 'vue'

const themeStorageKey = 'theme'
const legacyThemeStorageKeys = [
  { storage: () => localStorage, key: 'agentapi_landing_theme' },
  { storage: () => sessionStorage, key: 'agentapi_theme' },
] as const
type ThemePreference = 'dark' | 'light'
const themeColors: Record<ThemePreference, string> = {
  light: '#fbfaf7',
  dark: '#0e0c09',
}

export const isDark = ref(false)

function normalizeTheme(value: string | null): ThemePreference | null {
  return value === 'dark' || value === 'light' ? value : null
}

function applyTheme(preference: ThemePreference): void {
  isDark.value = preference === 'dark'
  document.documentElement.classList.toggle('dark', isDark.value)
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', themeColors[preference])
}

export function initializeTheme(): void {
  let savedTheme = normalizeTheme(localStorage.getItem(themeStorageKey))
  for (const legacy of legacyThemeStorageKeys) {
    const storage = legacy.storage()
    const legacyTheme = normalizeTheme(storage.getItem(legacy.key))
    if (legacyTheme) {
      if (!savedTheme) {
        savedTheme = legacyTheme
        localStorage.setItem(themeStorageKey, legacyTheme)
      }
      storage.removeItem(legacy.key)
    }
  }
  const preference = savedTheme || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  applyTheme(preference)
}

export function toggleTheme(): void {
  const preference: ThemePreference = isDark.value ? 'light' : 'dark'
  applyTheme(preference)
  localStorage.setItem(themeStorageKey, preference)
}
