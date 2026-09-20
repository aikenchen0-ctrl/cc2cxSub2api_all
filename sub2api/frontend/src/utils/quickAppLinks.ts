const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '0.0.0.0', '::1'])

function envUrl(name: keyof ImportMetaEnv): string | undefined {
  const value = import.meta.env[name]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function normalizeLink(value: string | undefined): string | undefined {
  const raw = value?.trim()
  if (!raw) return undefined
  return /^https?:\/\//i.test(raw) ? raw.replace(/\/$/, '') : `http://${raw}`
}

/**
 * Resolve companion-app origins once, so local development and production use
 * the same sidebar code. Production deployments can override these values at
 * build time without editing source or database menu settings.
 */
export function getQuickAppOrigins(hostname = window.location.hostname, dev = import.meta.env.DEV) {
  const isLocal = dev || LOCAL_HOSTS.has(hostname.toLowerCase())
  const configuredApi = normalizeLink(envUrl('VITE_LINK') || envUrl('VITE_PUBLIC_API_URL'))
  return {
    isLocal,
    canvas: normalizeLink(envUrl('VITE_CANVAS_LINK') || envUrl('VITE_CANVAS_URL')) || (isLocal ? 'http://localhost:3522' : 'https://canvas.cc2.cx'),
    qrcode: normalizeLink(envUrl('VITE_QRCODE_LINK') || envUrl('VITE_QRCODE_URL')) || (isLocal ? 'http://localhost:5221' : 'https://qrcode.cc2.cx'),
    api: configuredApi || (isLocal ? 'http://localhost:18080' : 'https://api.cc2.cx'),
  }
}
