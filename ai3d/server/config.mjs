import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { ProxyAgent } from 'undici'

const PROJECT_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

loadLocalEnv()

export const API_PORT = Number(process.env.API_PORT || 8787)
export const API_HOST = process.env.API_HOST || '127.0.0.1'
export const BODY_LIMIT = 28 * 1024 * 1024
export const MODEL_UPLOAD_LIMIT = 180 * 1024 * 1024
export const TRIPO_API_KEY = process.env.TRIPO_API_KEY
export const TRIPO_API_BASE = process.env.TRIPO_API_BASE || 'https://api.tripo3d.ai/v2/openapi'
export const TRIPO_MODEL_VERSION = process.env.TRIPO_MODEL_VERSION || 'v3.0-20250812'
export const RODIN_API_KEY = process.env.RODIN_API_KEY
export const RODIN_API_BASE = process.env.RODIN_API_BASE || 'https://api.hyper3d.com/api/v2'
export const RODIN_TIER = process.env.RODIN_TIER || 'Gen-2'
export const RODIN_QUALITY = process.env.RODIN_QUALITY || 'medium'
export const RODIN_MESH_MODE = process.env.RODIN_MESH_MODE || 'Raw'
export const RODIN_MATERIAL = process.env.RODIN_MATERIAL || 'PBR'
// An empty value means that no local Hunyuan service is available.  Do not
// silently point at loopback: inside the satellite container 127.0.0.1 is the
// AI3D process itself, which made health checks report a configured provider
// that could never accept a request.
export const HUNYUAN_API_BASE = process.env.HUNYUAN_API_BASE || ''
export const HUNYUAN_CREATE_PATH = process.env.HUNYUAN_CREATE_PATH || '/send'
export const HUNYUAN_STATUS_PATH = process.env.HUNYUAN_STATUS_PATH || '/status'
export const HUNYUAN_CLOUD_API_KEY = process.env.HUNYUAN_CLOUD_API_KEY
export const HUNYUAN_CLOUD_API_BASE = process.env.HUNYUAN_CLOUD_API_BASE || 'https://api.ai3d.cloud.tencent.com'
export const HUNYUAN_CLOUD_SUBMIT_PATH = process.env.HUNYUAN_CLOUD_SUBMIT_PATH || '/v1/ai3d/submit'
export const HUNYUAN_CLOUD_QUERY_PATH = process.env.HUNYUAN_CLOUD_QUERY_PATH || '/v1/ai3d/query'
export const HUNYUAN_CLOUD_MODEL = process.env.HUNYUAN_CLOUD_MODEL || '3.0'
export const HUNYUAN_CLOUD_GENERATE_TYPE = process.env.HUNYUAN_CLOUD_GENERATE_TYPE || 'Sketch'
export const HUNYUAN_CLOUD_ENABLE_PBR = process.env.HUNYUAN_CLOUD_ENABLE_PBR !== 'false'
export const HUNYUAN_CLOUD_FACE_COUNT = Number(process.env.HUNYUAN_CLOUD_FACE_COUNT || 30000)
export const FAL_API_KEY = process.env.FAL_API_KEY || process.env.FAL_KEY
export const FAL_DEFAULT_MODEL = process.env.FAL_DEFAULT_MODEL || 'fal-ai/hunyuan3d/v2'
export const VISION_PROVIDER = process.env.VISION_PROVIDER || 'openai'
export const OPENAI_API_KEY = process.env.OPENAI_API_KEY
export const OPENAI_API_BASE = process.env.OPENAI_API_BASE || 'https://api.openai.com/v1'
// Keep the standalone vision default aligned with the public satellite model
// catalogue.  Managed deployments use SUB2API_RELAY_MODEL, while local mode
// should still exercise the same public model name by default.
export const OPENAI_VISION_MODEL = process.env.OPENAI_VISION_MODEL || 'gpt-5.5'
export const LOCAL_MODEL_DIR = path.resolve(PROJECT_ROOT, process.env.LOCAL_MODEL_DIR || '.generated-models')
export const LOG_DIR = path.resolve(PROJECT_ROOT, process.env.LOG_DIR || '.logs')
export const LOG_FILE = path.resolve(LOG_DIR, process.env.LOG_FILE || '3d-model-studio-api.log')
export const OUTBOUND_PROXY_AGENT = createProxyAgent()

// .env.example contains readable placeholders; those must not make a
// provider appear configured when the first request would fail.
export function hasConfiguredSecret(value) {
  const normalized = String(value || '').trim().toLowerCase()
  if (!normalized) return false
  return !(
    normalized === 'changeme' ||
    normalized === 'change-me' ||
    normalized === 'replace-me' ||
    normalized === 'your-key' ||
    normalized === 'your_api_key' ||
    normalized === 'your-api-key' ||
    normalized.startsWith('your_') ||
    normalized.startsWith('your-') ||
    normalized.startsWith('replace-with-') ||
    normalized.startsWith('example-') ||
    normalized.startsWith('sk-super-') ||
    normalized === 'xxx' ||
    normalized === 'sk-xxx'
  )
}

export function hasOutboundProxy() {
  return Boolean(process.env.HTTPS_PROXY || process.env.https_proxy || process.env.HTTP_PROXY || process.env.http_proxy)
}

function loadLocalEnv() {
  const envPath = path.join(PROJECT_ROOT, '.env.local')
  if (!existsSync(envPath)) return

  const env = readFileSync(envPath, 'utf8')
  for (const line of env.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue

    const index = trimmed.indexOf('=')
    if (index === -1) continue

            const key = trimmed.slice(0, index).trim()
            let value = trimmed.slice(index + 1).trim()
            value = value.replace(/^["']|["']$/g, '')
            // Local project config wins over inherited shell/sandbox proxies.
            const forceOverride = ['HTTPS_PROXY', 'HTTP_PROXY', 'https_proxy', 'http_proxy', 'NO_PROXY', 'no_proxy'].includes(key)
            if (!process.env[key] || forceOverride) process.env[key] = value
  }
}

function createProxyAgent() {
  const proxy = process.env.HTTPS_PROXY || process.env.https_proxy || process.env.HTTP_PROXY || process.env.http_proxy
  if (!proxy) return null

  return new ProxyAgent(proxy)
}
