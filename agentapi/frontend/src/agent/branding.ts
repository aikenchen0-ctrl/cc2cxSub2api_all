import { ref } from 'vue'
import { agentAPI, type PublicSettings } from './api'

export const siteName = ref('AgentAPI')
export const siteLogo = ref('/logo.svg')

function absoluteAssetURL(value: string): string {
  try {
    return new URL(value || '/logo.svg', window.location.origin).href
  } catch {
    return new URL('/logo.svg', window.location.origin).href
  }
}

export function applyBranding(name: string, logo?: string): void {
  siteName.value = name.trim() || 'AgentAPI'
  siteLogo.value = logo?.trim() || '/logo.svg'
  const icon = document.querySelector<HTMLLinkElement>('link[rel~="icon"]') || document.createElement('link')
  icon.rel = 'icon'
  icon.href = absoluteAssetURL(siteLogo.value)
  if (!icon.parentNode) document.head.appendChild(icon)
}

export function applyPageTitle(pageTitle?: string): void {
  document.title = pageTitle ? `${pageTitle} · ${siteName.value}` : siteName.value
}

export async function loadBranding(): Promise<PublicSettings | null> {
  try {
    const settings = await agentAPI.getPublicSettings()
    applyBranding(settings.site_name, settings.site_logo)
    return settings
  } catch {
    applyBranding(siteName.value, siteLogo.value)
    return null
  }
}
