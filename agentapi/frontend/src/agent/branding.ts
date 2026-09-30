import { ref } from 'vue'
import { agentAPI, type AgentHomeSettings, type PublicSettings } from './api'

export const siteName = ref('AgentAPI')
export const siteLogo = ref('/logo.svg')
export const siteDocURL = ref('')
export const siteContactInfo = ref('')
export const siteSubtitle = ref('AI API Gateway Platform')
export const compactHomeEnabled = ref(false)
export const homeContent = ref('')

export function applyHomeSettings(settings: AgentHomeSettings): void {
  siteSubtitle.value = settings.site_subtitle?.trim() || 'AI API Gateway Platform'
  compactHomeEnabled.value = settings.compact_home_enabled === true
  homeContent.value = settings.home_content?.trim() || ''
}

function absoluteAssetURL(value: string): string {
  try {
    return new URL(value || '/logo.svg', window.location.origin).href
  } catch {
    return new URL('/logo.svg', window.location.origin).href
  }
}

function safePublicURL(value?: string): string {
  try {
    const parsed = new URL(value?.trim() || '')
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : ''
  } catch {
    return ''
  }
}

function safeContactInfo(value?: string): string {
  const normalized = value?.trim() || ''
  return normalized.length <= 300 && !/[\u0000-\u001f\u007f]/.test(normalized) ? normalized : ''
}

export function applyBranding(name: string, logo?: string, docURL?: string, contactInfo?: string): void {
  siteName.value = name.trim() || 'AgentAPI'
  siteLogo.value = logo?.trim() || '/logo.svg'
  siteDocURL.value = safePublicURL(docURL)
  siteContactInfo.value = safeContactInfo(contactInfo)
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
    applyBranding(settings.site_name, settings.site_logo, settings.doc_url, settings.contact_info)
    applyHomeSettings(settings)
    return settings
  } catch {
    applyBranding(siteName.value, siteLogo.value, siteDocURL.value, siteContactInfo.value)
    return null
  }
}
