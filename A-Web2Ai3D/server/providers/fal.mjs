import { createFalClient } from '@fal-ai/client'
import { fetch as undiciFetch } from 'undici'

import { FAL_API_KEY, FAL_DEFAULT_MODEL, OUTBOUND_PROXY_AGENT } from '../config.mjs'
import { parseDataUrl } from '../http-utils.mjs'
import { cacheRemoteModelAs, hasLocalModel, localModelUrl } from '../model-store.mjs'
import { isSuccessStatus } from '../object-utils.mjs'

export const FAL_MODEL_DEFINITIONS = [
  {
    id: 'fal-ai/hunyuan3d/v2',
    label: 'Hunyuan3D v2',
    imageField: 'input_image_url',
    defaults: {},
    supportsSeed: true,
  },
  {
    id: 'fal-ai/trellis',
    label: 'TRELLIS',
    imageField: 'image_url',
    defaults: { texture_size: '1024' },
    supportsSeed: true,
  },
  {
    id: 'fal-ai/triposr',
    label: 'TripoSR',
    imageField: 'image_url',
    defaults: { do_remove_background: true, output_format: 'glb' },
    supportsSeed: false,
  },
  {
    id: 'tripo3d/tripo/v2.5/image-to-3d',
    label: 'Tripo3D v2.5',
    imageField: 'image_url',
    defaults: { orientation: 'align_image', pbr: true, texture: 'standard' },
    supportsSeed: true,
  },
  {
    id: 'fal-ai/hyper3d/rodin',
    label: 'Hyper3D Rodin',
    imageField: 'input_image_urls',
    defaults: {
      geometry_file_format: 'glb',
      material: 'PBR',
      quality: 'medium',
      tier: 'Regular',
    },
    supportsPrompt: true,
    supportsSeed: true,
  },
]

export const FAL_MODEL_IDS = new Set(FAL_MODEL_DEFINITIONS.map((model) => model.id))
const FALLBACK_FAL_MODEL = FAL_MODEL_DEFINITIONS[0].id
let falClient = null

export function getFalHealth() {
  return {
    configured: Boolean(FAL_API_KEY),
    defaultModel: normalizeFalModelId(FAL_DEFAULT_MODEL),
    models: FAL_MODEL_DEFINITIONS.map(({ id, label }) => ({ id, label })),
  }
}

export async function createFalTask(payload) {
  const client = getFalClient()
  const modelId = normalizeFalModelId(payload.modelId || payload.falModelId || FAL_DEFAULT_MODEL)
  const image = parseDataUrl(payload.imageDataUrl)
  const blob = new Blob([image.buffer], { type: image.mime })
  const imageUrl = await client.storage.upload(blob, { lifecycle: { expiresIn: '1d' } })
  const input = buildFalInput(modelId, imageUrl, payload)
  const raw = await client.queue.submit(modelId, { input })
  const requestId = raw.request_id || raw.requestId

  if (!requestId) {
    const error = new Error('Fal task response did not include a request id.')
    error.detail = raw
    throw error
  }

  return {
    provider: 'fal',
    taskId: encodeFalTaskId({ modelId, requestId }),
    status: normalizeFalStatus(raw.status),
    raw,
  }
}

export async function getFalTask(taskId) {
  const client = getFalClient()

  if (!taskId) {
    throw Object.assign(new Error('taskId is required.'), { status: 400 })
  }

  const task = decodeFalTaskId(taskId)
  const cacheId = getFalCacheId(task)
  if (await hasLocalModel(cacheId, 'glb')) {
    return {
      provider: 'fal',
      taskId,
      status: 'success',
      progress: 100,
      modelUrl: localModelUrl(cacheId, 'glb'),
      rawModelUrl: '',
      error: '',
      raw: { cached: true },
    }
  }

  const statusRaw = await client.queue.status(task.modelId, { requestId: task.requestId, logs: true })
  const status = normalizeFalStatus(statusRaw.status)
  let modelUrl = ''
  let rawModelUrl = ''
  let cacheError = ''
  let result = null

  if (status === 'success') {
    result = await client.queue.result(task.modelId, { requestId: task.requestId })
    const modelFile = findFalModelFile(result.data ?? result)
    rawModelUrl = modelFile.url

    if (rawModelUrl) {
      try {
        modelUrl = await cacheRemoteModelAs(cacheId, rawModelUrl, modelFile.ext)
      } catch (error) {
        cacheError = error.message || 'Fal model cache failed.'
        modelUrl = `/api/3d/model?url=${encodeURIComponent(rawModelUrl)}`
      }
    } else {
      cacheError = 'Fal response did not include a GLB or GLTF URL.'
    }
  }

  return {
    provider: 'fal',
    taskId,
    status,
    progress: getFalProgress(statusRaw, status),
    modelUrl,
    rawModelUrl,
    error: statusRaw.error || cacheError || '',
    raw: result?.data ?? result ?? statusRaw,
  }
}

export function buildFalInput(modelId, imageUrl, payload = {}) {
  const model = getFalModelDefinition(modelId)
  const input = { ...model.defaults }

  if (model.imageField === 'input_image_urls') {
    input.input_image_urls = [imageUrl]
  } else {
    input[model.imageField] = imageUrl
  }

  if (model.supportsPrompt && payload.prompt) input.prompt = payload.prompt
  if (model.supportsSeed && payload.seed !== undefined && Number.isFinite(Number(payload.seed))) {
    input.seed = Number(payload.seed)
  }

  return input
}

export function encodeFalTaskId(task) {
  return `fal-${Buffer.from(JSON.stringify(task)).toString('base64url')}`
}

export function decodeFalTaskId(taskId) {
  const raw = String(taskId || '')
  if (!raw.startsWith('fal-')) {
    return { modelId: normalizeFalModelId(FAL_DEFAULT_MODEL), requestId: raw }
  }

  try {
    const parsed = JSON.parse(Buffer.from(raw.slice(4), 'base64url').toString('utf8'))
    return {
      modelId: normalizeFalModelId(parsed.modelId || FAL_DEFAULT_MODEL),
      requestId: parsed.requestId || parsed.request_id || raw,
    }
  } catch {
    return { modelId: normalizeFalModelId(FAL_DEFAULT_MODEL), requestId: raw }
  }
}

export function normalizeFalStatus(value) {
  const status = String(value || '').toLowerCase()
  if (!status) return 'queued'
  if (['in_queue', 'queued', 'pending'].includes(status)) return 'queued'
  if (['in_progress', 'running', 'processing'].includes(status)) return 'running'
  if (['failed', 'error', 'cancelled', 'canceled'].includes(status)) return 'failed'
  if (isSuccessStatus(status)) return 'success'
  return status
}

export function normalizeFalModelId(value) {
  const modelId = String(value || '').trim().replace(/^\/+|\/+$/g, '')
  return FAL_MODEL_IDS.has(modelId) ? modelId : FALLBACK_FAL_MODEL
}

export function findFalModelFile(value) {
  const candidates = []
  collectFalModelFiles(value, candidates, '')
  candidates.sort((a, b) => a.score - b.score)
  const candidate = candidates[0]
  return candidate ? { url: candidate.url, ext: candidate.ext } : { url: '', ext: 'glb' }
}

function getFalClient() {
  requireFalKey()
  if (falClient) return falClient

  falClient = createFalClient({
    credentials: FAL_API_KEY,
    fetch: (url, options = {}) => undiciFetch(url, {
      ...options,
      ...(OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}),
    }),
  })
  return falClient
}

function getFalModelDefinition(modelId) {
  const normalized = normalizeFalModelId(modelId)
  return FAL_MODEL_DEFINITIONS.find((model) => model.id === normalized) || FAL_MODEL_DEFINITIONS[0]
}

function getFalCacheId(task) {
  return `fal-${String(task.requestId || '').replace(/^fal-/, '')}`
}

function getFalProgress(raw, status) {
  if (status === 'success') return 100
  if (status === 'failed') return null
  if (status === 'queued') return Number.isFinite(raw.queue_position) ? 0 : 0
  if (typeof raw.progress === 'number') return raw.progress
  if (typeof raw.percent === 'number') return raw.percent
  return null
}

function requireFalKey() {
  if (!FAL_API_KEY) {
    const error = new Error('FAL_API_KEY is not configured on the backend.')
    error.status = 500
    throw error
  }
}

function collectFalModelFiles(value, candidates, key) {
  if (!value) return

  if (typeof value === 'string') {
    const ext = inferModelExtension({ url: value })
    if (ext) candidates.push({ url: value, ext, score: scoreFalModelCandidate(key, ext) })
    return
  }

  if (Array.isArray(value)) {
    value.forEach((item) => collectFalModelFiles(item, candidates, key))
    return
  }

  if (typeof value !== 'object') return

  const url = value.url || value.file_url || value.download_url || value.uri || value.href
  if (typeof url === 'string' && /^https?:\/\//i.test(url)) {
    const ext = inferModelExtension({ url, fileName: value.file_name || value.fileName || value.name, contentType: value.content_type || value.contentType || value.mime_type })
    if (ext) candidates.push({ url, ext, score: scoreFalModelCandidate(key, ext) })
  }

  for (const [childKey, child] of Object.entries(value)) {
    collectFalModelFiles(child, candidates, childKey)
  }
}

function inferModelExtension({ url, fileName, contentType }) {
  const source = `${url || ''} ${fileName || ''}`.toLowerCase()
  if (/\.glb(?:[?#\s]|$)/i.test(source)) return 'glb'
  if (/\.gltf(?:[?#\s]|$)/i.test(source)) return 'gltf'

  const type = String(contentType || '').toLowerCase()
  if (type.includes('model/gltf-binary') || type.includes('application/octet-stream')) return 'glb'
  if (type.includes('model/gltf+json')) return 'gltf'

  return ''
}

function scoreFalModelCandidate(key, ext) {
  const name = String(key || '').toLowerCase()
  const keyScore = name.includes('pbr') ? 0 : name.includes('glb') ? 1 : name.includes('mesh') ? 2 : name.includes('base') ? 3 : 4
  const extScore = ext === 'glb' ? 0 : 1
  return keyScore * 10 + extScore
}
