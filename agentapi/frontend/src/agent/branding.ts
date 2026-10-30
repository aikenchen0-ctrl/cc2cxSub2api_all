import { ref } from 'vue'
import { agentAPI, type AgentContextResponse, type AgentHomeSettings, type AgentView, type PublicSettings } from './api'

export const siteName = ref('AgentAPI')
export const siteLogo = ref('/logo.svg')
export const siteDocURL = ref('')
export const siteContactInfo = ref('')
export const siteSubtitle = ref('AI API Gateway Platform')
export const compactHomeEnabled = ref(false)
export const homeContent = ref('')
let pageTitle = ''

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
  // eslint-disable-next-line no-control-regex -- reject control characters in plain-text contact details.
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
  refreshDocumentTitle()
}

function refreshDocumentTitle(): void {
  document.title = pageTitle ? `${pageTitle} · ${siteName.value}` : siteName.value
}

export function applyPageTitle(nextPageTitle?: string): void {
  if (nextPageTitle !== undefined) pageTitle = nextPageTitle.trim()
  refreshDocumentTitle()
}

// Applying tenant context is safe only after the server has resolved the
// current session or a persisted custom domain to one tenant.
export function applyAgentBranding(agent: AgentView): void {
  applyBranding(agent.site_name || agent.name, agent.site_logo, agent.doc_url, agent.contact_info)
  applyHomeSettings(agent)
}

export function resetBranding(): void {
  applyBranding('AgentAPI', '/logo.svg')
  applyHomeSettings({})
}

export async function loadBranding(): Promise<PublicSettings | null> {
  try {
    const settings = await agentAPI.getPublicSettings()
    applyBranding(settings.site_name, settings.site_logo, settings.doc_url, settings.contact_info)
    applyHomeSettings(settings)
    return settings
  } catch {
    // Shared AgentAPI hosts cannot resolve an anonymous visitor to a tenant.
    // A valid session can, so retry through the tenant-bound context endpoint.
    try {
      const context = await agentAPI.getContext()
      applyAgentBranding(context.agent)
    } catch {
      resetBranding()
    }
    return null
  }
}

export async function loadSessionBranding(isCurrent: () => boolean = () => true): Promise<AgentContextResponse | null> {
  try {
    const context = await agentAPI.getContext()
    if (!isCurrent()) return null
    applyAgentBranding(context.agent)
    return context
  } catch {
    return null
  }
}
