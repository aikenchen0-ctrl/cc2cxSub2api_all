const STORAGE_PREFIX = 'cc2cx-ai3d'

// The namespace is populated by main.jsx after the HttpOnly session has been
// checked. It is deliberately a stable local identity id, never a JWT, API
// key, or SuperKey.
export function getStorageNamespace() {
  const raw = String(globalThis.__AI3D_USER_NAMESPACE__ || 'anonymous').trim()
  return raw.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 96) || 'anonymous'
}

export function scopedStorageKey(key) {
  return `${STORAGE_PREFIX}:${getStorageNamespace()}:${String(key)}`
}

export function loadStoredValue(key, fallback) {
  try {
    const scopedKey = scopedStorageKey(key)
    let raw = window.localStorage.getItem(scopedKey)
    // Preserve data from the pre-SSO local-only build for an unmanaged local
    // session. Never migrate the legacy global keys into a managed identity.
    if (raw === null && getStorageNamespace() === 'local') {
      raw = window.localStorage.getItem(key)
      if (raw !== null) window.localStorage.setItem(scopedKey, raw)
    }
    return raw ? JSON.parse(raw) : fallback
  } catch {
    return fallback
  }
}

export function storeValue(key, value) {
  try {
    window.localStorage.setItem(scopedStorageKey(key), JSON.stringify(value))
    return true
  } catch {
    // Storage can fail in private browsing; the UI should keep working.
    return false
  }
}
