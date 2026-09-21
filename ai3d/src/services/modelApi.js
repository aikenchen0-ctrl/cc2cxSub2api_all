import {
  DEFAULT_HUNYUAN_SKETCH_PROMPT,
  GENERATION_MODE_IDS,
  GENERATION_MODE_OPTIONS,
  GENERATION_POLL_INTERVAL_MS,
  GENERATION_PROVIDER_OPTIONS,
  GENERATION_TIMEOUT_MS,
  MODEL_API_BASE,
} from '../config/appConfig.js'

export function apiUrl(path) {
  if (/^https?:\/\//i.test(path)) return path
  const normalized = path.startsWith('/') ? path : `/${path}`
  if (!normalized.startsWith('/api/')) return normalized
  return `${MODEL_API_BASE.replace(/\/$/, '')}${normalized}`
}

export function delay(ms) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

export async function readApiResponse(response) {
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload.error) {
    throw new Error(payload.error || `Request failed with ${response.status}`)
  }
  return payload
}

export function getProviderPlan(provider) {
  if (provider === 'hunyuan-sketch') return ['hunyuan']
  return provider === 'auto' ? ['hunyuan', 'tripo', 'fal', 'rodin', 'cinematic'] : [provider || 'hunyuan']
}

export function getGenerationRequestOptions(mode, prompt = '') {
  if (mode === 'hunyuan-sketch') {
    return {
      provider: 'hunyuan',
      model: '3.0',
      generateType: 'Sketch',
      prompt: String(prompt || '').trim() || DEFAULT_HUNYUAN_SKETCH_PROMPT,
    }
  }

  if (mode === 'hunyuan') {
    return {
      provider: 'hunyuan',
      model: '3.1',
      generateType: 'Normal',
      prompt: '',
    }
  }

  return { provider: mode || 'hunyuan' }
}

export function getAvailableGenerationModes(apiHealth, selectedMode) {
  const providers = apiHealth?.providers
  const configured = {
    hunyuan: Boolean(providers?.hunyuan?.configured),
    tripo: Boolean(providers?.tripo?.configured),
    fal: Boolean(providers?.fal?.configured),
    rodin: Boolean(providers?.rodin?.configured),
  }

  return GENERATION_MODE_OPTIONS.filter((mode) => {
    if (mode.id === 'cinematic') return false
    if (mode.id === 'hunyuan' || mode.id === 'hunyuan-sketch' || mode.id === 'auto' || mode.id === 'local') {
      return !providers || configured.hunyuan
    }
    if (mode.id === selectedMode) return true
    if (!providers) return mode.id === 'tripo'
    return Boolean(configured[mode.id])
  })
}

export function resolveGenerationMode(mode, apiHealth) {
  const requested = GENERATION_MODE_IDS.has(mode) ? mode : 'hunyuan'
  if (requested === 'local') return 'hunyuan'
  if (requested === 'cinematic') return 'hunyuan'
  if (requested === 'hunyuan' || requested === 'hunyuan-sketch' || requested === 'auto') return requested
  if (!apiHealth?.providers) return requested

  const available = getAvailableGenerationModes(apiHealth, requested)
  if (available.some((item) => item.id === requested)) return requested
  return 'hunyuan'
}

export function getProviderLabel(provider) {
  if (provider === 'local') return 'Local'
  if (provider === 'cinematic') return 'JS Depth'
  if (provider === 'reference') return 'Khronos Reference'
  return GENERATION_PROVIDER_OPTIONS.find((item) => item.id === provider)?.label ?? 'Hyper3D'
}

export async function create3dGeneration({ provider, imageDataUrl, fileName, prompt, modelId, model, generateType }) {
  const response = await fetch(apiUrl('/api/3d/generate'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider, imageDataUrl, fileName, prompt, modelId, model, generateType }),
  })

  return readApiResponse(response)
}

export async function analyzeAssetImage({ imageDataUrl, fileName }) {
  const response = await fetch(apiUrl('/api/3d/analyze'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ imageDataUrl, fileName }),
  })

  return readApiResponse(response)
}

export async function uploadLocal3dModel(file) {
  const response = await fetch(apiUrl(`/api/3d/local-model?fileName=${encodeURIComponent(file.name)}`), {
    method: 'POST',
    headers: { 'Content-Type': file.type || 'model/gltf-binary' },
    body: file,
  })

  return readApiResponse(response)
}

export async function get3dApiHealth() {
  const response = await fetch(apiUrl('/api/3d/health'))
  return readApiResponse(response)
}

export async function get3dServerLogs(limit = 100) {
  const response = await fetch(apiUrl(`/api/3d/logs?limit=${encodeURIComponent(limit)}`))
  return readApiResponse(response)
}

export async function get3dGenerationStatus(taskId, provider) {
  const response = await fetch(apiUrl(`/api/3d/status/${encodeURIComponent(taskId)}?provider=${encodeURIComponent(provider || 'rodin')}`))
  return readApiResponse(response)
}

export async function waitFor3dModel(taskId, provider, onStatus) {
  const deadline = Date.now() + GENERATION_TIMEOUT_MS
  let firstPoll = true

  while (Date.now() < deadline) {
    if (!firstPoll) await delay(GENERATION_POLL_INTERVAL_MS)
    firstPoll = false
    const status = await get3dGenerationStatus(taskId, provider)
    onStatus?.(status)

    if (['success', 'succeeded', 'completed', 'complete', 'done', 'finish', 'finished'].includes(String(status.status).toLowerCase())) {
      if (!status.modelUrl) throw new Error(`${getProviderLabel(provider)} finished but no GLB model URL was returned.`)
      return status
    }

    if (['failed', 'error', 'cancelled', 'canceled'].includes(String(status.status).toLowerCase())) {
      throw new Error(status.error || `${getProviderLabel(provider)} generation failed.`)
    }
  }

  throw new Error(`${getProviderLabel(provider)} generation timed out.`)
}
