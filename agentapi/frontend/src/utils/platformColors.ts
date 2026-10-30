/**
 * Centralized platform styling for AgentAPI surfaces.
 *
 * Platform identity is conveyed by the icon and label. Brand surfaces stay in
 * the white-and-gold palette so plan cards and payment flows match the console.
 */

export const PLATFORMS = [
  'anthropic',
  'openai',
  'antigravity',
  'gemini',
  'grok',
  'kimi',
  'zhipu',
  'deepseek',
  'minimax',
  'opencode_go',
  'composite',
] as const

export type Platform = (typeof PLATFORMS)[number]

type PlatformStyle = Record<Platform, string>

function everyPlatform(value: string): PlatformStyle {
  return Object.fromEntries(PLATFORMS.map(platform => [platform, value])) as PlatformStyle
}

const BADGE = everyPlatform('bg-primary-500/10 text-primary-700 border-primary-500/30 dark:text-primary-300')
const BADGE_LIGHT = everyPlatform('bg-primary-500/10 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300')
const BORDER = everyPlatform('border-primary-500/20 dark:border-primary-500/20')
const BORDER_STRONG = everyPlatform('border-primary-500/35 dark:border-primary-500/30')
const ACCENT = everyPlatform('#b38a2b')
const ACCENT_BAR = everyPlatform('bg-gradient-to-r from-primary-300 to-primary-500')
const TEXT = everyPlatform('text-primary-700 dark:text-primary-300')
const ICON = everyPlatform('text-primary-600 dark:text-primary-400')
const BUTTON = everyPlatform('bg-primary-400 text-accent-950 hover:bg-primary-500 active:bg-primary-600 dark:bg-primary-400 dark:text-dark-950 dark:hover:bg-primary-300')
const DISCOUNT = everyPlatform('bg-primary-100 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300')
const GRADIENT = everyPlatform('from-primary-300 to-primary-500')
const GRADIENT_TEXT = everyPlatform('text-accent-950')
const GRADIENT_SUBTEXT = everyPlatform('text-accent-800')

const BADGE_DEFAULT = 'bg-gray-500/10 text-gray-600 border-gray-500/30 dark:text-gray-400'
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'
const BORDER_STRONG_DEFAULT = 'border-gray-300 dark:border-dark-600'
const ACCENT_DEFAULT = '#b38a2b'
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-300 to-primary-500'
const TEXT_DEFAULT = 'text-primary-700 dark:text-primary-300'
const ICON_DEFAULT = 'text-primary-600 dark:text-primary-400'
const BUTTON_DEFAULT = 'bg-primary-400 text-accent-950 hover:bg-primary-500 dark:bg-primary-400 dark:text-dark-950 dark:hover:bg-primary-300'
const DISCOUNT_DEFAULT = 'bg-primary-100 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300'
const GRADIENT_DEFAULT = 'from-primary-300 to-primary-500'
const GRADIENT_TEXT_DEFAULT = 'text-accent-950'
const GRADIENT_SUBTEXT_DEFAULT = 'text-accent-800'

function isPlatform(value: string): value is Platform {
  return (PLATFORMS as readonly string[]).includes(value)
}

export function platformBadgeClass(platform: string): string {
  return isPlatform(platform) ? BADGE[platform] : BADGE_DEFAULT
}

export function platformBadgeLightClass(platform: string): string {
  return isPlatform(platform) ? BADGE_LIGHT[platform] : BADGE_DEFAULT
}

export function platformBorderClass(platform: string): string {
  return isPlatform(platform) ? BORDER[platform] : BORDER_DEFAULT
}

export function platformBorderStrongClass(platform: string): string {
  return isPlatform(platform) ? BORDER_STRONG[platform] : BORDER_STRONG_DEFAULT
}

export function platformAccentColor(platform: string): string {
  return isPlatform(platform) ? ACCENT[platform] : ACCENT_DEFAULT
}

export function platformAccentBarClass(platform: string): string {
  return isPlatform(platform) ? ACCENT_BAR[platform] : ACCENT_BAR_DEFAULT
}

export function platformTextClass(platform: string): string {
  return isPlatform(platform) ? TEXT[platform] : TEXT_DEFAULT
}

export function platformIconClass(platform: string): string {
  return isPlatform(platform) ? ICON[platform] : ICON_DEFAULT
}

export function platformButtonClass(platform: string): string {
  return isPlatform(platform) ? BUTTON[platform] : BUTTON_DEFAULT
}

export function platformDiscountClass(platform: string): string {
  return isPlatform(platform) ? DISCOUNT[platform] : DISCOUNT_DEFAULT
}

export function platformGradientClass(platform: string): string {
  return isPlatform(platform) ? GRADIENT[platform] : GRADIENT_DEFAULT
}

export function platformGradientTextClass(platform: string): string {
  return isPlatform(platform) ? GRADIENT_TEXT[platform] : GRADIENT_TEXT_DEFAULT
}

export function platformGradientSubtextClass(platform: string): string {
  return isPlatform(platform) ? GRADIENT_SUBTEXT[platform] : GRADIENT_SUBTEXT_DEFAULT
}

export function platformLabel(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return '[OI]'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    case 'grok': return 'Grok'
    case 'kimi': return 'Kimi'
    case 'zhipu': return 'Zhipu GLM'
    case 'deepseek': return 'DeepSeek'
    case 'minimax': return 'MiniMax'
    case 'opencode_go': return 'OpenCode'
    case 'composite': return 'Composite'
    default: return platform || 'API'
  }
}