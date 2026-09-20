import { getProviderLabel } from '../services/modelApi.js'
import { getAssetIntelligence } from './assetIntelligence.js'
import { inferMotionProfile } from './motionProfiles.js'

export function getAssetMetadata(cell = {}) {
  const { category, scene } = getAssetIntelligence(cell)
  const provider = getAssetProviderLabel(cell)
  const status = normalizeStatus(cell)
  const motion = inferMotionProfile(cell)
  const title = cell.fullName || cell.name || 'Untitled Asset'
  const task = cell.generation?.taskId ? String(cell.generation.taskId).slice(0, 14) : 'none'
  const source = getSourceLabel(cell)

  return {
    title,
    subtitle: category.label,
    accent: cell.accent || '#72a4bf',
    insightSource: cell.intelligence?.configured ? `${cell.intelligence.provider || 'AI'} vision analysis` : 'asset name, source file, and generation metadata',
    facts: [
      ['Category', category.label],
      ['Source', source],
      ['Provider', provider],
      ['Status', status],
      ['Scene', scene.label],
      ['Analyzer', cell.intelligence?.configured ? `${cell.intelligence.provider || 'AI'} vision` : 'Local rules'],
      ['Scale', category.scale],
      ['Task', task],
    ],
    description: buildDescription(cell, category, scene),
    value: buildValue(cell, category, scene, motion),
    tags: dedupeTags([...category.tags, ...scene.badges, provider.toLowerCase(), status.toLowerCase().replace(/\s+/g, '-')]).slice(0, 8),
  }
}

function buildDescription(cell, category, scene) {
  if (cell.reference) {
    return cell.referenceSummary || category.description
  }

  const modelState = cell.generation?.modelUrl
    ? 'A generated GLB is available, so the viewer is showing the actual cached 3D model.'
    : cell.generation?.provider === 'cinematic'
    ? 'This is currently a browser-side depth preview rather than a full GLB mesh.'
    : 'The viewer may use a procedural preview until the generated GLB is ready.'

  return `${category.description} ${modelState} The selected presentation scene is ${scene.label}: ${scene.summary}`
}

function buildValue(cell, category, scene, motion) {
  const material = `Material focus: ${category.material}.`
  const structure = `Inspection focus: ${category.inspectionFocus}.`
  const demo = `Recommended presentation: ${motion.label}. ${scene.summary} ${category.value}`
  const warning = cell.generation?.status === 'failed'
    ? ' Current generation failed, so this asset should not be used for a final demo until retried.'
    : ''

  return `${material} ${structure} ${demo}${warning}`
}

function getSourceLabel(cell) {
  if (cell.reference) return 'Khronos reference model'
  if (cell.generation?.provider === 'local') return 'Local GLB import'
  if (cell.imageUrl || cell.thumbnailUrl) return 'Uploaded reference image'
  if (cell.custom) return 'Generated workspace asset'
  return 'Built-in starter scene'
}

function getAssetProviderLabel(cell) {
  if (cell.reference) return 'Khronos Reference'
  if (!cell.custom && !cell.generation?.provider && !cell.generation?.requestedProvider) return 'Built-in'
  return getProviderLabel(cell.generation?.provider || cell.generation?.requestedProvider)
}

function normalizeStatus(cell) {
  if (cell.reference) return 'Reference ready'
  if (cell.generation?.modelUrl) return 'GLB ready'
  if (cell.generation?.status === 'failed') return 'Generation failed'
  if (cell.generation?.status) return String(cell.generation.status)
  return cell.custom ? 'Queued' : 'Interactive starter'
}

function dedupeTags(tags) {
  return [...new Set(tags.filter(Boolean).map((tag) => String(tag).trim()).filter(Boolean))]
}
