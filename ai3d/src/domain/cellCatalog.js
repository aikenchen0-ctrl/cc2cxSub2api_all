import { CUSTOM_CELL_STORAGE_KEY } from '../config/appConfig.js'
import { loadStoredValue } from '../lib/storage.js'
import {
  CELL_DETAIL_OVERRIDES,
  CELL_PROFILES,
  CELL_TYPES,
  DEFAULT_ORGANELLE_BY_CELL,
  KHRONOS_REFERENCE_CELLS,
  ORGANELLES,
  ORGANELLE_ORDER,
  SEEDED_GENERATED_CELLS,
} from './cellData.js'

export function getStoredCustomCells() {
  return loadStoredValue(CUSTOM_CELL_STORAGE_KEY, [])
}

export function getPrimaryCells(customCells = getStoredCustomCells()) {
  const activeCustomCells = customCells.filter((cell) => cell.generation?.status !== 'failed')
  const failedCustomCells = customCells.filter((cell) => cell.generation?.status === 'failed')

  return [...activeCustomCells, ...SEEDED_GENERATED_CELLS, ...failedCustomCells, ...CELL_TYPES]
}

export function getAllCells(customCells = getStoredCustomCells()) {
  return [...getPrimaryCells(customCells), ...KHRONOS_REFERENCE_CELLS]
}

export function getCell(cellId, customCells = getStoredCustomCells()) {
  return getAllCells(customCells).find((cell) => cell.id === cellId) ?? CELL_TYPES[1]
}

export function getCustomCell(cellId, customCells = getStoredCustomCells()) {
  return [...customCells, ...SEEDED_GENERATED_CELLS, ...KHRONOS_REFERENCE_CELLS].find((cell) => cell.id === cellId)
}

export function getModelCellId(cellId, customCells = getStoredCustomCells()) {
  return getCustomCell(cellId, customCells)?.template ?? cellId
}

export function getCellProfile(cellId, customCells = getStoredCustomCells()) {
  const customCell = getCustomCell(cellId, customCells)
  if (customCell) {
    const baseProfile = CELL_PROFILES[customCell.template] ?? CELL_PROFILES.animal
    if (customCell.reference) {
      return {
        ...baseProfile,
        summary: customCell.referenceSummary,
        comparison: `${customCell.name} is a Khronos glTF reference asset for inspecting material behavior and GLB loader compatibility, not a biological teaching model.`,
        occurs: customCell.referenceSource,
        organelles: baseProfile.organelles,
      }
    }

    const hasGeneratedModel = Boolean(customCell.generation?.modelUrl)
    const isCinematic = customCell.generation?.provider === 'cinematic'
    return {
      ...baseProfile,
      summary: isCinematic
        ? 'Browser-generated JS depth relief from the uploaded image. This is a visual fallback, not a real GLB mesh.'
        : hasGeneratedModel
        ? 'AI-generated GLB from the uploaded image, loaded as an interactive WebGL model.'
        : 'Uploaded source asset queued for image-to-3D generation. A procedural preview is used only while the GLB is unavailable.',
      comparison: isCinematic
        ? 'This custom sample uses a browser-generated displacement mesh plus transparent depth slabs, not a GLB or full AI-generated mesh.'
        : hasGeneratedModel
        ? 'This custom sample is loaded as a real generated GLB in the WebGL viewer.'
        : 'This custom sample will use a generic procedural preview while generation is running.',
      occurs: 'Uploaded by the user as a custom 3D model source.',
      organelles: ORGANELLE_ORDER,
    }
  }

  return CELL_PROFILES[cellId] ?? CELL_PROFILES['white-blood']
}

export function getAvailableOrganelleIds(cellId, customCells = getStoredCustomCells()) {
  return getCellProfile(cellId, customCells).organelles ?? ORGANELLE_ORDER
}

export function getDefaultOrganelle(cellId, customCells = getStoredCustomCells()) {
  const available = getAvailableOrganelleIds(cellId, customCells)
  const preferred = DEFAULT_ORGANELLE_BY_CELL[cellId] ?? available[0]
  return available.includes(preferred) ? preferred : available[0]
}

export function getOrganelleDetail(cellId, organelleId, customCells = getStoredCustomCells()) {
  const customCell = getCustomCell(cellId, customCells)
  const detailCellId = customCell ? null : cellId

  return {
    ...ORGANELLES[organelleId],
    ...(detailCellId ? CELL_DETAIL_OVERRIDES[detailCellId]?.[organelleId] ?? {} : {}),
  }
}

export function getGenerationPrompt(cell) {
  if (cell.intelligence?.generationPrompt) {
    return [
      cell.intelligence.generationPrompt,
      'Make it a single integrated object, not a flat relief, not a display base.',
      'Preserve the recognizable silhouette, major volumes, surface details, and material separation.',
      'Style: polished interactive 3D studio asset, clean PBR materials, soft studio lighting.',
    ].join(' ')
  }

  return [
    `A high quality 3D model generated from the uploaded reference image named ${cell.name}.`,
    'Make it a single integrated object, not a flat relief, not a display base.',
    'Preserve the recognizable silhouette, major volumes, surface details, and material separation.',
    'Style: polished interactive 3D studio asset, clean PBR materials, soft studio lighting.',
  ].join(' ')
}

export function getGeneratedModelUrl(cell) {
  return cell.custom ? cell.generation?.modelUrl || '' : ''
}

export function cleanFileName(fileName) {
  return fileName.replace(/\.[^.]+$/, '').replace(/[-_]+/g, ' ').trim()
}

export function inferCellTemplate(fileName) {
  const name = fileName.toLowerCase()
  if (name.includes('plant') || name.includes('leaf') || name.includes('chloroplast')) return 'plant'
  if (name.includes('bacteria') || name.includes('bacillus') || name.includes('microbe')) return 'bacteria'
  if (name.includes('neuron') || name.includes('nerve')) return 'neuron'
  if (name.includes('muscle') || name.includes('fiber')) return 'muscle'
  if (name.includes('epithelial') || name.includes('tissue')) return 'epithelial'
  if (name.includes('blood') || name.includes('immune') || name.includes('wbc')) return 'white-blood'
  return 'animal'
}

export function isLocalModelFile(file) {
  return /\.(?:glb|gltf)$/i.test(file.name)
}

export function createCustomCell(fileName, imageUrl, options = {}) {
  const template = inferCellTemplate(fileName)
  const base = getCell(template)
  const name = cleanFileName(fileName) || 'Uploaded Model'
  const provider = options.provider || 'tripo'

  return {
    id: `custom-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: name.length > 20 ? `${name.slice(0, 20)}...` : name,
    fullName: name,
    sourceFileName: fileName,
    type: options.type || 'Uploaded 3D Asset',
    accent: base.accent,
    custom: true,
    template,
    imageUrl,
    thumbnailUrl: options.thumbnailUrl || '',
    generation: {
      provider,
      requestedProvider: options.requestedProvider || provider,
      status: options.status || 'queued',
      taskId: options.taskId || '',
      modelUrl: options.modelUrl || '',
      rawModelUrl: options.rawModelUrl || '',
      message: options.message || 'Waiting for image-to-3D generation.',
    },
  }
}

export function getUploadPreviewFromCustomCells(customCells) {
  const latest = customCells.find((cell) => cell.custom)
  if (!latest) return null
  return { name: latest.name, url: latest.imageUrl || latest.thumbnailUrl || '' }
}
