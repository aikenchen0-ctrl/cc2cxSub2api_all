import { createWriteStream } from 'node:fs'
import { access, mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { Readable } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { fetch as undiciFetch } from 'undici'
import { LOCAL_MODEL_DIR, MODEL_UPLOAD_LIMIT, OUTBOUND_PROXY_AGENT, TRIPO_API_BASE, TRIPO_API_KEY } from './config.mjs'
import { readRawBody, sanitizeFileName } from './http-utils.mjs'

export async function saveLocalModel(taskId, modelData, ext = 'glb') {
  const buffer = Buffer.isBuffer(modelData) ? modelData : parseModelBase64(modelData)
  validateModelBuffer(buffer, ext)

  await mkdir(LOCAL_MODEL_DIR, { recursive: true })
  await writeFile(localModelPath(taskId, ext), buffer)
}

export async function hasLocalModel(taskId, ext = 'glb') {
  try {
    await access(localModelPath(taskId, ext))
    return true
  } catch {
    return false
  }
}

export function localModelPath(taskId, ext = 'glb') {
  return path.join(LOCAL_MODEL_DIR, `${sanitizeModelId(taskId)}.${ext}`)
}

export function localModelUrl(taskId, ext = 'glb') {
  return `/api/3d/local-model/${encodeURIComponent(sanitizeModelId(taskId))}.${ext}`
}

export async function serveLocalModel(url, response) {
  const rawFileName = decodeURIComponent(url.pathname.replace('/api/3d/local-model/', ''))
  const ext = getModelExtension(rawFileName)
  const modelId = rawFileName.replace(/\.(?:glb|gltf)$/i, '')
  const buffer = await readFile(localModelPath(modelId, ext))
  response.writeHead(200, {
    'Content-Type': ext === 'gltf' ? 'model/gltf+json' : 'model/gltf-binary',
    'Cache-Control': 'private, max-age=3600',
  })
  response.end(buffer)
}

export async function importLocalModel(request, url) {
  const fileName = sanitizeFileName(url.searchParams.get('fileName') || 'local-model.glb')
  const ext = getModelExtension(fileName)
  const buffer = await readRawBody(request, MODEL_UPLOAD_LIMIT)
  validateModelBuffer(buffer, ext)

  const baseName = fileName.replace(/\.(?:glb|gltf)$/i, '') || 'local-model'
  const modelId = `local-${Date.now()}-${baseName}`
  await saveLocalModel(modelId, buffer, ext)

  return {
    provider: 'local',
    taskId: sanitizeModelId(modelId),
    status: 'success',
    progress: 100,
    modelUrl: localModelUrl(modelId, ext),
    rawModelUrl: '',
    fileName,
  }
}

export async function cacheRemoteModel(taskId, rawModelUrl) {
  return cacheRemoteModelAs(taskId, rawModelUrl, getModelExtension(rawModelUrl))
}

export async function cacheRemoteModelAs(taskId, rawModelUrl, ext = 'glb') {
  if (await hasLocalModel(taskId, ext)) return localModelUrl(taskId, ext)

  await mkdir(LOCAL_MODEL_DIR, { recursive: true })
  const targetPath = localModelPath(taskId, ext)
  const tempPath = `${targetPath}.${Date.now()}.tmp`

  try {
    const remote = await fetchRemoteModel(rawModelUrl)
    await pipeline(Readable.fromWeb(remote.body), createWriteStream(tempPath))
    const buffer = await readFile(tempPath)
    validateModelBuffer(buffer, ext)
    await rename(tempPath, targetPath)
  } catch (error) {
    await rm(tempPath, { force: true }).catch(() => {})
    throw error
  }

  return localModelUrl(taskId, ext)
}

export async function proxyModel(url, response) {
  const rawUrl = url.searchParams.get('url')
  if (!rawUrl || !isAllowedProxyModelUrl(rawUrl)) {
    throw Object.assign(new Error('A valid HTTPS or localhost model URL is required.'), { status: 400 })
  }

  const fetchOptions = shouldUseProxy(rawUrl) && OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}
  const remote = await undiciFetch(rawUrl, fetchOptions)
  if (!remote.ok || !remote.body) {
    const retry = await fetchWithTripoAuth(rawUrl, fetchOptions)
    if (!retry.ok || !retry.body) {
      throw Object.assign(new Error(`Model download failed with ${retry.status || remote.status}.`), { status: 502 })
    }
    await streamRemoteModel(retry, response)
    return
  }

  await streamRemoteModel(remote, response)
}

export function getModelExtension(value) {
  const pathname = /^https?:\/\//i.test(String(value)) ? new URL(value).pathname : String(value)
  const ext = path.extname(pathname).replace('.', '').toLowerCase()
  if (ext === 'gltf') return 'gltf'
  if (ext === 'glb') return 'glb'
  throw Object.assign(new Error('Only GLB or self-contained GLTF models are supported.'), { status: 400 })
}

export function validateModelBuffer(buffer, ext = 'glb') {
  if (!Buffer.isBuffer(buffer) || buffer.length < 32) {
    throw Object.assign(new Error('Model file is too small or invalid.'), { status: 400 })
  }

  if (ext === 'glb') {
    if (buffer.subarray(0, 4).toString('ascii') !== 'glTF') {
      throw Object.assign(new Error('GLB files must start with a glTF binary header.'), { status: 400 })
    }
    return
  }

  try {
    JSON.parse(buffer.toString('utf8'))
  } catch {
    throw Object.assign(new Error('GLTF files must be valid JSON.'), { status: 400 })
  }
}

export function sanitizeModelId(value) {
  return sanitizeFileName(String(value)).replace(/\.(?:glb|gltf)$/i, '').replace(/\s+/g, '-').slice(0, 96) || `model-${Date.now()}`
}

export function shouldUseProxy(rawUrl) {
  try {
    const parsed = new URL(rawUrl)
    return !['127.0.0.1', 'localhost', '::1'].includes(parsed.hostname)
  } catch {
    return true
  }
}

export function shouldAttachTripoAuth(rawUrl) {
  if (!TRIPO_API_KEY) return false

  try {
    const host = new URL(rawUrl).hostname
    const tripoHost = new URL(TRIPO_API_BASE).hostname
    return host === tripoHost || host.endsWith('.tripo3d.ai')
  } catch {
    return false
  }
}

async function fetchRemoteModel(rawUrl) {
  const fetchOptions = shouldUseProxy(rawUrl) && OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}
  const remote = await undiciFetch(rawUrl, fetchOptions)
  if (remote.ok && remote.body) return remote

  const retry = await fetchWithTripoAuth(rawUrl, fetchOptions)
  if (retry.ok && retry.body) return retry

  throw Object.assign(new Error(`Model download failed with ${retry.status || remote.status}.`), { status: 502 })
}

async function fetchWithTripoAuth(rawUrl, fetchOptions) {
  if (!shouldAttachTripoAuth(rawUrl)) {
    return { ok: false, status: 401, body: null }
  }

  return undiciFetch(rawUrl, {
    headers: { Authorization: `Bearer ${TRIPO_API_KEY}` },
    ...fetchOptions,
  })
}

function parseModelBase64(modelBase64) {
  const raw = String(modelBase64 || '').replace(/^data:.*?;base64,/, '')
  return Buffer.from(raw, 'base64')
}

function isAllowedProxyModelUrl(rawUrl) {
  try {
    const parsed = new URL(rawUrl)
    if (parsed.protocol === 'https:') return true
    if (parsed.protocol !== 'http:') return false
    return ['127.0.0.1', 'localhost', '::1'].includes(parsed.hostname)
  } catch {
    return false
  }
}

async function streamRemoteModel(remote, response) {
  response.writeHead(200, {
    'Content-Type': remote.headers.get('content-type') || 'model/gltf-binary',
    'Cache-Control': 'private, max-age=3600',
  })

  await pipeline(Readable.fromWeb(remote.body), response)
}
