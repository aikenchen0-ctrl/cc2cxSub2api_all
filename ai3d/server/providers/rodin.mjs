import { Blob } from 'node:buffer'
import { fetch as undiciFetch, FormData } from 'undici'

import {
  OUTBOUND_PROXY_AGENT,
  RODIN_API_BASE,
  RODIN_API_KEY,
  RODIN_MATERIAL,
  RODIN_MESH_MODE,
  RODIN_QUALITY,
  RODIN_TIER,
  hasOutboundProxy,
} from '../config.mjs'
import { parseDataUrl, sanitizeFileName } from '../http-utils.mjs'
import { cacheRemoteModelAs, hasLocalModel, localModelUrl } from '../model-store.mjs'
import { findFirstValue } from '../object-utils.mjs'

export function getRodinHealth() {
  return {
    configured: Boolean(RODIN_API_KEY),
    baseUrl: RODIN_API_BASE,
    tier: RODIN_TIER,
    quality: RODIN_QUALITY,
    meshMode: RODIN_MESH_MODE,
    material: RODIN_MATERIAL,
  }
}

export async function createRodinTask(payload) {
  requireRodinKey()

  const image = parseDataUrl(payload.imageDataUrl)
  const fileName = sanitizeFileName(payload.fileName || `cell-reference.${image.ext}`)
  const form = new FormData()
  form.append('images', new Blob([image.buffer], { type: image.mime }), fileName)
  form.append('geometry_file_format', 'glb')
  form.append('material', payload.material || RODIN_MATERIAL)
  form.append('quality', payload.quality || RODIN_QUALITY)
  form.append('tier', payload.tier || RODIN_TIER)
  form.append('mesh_mode', payload.meshMode || RODIN_MESH_MODE)

  if (payload.prompt) form.append('prompt', payload.prompt)
  if (payload.seed !== undefined) form.append('seed', String(payload.seed))

  const raw = await rodinRequest('/rodin', {
    method: 'POST',
    body: form,
  })
  const taskUuid = findFirstValue(raw, ['uuid', 'task_uuid', 'taskUuid', 'taskId', 'id'])
  const subscriptionKey = findFirstValue(raw.jobs || raw, ['subscription_key', 'subscriptionKey'])

  if (!taskUuid) {
    const error = new Error('Rodin task response did not include a task uuid.')
    error.detail = sanitizeRodinRaw(raw)
    throw error
  }

  if (!subscriptionKey) {
    const error = new Error('Rodin task response did not include a subscription key.')
    error.detail = sanitizeRodinRaw(raw)
    throw error
  }

  return {
    provider: 'rodin',
    taskId: encodeRodinTaskId({ taskUuid, subscriptionKey }),
    status: 'queued',
    raw: sanitizeRodinRaw(raw),
  }
}

export async function getRodinTask(taskId) {
  requireRodinKey()

  if (!taskId) {
    throw Object.assign(new Error('taskId is required.'), { status: 400 })
  }

  const rodinTask = decodeRodinTaskId(taskId)
  if (await hasLocalModel(rodinTask.taskUuid, 'glb')) {
    return {
      provider: 'rodin',
      taskId,
      status: 'success',
      progress: 100,
      modelUrl: localModelUrl(rodinTask.taskUuid, 'glb'),
      rawModelUrl: '',
      error: '',
      raw: { cached: true },
    }
  }

  const raw = await rodinRequest('/status', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ subscription_key: rodinTask.subscriptionKey }),
  })
  const jobs = Array.isArray(raw.jobs) ? raw.jobs : []
  const status = normalizeRodinStatus(jobs.map((job) => job.status).filter(Boolean))
  let modelUrl = ''
  let rawModelUrl = ''
  let cacheError = ''

  if (status === 'success') {
    try {
      const download = await getRodinDownload(rodinTask.taskUuid)
      rawModelUrl = download.url
      modelUrl = await cacheRemoteModelAs(rodinTask.taskUuid, rawModelUrl, download.ext)
    } catch (error) {
      cacheError = error.message || 'Rodin model download failed.'
    }
  }

  return {
    provider: 'rodin',
    taskId,
    status,
    progress: getRodinProgress(status, jobs),
    modelUrl,
    rawModelUrl,
    error: raw.error || cacheError || '',
    raw: sanitizeRodinRaw(raw),
  }
}

export function encodeRodinTaskId(task) {
  return `rodin-${Buffer.from(JSON.stringify(task)).toString('base64url')}`
}

export function decodeRodinTaskId(taskId) {
  const raw = String(taskId || '')
  if (!raw.startsWith('rodin-')) {
    return { taskUuid: raw, subscriptionKey: raw }
  }

  try {
    const parsed = JSON.parse(Buffer.from(raw.slice(6), 'base64url').toString('utf8'))
    return {
      taskUuid: parsed.taskUuid || parsed.uuid || raw,
      subscriptionKey: parsed.subscriptionKey || parsed.subscription_key || parsed.taskUuid || raw,
    }
  } catch {
    return { taskUuid: raw, subscriptionKey: raw }
  }
}

export function normalizeRodinStatus(statuses) {
  const values = (Array.isArray(statuses) ? statuses : [statuses]).map((status) => String(status || '').trim().toLowerCase())
  if (!values.length) return 'running'
  if (values.some((status) => ['failed', 'failure', 'error', 'cancelled', 'canceled'].includes(status))) return 'failed'
  if (values.every((status) => ['done', 'success', 'succeeded', 'completed', 'complete', 'finish', 'finished'].includes(status))) return 'success'
  if (values.some((status) => ['waiting', 'queued', 'pending'].includes(status))) return 'queued'
  return 'running'
}

export function findRodinDownloadItem(raw) {
  const items = Array.isArray(raw?.list) ? raw.list : []
  return items.find((entry) => /\.glb(?:[?#]|$)/i.test(entry.name || entry.url || ''))
    || items.find((entry) => /\.gltf(?:[?#]|$)/i.test(entry.name || entry.url || ''))
    || items.find((entry) => /^https?:\/\//i.test(entry.url || ''))
    || null
}

function requireRodinKey() {
  if (!RODIN_API_KEY) {
    const error = new Error('RODIN_API_KEY is not configured on the backend.')
    error.status = 500
    throw error
  }
}

async function getRodinDownload(taskUuid) {
  const raw = await rodinRequest('/download', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ task_uuid: taskUuid }),
  })
  const item = findRodinDownloadItem(raw)

  if (!item?.url) {
    const error = new Error('Rodin download response did not include a model URL.')
    error.detail = sanitizeRodinRaw(raw)
    throw error
  }

  const ext = /\.gltf(?:[?#]|$)/i.test(item.name || item.url) ? 'gltf' : 'glb'
  return { url: item.url, ext, raw }
}

function getRodinProgress(status, jobs) {
  if (status === 'success') return 100
  if (status === 'queued') return 0
  if (!Array.isArray(jobs) || !jobs.length) return null

  const done = jobs.filter((job) => normalizeRodinStatus(job.status) === 'success').length
  if (!done) return null
  return Math.round((done / jobs.length) * 100)
}

async function rodinRequest(requestPath, options = {}) {
  let response
  try {
    response = await undiciFetch(`${RODIN_API_BASE.replace(/\/$/, '')}${requestPath.startsWith('/') ? requestPath : `/${requestPath}`}`, {
      ...options,
      ...(OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}),
      headers: {
        Authorization: `Bearer ${RODIN_API_KEY}`,
        Accept: 'application/json',
        ...(options.headers || {}),
      },
    })
  } catch (error) {
    const wrapped = new Error(`Rodin network request failed: ${error.message}`)
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
    data = { message: text || 'Non-JSON response from Rodin.' }
  }

  if (!response.ok || data.error) {
    const error = new Error(data.message || data.error || `Rodin request failed with ${response.status}.`)
    error.status = response.status || 502
    error.detail = sanitizeRodinRaw(data)
    throw error
  }

  return data
}

function sanitizeRodinRaw(raw) {
  if (!raw || typeof raw !== 'object') return raw
  return JSON.parse(JSON.stringify(raw, (key, value) => {
    if (['subscription_key', 'subscriptionKey'].includes(key)) return '[secret omitted]'
    return value
  }))
}
