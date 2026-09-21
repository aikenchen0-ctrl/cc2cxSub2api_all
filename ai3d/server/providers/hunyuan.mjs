import { fetch as undiciFetch } from 'undici'
import { HUNYUAN_API_BASE, HUNYUAN_CREATE_PATH, HUNYUAN_STATUS_PATH } from '../config.mjs'
import { parseDataUrl } from '../http-utils.mjs'
import { hasLocalModel, localModelUrl, saveLocalModel } from '../model-store.mjs'
import { findFirstValue, findModelUrl } from '../object-utils.mjs'
import {
  createHunyuanCloudTask,
  getHunyuanCloudHealth,
  getHunyuanCloudTask,
  isHunyuanCloudConfigured,
} from './hunyuan-cloud.mjs'

export function getHunyuanHealth() {
  const cloud = getHunyuanCloudHealth()
  if (cloud.configured) {
    return {
      configured: true,
      mode: 'cloud',
      ...cloud,
    }
  }

  return {
    configured: Boolean(HUNYUAN_API_BASE),
    mode: 'local',
    baseUrl: HUNYUAN_API_BASE,
    createPath: HUNYUAN_CREATE_PATH,
    statusPath: HUNYUAN_STATUS_PATH,
  }
}

export async function createHunyuanTask(payload) {
  if (isHunyuanCloudConfigured()) return createHunyuanCloudTask(payload)

  const image = parseDataUrl(payload.imageDataUrl)
  const imageBase64 = image.buffer.toString('base64')
  const requestBody = {
    image: `data:${image.mime};base64,${imageBase64}`,
    image_base64: imageBase64,
    prompt: payload.prompt || '',
    seed: payload.seed ?? 1234,
    remove_background: payload.removeBackground ?? true,
    texture: payload.texture ?? true,
    pbr: payload.pbr ?? true,
    octree_resolution: payload.octreeResolution ?? 256,
    num_inference_steps: payload.numInferenceSteps ?? 50,
    guidance_scale: payload.guidanceScale ?? 5.5,
    face_count: payload.faceCount ?? 60000,
  }

  const raw = await hunyuanRequest(HUNYUAN_CREATE_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(requestBody),
  })
  const data = raw.data || raw
  const taskId = findFirstValue(data, ['uid', 'task_id', 'taskId', 'id']) || `hunyuan-${Date.now()}`
  const rawModelUrl = findModelUrl(data)
  const modelBase64 = findFirstValue(data, ['model_base64', 'modelBase64', 'glb_base64', 'glbBase64'])
  let modelUrl = rawModelUrl ? `/api/3d/model?url=${encodeURIComponent(rawModelUrl)}` : ''

  if (modelBase64) {
    await saveLocalModel(taskId, modelBase64, 'glb')
    modelUrl = localModelUrl(taskId, 'glb')
  }

  return {
    provider: 'hunyuan',
    taskId,
    status: modelUrl ? 'success' : 'queued',
    modelUrl,
    raw: sanitizeHunyuanRaw(raw),
  }
}

export async function getHunyuanTask(taskId) {
  if (isHunyuanCloudConfigured()) return getHunyuanCloudTask(taskId)

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
      raw: {},
    }
  }

  const raw = await hunyuanRequest(`${HUNYUAN_STATUS_PATH}/${encodeURIComponent(taskId)}`, { method: 'GET' })
  const data = raw.data || raw
  const status = normalizeHunyuanStatus(data.status || data.task_status || data.state || data.message || 'running')
  const progress = data.progress ?? data.percent ?? null
  const rawModelUrl = findModelUrl(data)
  const modelBase64 = findFirstValue(data, ['model_base64', 'modelBase64', 'glb_base64', 'glbBase64'])
  let modelUrl = rawModelUrl ? `/api/3d/model?url=${encodeURIComponent(rawModelUrl)}` : ''

  if (modelBase64) {
    await saveLocalModel(taskId, modelBase64, 'glb')
    modelUrl = localModelUrl(taskId, 'glb')
  }

  return {
    provider: 'hunyuan',
    taskId,
    status,
    progress,
    modelUrl,
    rawModelUrl,
    error: data.error || data.message || '',
    raw: sanitizeHunyuanRaw(raw),
  }
}

async function hunyuanRequest(requestPath, options = {}) {
  let response
  try {
    response = await undiciFetch(`${HUNYUAN_API_BASE.replace(/\/$/, '')}${requestPath.startsWith('/') ? requestPath : `/${requestPath}`}`, options)
  } catch (error) {
    const wrapped = new Error(`Hunyuan3D local server unavailable at ${HUNYUAN_API_BASE}. Start the local Hunyuan3D API server or switch provider.`)
    wrapped.detail = {
      path: requestPath,
      cause: error.cause?.message || error.cause?.code || error.message,
    }
    throw wrapped
  }

  const text = await response.text()
  let data
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    data = { message: text || 'Non-JSON response from Hunyuan3D.' }
  }

  if (!response.ok || (typeof data.code === 'number' && data.code !== 0)) {
    const error = new Error(data.message || data.error || `Hunyuan3D request failed with ${response.status}.`)
    error.status = response.status || 502
    error.detail = sanitizeHunyuanRaw(data)
    throw error
  }

  return data
}

function normalizeHunyuanStatus(status) {
  const value = String(status || '').toLowerCase()
  if (['success', 'succeeded', 'completed', 'complete', 'done', 'finish', 'finished'].includes(value)) return 'success'
  if (['failed', 'error', 'cancelled', 'canceled'].includes(value)) return 'failed'
  if (['queued', 'pending', 'waiting'].includes(value)) return 'queued'
  return 'running'
}

function sanitizeHunyuanRaw(raw) {
  if (!raw || typeof raw !== 'object') return raw
  return JSON.parse(JSON.stringify(raw, (key, value) => {
    if (['model_base64', 'modelBase64', 'glb_base64', 'glbBase64'].includes(key)) return '[base64 omitted]'
    return value
  }))
}
