import { apiUrl, getProviderLabel } from '../services/modelApi.js'

const MODEL_METRIC_CACHE = new Map()

export async function inspectModelUrl(modelUrl) {
  if (!modelUrl) return null
  if (MODEL_METRIC_CACHE.has(modelUrl)) return MODEL_METRIC_CACHE.get(modelUrl)

  const promise = inspectModelUrlUncached(modelUrl)
  MODEL_METRIC_CACHE.set(modelUrl, promise)
  return promise
}

async function inspectModelUrlUncached(modelUrl) {
  const resolvedUrl = apiUrl(modelUrl)
  if (/\/api\/3d\/local-model\//i.test(modelUrl) || /\/api\/3d\/model\?/i.test(modelUrl)) {
    return {
      fileBytes: 0,
      nodeCount: 0,
      meshCount: 0,
      materialCount: 0,
      textureCount: 0,
      triangleCount: 0,
      skippedParse: true,
      inspectedAt: new Date().toISOString(),
    }
  }

  const response = await fetch(resolvedUrl)
  if (!response.ok) {
    throw new Error(`Model metrics unavailable (${response.status})`)
  }

  const headerSize = Number(response.headers.get('content-length'))
  if (Number.isFinite(headerSize) && headerSize > 8 * 1024 * 1024) {
    return {
      fileBytes: headerSize,
      nodeCount: 0,
      meshCount: 0,
      materialCount: 0,
      textureCount: 0,
      triangleCount: 0,
      skippedParse: true,
      inspectedAt: new Date().toISOString(),
    }
  }

  const buffer = await response.arrayBuffer()
  const { GLTFLoader } = await import('three/examples/jsm/loaders/GLTFLoader.js')
  const loader = new GLTFLoader()
  const gltf = await new Promise((resolve, reject) => {
    loader.parse(buffer, '', resolve, reject)
  })

  return extractSceneMetrics(gltf.scene, Number.isFinite(headerSize) && headerSize > 0 ? headerSize : buffer.byteLength)
}

function extractSceneMetrics(scene, fileBytes = 0) {
  let nodeCount = 0
  let meshCount = 0
  let triangleCount = 0
  const materials = new Set()
  const textures = new Set()
  const textureSlots = ['map', 'normalMap', 'roughnessMap', 'metalnessMap', 'aoMap', 'emissiveMap', 'alphaMap', 'bumpMap', 'displacementMap']

  scene.traverse((node) => {
    nodeCount += 1
    if (!node.isMesh) return

    meshCount += 1
    const geometry = node.geometry
    const positionCount = geometry?.attributes?.position?.count || 0
    triangleCount += geometry?.index?.count ? Math.floor(geometry.index.count / 3) : Math.floor(positionCount / 3)

    const nodeMaterials = Array.isArray(node.material) ? node.material : [node.material].filter(Boolean)
    nodeMaterials.forEach((material) => {
      materials.add(material)
      textureSlots.forEach((slot) => {
        if (material?.[slot]) textures.add(material[slot])
      })
    })
  })

  return {
    fileBytes,
    nodeCount,
    meshCount,
    materialCount: materials.size,
    textureCount: textures.size,
    triangleCount,
    inspectedAt: new Date().toISOString(),
  }
}

export function getModelQuality(cell, metrics, generationHistory = []) {
  const generation = cell.custom ? cell.generation || {} : {}
  const status = String(generation.status || (cell.custom ? 'pending' : 'built-in')).toLowerCase()
  const hasGlb = Boolean(generation.modelUrl)
  const provider = generation.provider || (cell.custom ? 'unknown' : 'built-in')
  const history = generationHistory.find((entry) => entry.cellId === cell.id && ['success', 'failed'].includes(String(entry.status).toLowerCase()))
  const durationMs = history?.durationMs
  const failed = status === 'failed'
  const loadingMetrics = hasGlb && !metrics
  const metricError = metrics?.error || ''
  const score = calculateScore({ cell, generation, metrics, durationMs, failed, hasGlb })

  return {
    score,
    verdict: getVerdict(score, { cell, failed, hasGlb, metricError }),
    providerLabel: provider === 'built-in' ? 'Built-in' : getProviderLabel(provider),
    status,
    hasGlb,
    durationMs,
    loadingMetrics,
    metricError,
    fileBytes: metrics?.fileBytes || 0,
    nodeCount: metrics?.nodeCount || 0,
    meshCount: metrics?.meshCount || 0,
    materialCount: metrics?.materialCount || 0,
    textureCount: metrics?.textureCount || 0,
    triangleCount: metrics?.triangleCount || 0,
  }
}

function calculateScore({ cell, generation, metrics, durationMs, failed, hasGlb }) {
  if (failed) return 12
  if (generation?.status && !['success', 'local'].includes(String(generation.status).toLowerCase()) && !hasGlb) return 38

  let score = cell.custom ? 28 : 68
  if (hasGlb) score += 28
  else if (generation?.provider === 'cinematic') score += 12

  if (metrics?.triangleCount >= 50000) score += 16
  else if (metrics?.triangleCount >= 10000) score += 13
  else if (metrics?.triangleCount >= 2000) score += 9
  else if (metrics?.triangleCount > 0) score += 5

  if (metrics?.textureCount >= 4) score += 12
  else if (metrics?.textureCount > 0) score += 8
  else if (hasGlb) score += 2

  if (metrics?.meshCount >= 8) score += 7
  else if (metrics?.meshCount >= 2) score += 4

  if (metrics?.fileBytes >= 2_000_000) score += 5
  if (Number.isFinite(durationMs) && durationMs > 0 && durationMs < 180_000) score += 4

  return Math.max(0, Math.min(98, Math.round(score)))
}

function getVerdict(score, { cell, failed, hasGlb, metricError }) {
  if (failed) return 'Failed'
  if (metricError) return 'GLB loaded, metrics limited'
  if (cell.custom && !hasGlb && cell.generation?.provider === 'cinematic') return 'Preview only'
  if (cell.custom && !hasGlb) return 'Waiting for GLB'
  if (score >= 86) return 'Demo-ready'
  if (score >= 72) return 'Solid'
  if (score >= 55) return 'Usable'
  return 'Needs better source'
}

export function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return 'n/a'
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toFixed(bytes >= 10_000_000 ? 0 : 1)} MB`
  if (bytes >= 1000) return `${Math.round(bytes / 1000)} KB`
  return `${Math.round(bytes)} B`
}

export function formatDuration(ms) {
  if (!Number.isFinite(ms) || ms <= 0) return 'n/a'
  if (ms >= 60_000) return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
  return `${Math.max(1, Math.round(ms / 1000))}s`
}

export function formatNumber(value) {
  if (!Number.isFinite(value) || value <= 0) return '0'
  return new Intl.NumberFormat(undefined, { notation: value >= 100000 ? 'compact' : 'standard' }).format(value)
}
