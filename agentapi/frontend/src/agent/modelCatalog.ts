export type AgentModelCapability = 'text' | 'image' | 'video'

export interface AgentCatalogModel {
  name: string
  capability: AgentModelCapability
  provider: string
}

export interface AgentCatalogGroup {
  provider: string
  models: AgentCatalogModel[]
}

export function modelCapability(name: string): AgentModelCapability {
  const normalized = name.trim().toLowerCase()
  if (normalized.startsWith('grok-imagine-video-') || normalized.startsWith('seedance-') || normalized.startsWith('kling-')) return 'video'
  if (normalized.startsWith('gpt-image-') || normalized === 'gemini-3.1-flash-image' || normalized.startsWith('grok-imagine-image') || normalized === 'grok-imagine') return 'image'
  return 'text'
}

export function modelProvider(name: string): string {
  const normalized = name.trim().toLowerCase()
  if (/^(gpt-|o[134](?:-|$)|chatgpt-|codex)/.test(normalized)) return 'OpenAI'
  if (normalized.startsWith('claude-')) return 'Anthropic'
  if (normalized.startsWith('gemini-')) return 'Google'
  if (normalized.startsWith('grok-') || normalized === 'grok-imagine') return 'xAI'
  if (normalized.startsWith('seedance-') || normalized.startsWith('doubao-')) return 'ByteDance'
  if (normalized.startsWith('kling-')) return 'Kling'
  if (normalized.startsWith('deepseek-')) return 'DeepSeek'
  if (normalized.startsWith('qwen-')) return 'Qwen'
  return '其他'
}

export function groupCatalogModels(models: string[]): AgentCatalogGroup[] {
  const groups = new Map<string, AgentCatalogModel[]>()
  for (const rawName of models) {
    const name = rawName.trim()
    if (!name) continue
    const provider = modelProvider(name)
    const entry = { name, provider, capability: modelCapability(name) }
    groups.set(provider, [...(groups.get(provider) || []), entry])
  }
  return [...groups.entries()]
    .map(([provider, entries]) => ({ provider, models: entries.sort((a, b) => a.name.localeCompare(b.name)) }))
    .sort((a, b) => a.provider.localeCompare(b.provider))
}
