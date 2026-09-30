import { ref } from 'vue'

const themeStorageKey = 'theme'
const legacyThemeStorageKey = 'agentapi_theme'
type ThemePreference = 'dark' | 'light'

export const isDark = ref(false)

function normalizeTheme(value: string | null): ThemePreference | null {
  return value === 'dark' || value === 'light' ? value : null
}

function applyTheme(preference: ThemePreference): void {
  isDark.value = preference === 'dark'
  document.documentElement.classList.toggle('dark', isDark.value)
}

export function initializeTheme(): void {
  let savedTheme = normalizeTheme(localStorage.getItem(themeStorageKey))
  if (!savedTheme) {
    const legacyTheme = normalizeTheme(sessionStorage.getItem(legacyThemeStorageKey))
    if (legacyTheme) {
      savedTheme = legacyTheme
      localStorage.setItem(themeStorageKey, legacyTheme)
      sessionStorage.removeItem(legacyThemeStorageKey)
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
