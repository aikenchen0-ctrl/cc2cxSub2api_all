import { fetch as undiciFetch } from 'undici'
import {
  HUNYUAN_CLOUD_API_BASE,
  HUNYUAN_CLOUD_API_KEY,
  HUNYUAN_CLOUD_ENABLE_PBR,
  HUNYUAN_CLOUD_FACE_COUNT,
  HUNYUAN_CLOUD_GENERATE_TYPE,
  HUNYUAN_CLOUD_MODEL,
  HUNYUAN_CLOUD_QUERY_PATH,
  HUNYUAN_CLOUD_SUBMIT_PATH,
  OUTBOUND_PROXY_AGENT,
  hasConfiguredSecret,
  hasOutboundProxy,
} from '../config.mjs'
import { parseDataUrl } from '../http-utils.mjs'
import { cacheRemoteModel, hasLocalModel, localModelUrl } from '../model-store.mjs'
import { findFirstValue, findModelUrl, isSuccessStatus } from '../object-utils.mjs'

const MAX_IMAGE_BYTES = 5 * 1024 * 1024
export const DEFAULT_HUNYUAN_SKETCH_PROMPT = '一只上皮细胞，光滑表面，细胞核清晰'

export function isHunyuanCloudConfigured() {
  return hasConfiguredSecret(HUNYUAN_CLOUD_API_KEY)
}

export function getHunyuanCloudHealth() {
  return {
    configured: isHunyuanCloudConfigured(),
    model: HUNYUAN_CLOUD_MODEL,
    baseUrl: HUNYUAN_CLOUD_API_BASE,
    generateType: HUNYUAN_CLOUD_GENERATE_TYPE,
    enablePbr: HUNYUAN_CLOUD_ENABLE_PBR,
    faceCount: HUNYUAN_CLOUD_FACE_COUNT,
  }
}

/**
 * 构造混元生 3D 提交请求体；该函数不执行网络请求，便于前后端复用和测试。
 * 草图模式是唯一允许图片与提示词同传的模式，并且始终锁定 3.0。
 */
export function buildHunyuanCloudSubmitBody({
  imageDataUrl = '',
  imageBase64 = '',
  imageField = 'ImageBase64',
  generateType = HUNYUAN_CLOUD_GENERATE_TYPE,
  model = HUNYUAN_CLOUD_MODEL,
  enablePbr = HUNYUAN_CLOUD_ENABLE_PBR,
  faceCount = HUNYUAN_CLOUD_FACE_COUNT,
  prompt = '',
  legacy = true,
} = {}) {
  const normalizedType = normalizeCloudGenerateType(generateType)
  const resolvedModel = resolveCloudModel(normalizedType, model)
  const resolvedPrompt = resolveCloudPrompt(normalizedType, prompt)

  if (legacy) {
    return {
      Model: resolvedModel,
      [imageField]: imageDataUrl,
      EnablePBR: enablePbr,
      FaceCount: faceCount,
      GenerateType: normalizedType,
      ...(isSketchType(normalizedType) ? { Prompt: resolvedPrompt } : {}),
    }
  }

  return {
    model: resolvedModel,
    image_base64: imageBase64 || extractImageBase64(imageDataUrl),
    generate_type: String(normalizedType).toLowerCase(),
    enable_pbr: enablePbr,
    face_count: faceCount,
    ...(isSketchType(normalizedType) ? { prompt: resolvedPrompt } : {}),
  }
}

export async function createHunyuanCloudTask(payload) {
  requireCloudKey()
  const image = parseDataUrl(payload.imageDataUrl)
  if (image.buffer.length > MAX_IMAGE_BYTES) {
    throw Object.assign(new Error('Hunyuan cloud image must be 5MB or smaller after encoding.'), { status: 413 })
  }

  const generateType = payload.generateType || HUNYUAN_CLOUD_GENERATE_TYPE
  const model = resolveCloudModel(generateType, payload.model || payload.modelId || HUNYUAN_CLOUD_MODEL)
  const enablePbr = payload.pbr ?? payload.enablePbr ?? HUNYUAN_CLOUD_ENABLE_PBR
  const faceCount = payload.faceCount ?? HUNYUAN_CLOUD_FACE_COUNT
  const prompt = clipPrompt(payload.prompt)
  const imageDataUrl = `data:${image.mime};base64,${image.buffer.toString('base64')}`
  const bodies = usesLegacyAi3dApi()
    ? [
        buildHunyuanCloudSubmitBody({
          imageDataUrl,
          generateType,
          model,
          enablePbr,
          faceCount,
          prompt,
          imageField: 'ImageBase64',
        }),
        buildHunyuanCloudSubmitBody({
          imageDataUrl,
          generateType,
          model,
          enablePbr,
          faceCount,
          prompt,
          imageField: 'ImageUrl',
        }),
      ]
    : [
        buildHunyuanCloudSubmitBody({
          imageDataUrl,
          imageBase64: image.buffer.toString('base64'),
          generateType,
          model,
          enablePbr,
          faceCount,
          prompt,
          legacy: false,
        }),
      ]

  let raw
  let lastError
  for (const body of bodies) {
    try {
      raw = await hunyuanCloudRequest(HUNYUAN_CLOUD_SUBMIT_PATH, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      lastError = null
      break
    } catch (error) {
      lastError = error
    }
  }
  if (!raw) throw lastError

  const data = unwrapCloudPayload(raw)
  const taskId = findFirstValue(data, ['JobId', 'id', 'job_id', 'task_id', 'taskId'])
  if (!taskId) {
    const error = new Error(extractCloudError(raw) || 'Hunyuan cloud task response did not include a job id.')
    error.detail = sanitizeCloudRaw(raw)
    throw error
  }

  return {
    provider: 'hunyuan',
    taskId,
    status: normalizeCloudStatus(data.Status || data.status || 'queued'),
    raw: sanitizeCloudRaw(raw),
  }
}

export async function getHunyuanCloudTask(taskId) {
  if (!taskId) {
    throw Object.assign(new Error('taskId is required.'), { status: 400 })
  }

  if (await hasLocalModel(taskId, 'glb')) {
    return {
      provider: 'hunyuan',
      taskId,
      status: 'success',
      progress: 100,
      modelUrl: localModelUrl(taskId, 'glb'),
      rawModelUrl: '',
      error: '',
      raw: { cached: true },
    }
  }

  requireCloudKey()
  const queryBody = usesLegacyAi3dApi()
    ? { JobId: taskId }
    : { model: HUNYUAN_CLOUD_MODEL, id: taskId }
  const raw = await hunyuanCloudRequest(HUNYUAN_CLOUD_QUERY_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(queryBody),
  })
  const data = unwrapCloudPayload(raw)
  const status = normalizeCloudStatus(data.Status || data.status || data.task_status || data.state || 'running')
  const rawModelUrl = findGlbUrl(data) || findModelUrl(data)
  let modelUrl = ''
  let cacheError = ''
  let nextStatus = status

  if (rawModelUrl && isSuccessStatus(status)) {
    try {
      modelUrl = await cacheRemoteModel(taskId, rawModelUrl)
    } catch (error) {
      cacheError = error.message || 'Model cache failed.'
      nextStatus = 'running'
    }
  }

  return {
    provider: 'hunyuan',
    taskId,
    status: nextStatus,
    progress: data.progress ?? data.percent ?? (isSuccessStatus(nextStatus) ? 100 : null),
    modelUrl,
    rawModelUrl: '',
    error: extractCloudError(data) || cacheError,
    raw: sanitizeCloudRaw(raw),
  }
}

function usesLegacyAi3dApi() {
  return /ai3d\.cloud\.tencent\.com/i.test(HUNYUAN_CLOUD_API_BASE)
}

function isSketchType(generateType) {
  return String(generateType || '').toLowerCase() === 'sketch'
}

function normalizeCloudGenerateType(generateType) {
  const value = String(generateType || '').trim()
  if (value.toLowerCase() === 'sketch') return 'Sketch'
  if (value.toLowerCase() === 'normal') return 'Normal'
  if (value.toLowerCase() === 'lowpoly') return 'LowPoly'
  return value || HUNYUAN_CLOUD_GENERATE_TYPE
}

function resolveCloudModel(generateType, requestedModel) {
  if (isSketchType(generateType) || String(generateType || '').toLowerCase() === 'lowpoly') {
    return '3.0'
  }
  return requestedModel || HUNYUAN_CLOUD_MODEL
}

function clipPrompt(value) {
  const text = String(value || '').trim()
  if (!text) return ''
  return [...text].slice(0, 1024).join('')
}

function resolveCloudPrompt(generateType, value) {
  if (!isSketchType(generateType)) return ''
  return clipPrompt(value) || DEFAULT_HUNYUAN_SKETCH_PROMPT
}

function extractImageBase64(value) {
  const text = String(value || '')
  const comma = text.indexOf(',')
  return text.startsWith('data:') && comma >= 0 ? text.slice(comma + 1) : text
}

function requireCloudKey() {
  if (!hasConfiguredSecret(HUNYUAN_CLOUD_API_KEY)) {
    const error = new Error('HUNYUAN_CLOUD_API_KEY is not configured on the backend.')
    error.status = 503
    throw error
  }
}

function unwrapCloudPayload(raw) {
  if (!raw || typeof raw !== 'object') return {}
  if (raw.Response && typeof raw.Response === 'object') return raw.Response
  if (raw.data && typeof raw.data === 'object' && !Array.isArray(raw.data) && (raw.data.id || raw.data.status || raw.data.JobId)) {
    return raw.data
  }
  return raw
}

function findGlbUrl(value) {
  const files = []
  collectFiles(value, files)
  const glb = files.find((item) => ['glb', 'GLB'].includes(String(item.Type || item.type || '')) && (item.Url || item.url))
  if (glb) return glb.Url || glb.url
  const byExt = files.find((item) => /\.glb(?:[?#]|$)/i.test(item.Url || item.url || ''))
  return byExt?.Url || byExt?.url || ''
}

function collectFiles(value, files) {
  if (!value) return
  if (Array.isArray(value)) {
    value.forEach((item) => collectFiles(item, files))
    return
  }
  if (typeof value !== 'object') return
  if (typeof value.url === 'string' || typeof value.Url === 'string') files.push(value)
  Object.values(value).forEach((item) => {
    if (item && typeof item === 'object') collectFiles(item, files)
  })
}

function normalizeCloudStatus(status) {
  const value = String(status || '').toLowerCase()
  if (['success', 'succeeded', 'completed', 'complete', 'done', 'finish', 'finished'].includes(value)) return 'success'
  if (['failed', 'fail', 'error', 'cancelled', 'canceled'].includes(value)) return 'failed'
  if (['queued', 'pending', 'waiting', 'wait'].includes(value)) return 'queued'
  if (['running', 'in_progress', 'processing', 'run'].includes(value)) return 'running'
  return 'running'
}

async function hunyuanCloudRequest(requestPath, options = {}) {
  const url = `${HUNYUAN_CLOUD_API_BASE.replace(/\/$/, '')}${requestPath.startsWith('/') ? requestPath : `/${requestPath}`}`
  let response
  try {
    response = await undiciFetch(url, {
      ...options,
      ...(OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}),
      headers: {
        Authorization: `Bearer ${HUNYUAN_CLOUD_API_KEY}`,
        ...(options.headers || {}),
      },
    })
  } catch (error) {
    const wrapped = new Error(`Hunyuan cloud network request failed: ${error.message}`)
    wrapped.detail = {
      path: requestPath,
      cause: error.cause?.message || error.cause?.code || '',
      proxy: hasOutboundProxy(),
    }
    throw wrapped
  }

  const text = await response.text()
  let data
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    data = { message: text || 'Non-JSON response from Hunyuan cloud.' }
  }

  const errorMessage = extractCloudError(data)
  const failed = !response.ok
    || (typeof data.code === 'number' && data.code !== 0)
    || Boolean(data.error)
    || Boolean(data.Error)
    || Boolean(data.Response?.Error)
  if (failed) {
    const error = new Error(errorMessage || `Hunyuan cloud request failed with ${response.status}.`)
    error.status = response.status || 502
    error.detail = sanitizeCloudRaw(data)
    throw error
  }

  return data
}

function extractCloudError(data) {
  if (!data || typeof data !== 'object') return ''
  const nested = data.error && typeof data.error === 'object'
    ? data.error
    : data.Error || data.Response?.Error
  const candidates = [
    data.ErrorMessage,
    nested?.Message,
    nested?.message_zh,
    nested?.message,
    data.error_message,
    data.message_zh,
    data.message,
    typeof data.error === 'string' ? data.error : '',
  ]
  return candidates.find((item) => typeof item === 'string' && item.trim()) || ''
}

function sanitizeCloudRaw(raw) {
  if (!raw || typeof raw !== 'object') return raw
  return JSON.parse(JSON.stringify(raw, (key, value) => {
    if (['image_base64', 'ImageBase64'].includes(key)) return '[base64 omitted]'
    if (['Url', 'ImageUrl'].includes(key) && typeof value === 'string' && value.startsWith('data:')) return '[base64 omitted]'
    return value
  }))
}
