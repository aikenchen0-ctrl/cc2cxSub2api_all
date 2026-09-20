import { CUSTOM_CELL_STORAGE_KEY } from '../config/appConfig.js'
import { storeValue } from '../lib/storage.js'

export function persistCustomCells(cells) {
  if (storeValue(CUSTOM_CELL_STORAGE_KEY, cells)) {
    return { cells, stored: true, compacted: false }
  }

  const withoutGeneratedPreviews = compactCustomCellsForStorage(cells, 'generated-previews')
  if (storeValue(CUSTOM_CELL_STORAGE_KEY, withoutGeneratedPreviews)) {
    return { cells: withoutGeneratedPreviews, stored: true, compacted: true }
  }

  const withoutAllPreviews = compactCustomCellsForStorage(cells, 'all-previews')
  if (storeValue(CUSTOM_CELL_STORAGE_KEY, withoutAllPreviews)) {
    return { cells: withoutAllPreviews, stored: true, compacted: true }
  }

  const minimal = compactCustomCellsForStorage(cells, 'minimal')
  return {
    cells: minimal,
    stored: storeValue(CUSTOM_CELL_STORAGE_KEY, minimal),
    compacted: true,
  }
}

export function compactCustomCellsForStorage(cells, mode) {
  let changed = false
  const compacted = cells.map((cell) => {
    if (mode === 'generated-previews' && !canDropPreview(cell)) return cell
    if (mode !== 'minimal' && !cell.imageUrl && cell.previewDropped) return cell

    const next = {
      ...cell,
      imageUrl: '',
      previewDropped: true,
    }

    if (mode !== 'minimal') {
      changed = true
      return next
    }

    const nextMessage = shortenMessage(next.generation?.message)
    if (!cell.imageUrl && cell.previewDropped && !next.generation?.rawModelUrl && next.generation?.message === nextMessage) return cell

    changed = true
    return {
      ...next,
      generation: {
        ...next.generation,
        rawModelUrl: '',
        message: shortenMessage(next.generation?.message),
      },
    }
  })

  return changed ? compacted : cells
}

function canDropPreview(cell) {
  const generation = cell.generation || {}
  return Boolean(generation.modelUrl) || ['success', 'local'].includes(String(generation.status || '').toLowerCase())
}

function shortenMessage(message) {
  const value = String(message || '')
  return value.length > 180 ? `${value.slice(0, 177)}...` : value
}
