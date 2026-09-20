import { appendFile, mkdir, readFile, stat } from 'node:fs/promises'
import { randomUUID } from 'node:crypto'
import path from 'node:path'

import { LOG_DIR, LOG_FILE } from './config.mjs'

const MAX_LOG_READ_BYTES = 768 * 1024
const SENSITIVE_KEYS = new Set([
  'authorization',
  'cookie',
  'imageDataUrl',
  'modelBase64',
  'TRIPO_API_KEY',
  'RODIN_API_KEY',
  'FAL_API_KEY',
  'OPENAI_API_KEY',
])

export function createRequestId() {
  return randomUUID().slice(0, 12)
}

export async function logEvent(level, event, fields = {}) {
  const entry = {
    ts: new Date().toISOString(),
    level,
    event,
    ...sanitizeLogValue(fields),
  }

  try {
    await mkdir(LOG_DIR, { recursive: true })
    await appendFile(LOG_FILE, `${JSON.stringify(entry)}\n`, 'utf8')
  } catch (error) {
    console.warn('log write failed', error)
  }

  return entry
}

export async function readRecentLogs(limit = 100) {
  try {
    const fileStat = await stat(LOG_FILE)
    const content = await readFile(LOG_FILE, 'utf8')
    const slice = content.length > MAX_LOG_READ_BYTES ? content.slice(-MAX_LOG_READ_BYTES) : content
    const lines = slice.trim().split(/\r?\n/).filter(Boolean)
    const entries = lines.slice(-normalizeLimit(limit)).map(parseLogLine).filter(Boolean)

    return {
      file: path.relative(process.cwd(), LOG_FILE),
      size: fileStat.size,
      entries,
    }
  } catch {
    return {
      file: path.relative(process.cwd(), LOG_FILE),
      size: 0,
      entries: [],
    }
  }
}

export function summarizePayload(payload = {}) {
  return {
    provider: payload.provider,
    modelId: payload.modelId,
    fileName: payload.fileName,
    hasImage: typeof payload.imageDataUrl === 'string',
    imageBytes: estimateDataUrlBytes(payload.imageDataUrl),
    promptChars: typeof payload.prompt === 'string' ? payload.prompt.length : 0,
  }
}

export function summarizeError(error) {
  if (!error) return {}

  return {
    message: error.message || 'Unknown error',
    status: error.status,
    detail: sanitizeLogValue(error.detail),
  }
}

function normalizeLimit(limit) {
  const value = Number(limit)
  if (!Number.isFinite(value)) return 100
  return Math.max(1, Math.min(500, Math.round(value)))
}

function parseLogLine(line) {
  try {
    return JSON.parse(line)
  } catch {
    return null
  }
}

function estimateDataUrlBytes(value) {
  if (typeof value !== 'string') return 0
  const comma = value.indexOf(',')
  const base64 = comma === -1 ? value : value.slice(comma + 1)
  return Math.round((base64.length * 3) / 4)
}

function sanitizeLogValue(value, key = '') {
  if (value === null || value === undefined) return value
  if (SENSITIVE_KEYS.has(key)) return '[redacted]'
  if (typeof value === 'string') {
    if (value.startsWith('data:image/')) return `[image-data:${estimateDataUrlBytes(value)} bytes]`
    if (value.length > 900) return `${value.slice(0, 900)}...`
    return value
  }
  if (typeof value !== 'object') return value
  if (Array.isArray(value)) return value.slice(0, 30).map((item) => sanitizeLogValue(item))

  return Object.fromEntries(
    Object.entries(value)
      .slice(0, 80)
      .map(([entryKey, entryValue]) => [entryKey, sanitizeLogValue(entryValue, entryKey)]),
  )
}
