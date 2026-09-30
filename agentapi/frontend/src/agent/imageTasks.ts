import type { ImageGenerationResponse, ImageTaskResponse } from './api'

export interface AgentImageResult {
  url: string
  revisedPrompt?: string
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined
}

export function imageTaskField(response: ImageTaskResponse | null | undefined, ...keys: string[]): string {
  if (!response) return ''
  const root = response as Record<string, unknown>
  const nested = asRecord(root.data)
  for (const key of keys) {
    const value = root[key] ?? nested?.[key]
    if (typeof value === 'string' || typeof value === 'number') {
      const text = String(value).trim()
      if (text) return text
    }
  }
  return ''
}

export function imageTaskID(response: ImageTaskResponse | null | undefined): string {
  return imageTaskField(response, 'id', 'task_id')
}

export function imageTaskStatus(response: ImageTaskResponse | null | undefined): string {
  return imageTaskField(response, 'status', 'state').toLowerCase()
}

export function imageTaskIsTerminal(status: string): boolean {
  return ['done', 'completed', 'complete', 'succeeded', 'success', 'failed', 'error', 'cancelled', 'canceled', 'rejected', 'expired'].includes(status.trim().toLowerCase())
}

export function safeImageURL(value: unknown): string {
  if (typeof value !== 'string') return ''
  const url = value.trim()
  if (/^https?:\/\//i.test(url)) return url
  if (/^data:image\/(png|jpeg|jpg|webp|gif);base64,/i.test(url)) return url
  return ''
}

export function imageResultsFrom(response: ImageGenerationResponse | ImageTaskResponse | null | undefined): AgentImageResult[] {
  if (!response) return []
  const root = response as Record<string, unknown>
  const responseData = root.result ?? root.data
  const nested = asRecord(responseData)
  const items = Array.isArray(responseData)
    ? responseData
    : Array.isArray(nested?.data)
      ? nested.data
      : []

  return items.flatMap((rawItem) => {
    const item = asRecord(rawItem)
    if (!item) return []
    let url = safeImageURL(item.url)
    if (!url && typeof item.b64_json === 'string' && item.b64_json.trim()) {
      const format = String(item.output_format || '').toLowerCase()
      const mimeCandidate = typeof item.mime_type === 'string' ? item.mime_type.toLowerCase() : ''
      const mime = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(mimeCandidate)
        ? mimeCandidate
        : format === 'jpeg' || format === 'jpg'
          ? 'image/jpeg'
          : format === 'webp'
            ? 'image/webp'
            : 'image/png'
      url = `data:${mime};base64,${item.b64_json}`
    }
    return url
      ? [{ url, revisedPrompt: typeof item.revised_prompt === 'string' ? item.revised_prompt : undefined }]
      : []
  })
}
