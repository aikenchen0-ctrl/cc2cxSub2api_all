import { AlertTriangle, CheckCircle2, ChevronDown, Clock3, Heart, RotateCcw, Sparkles as SparklesIcon, Trash2 } from 'lucide-react'

import { CELL_TYPES } from '../domain/cellData.js'
import { getCell, getPrimaryCells } from '../domain/cellCatalog.js'
import { t } from '../i18n.js'
import { getProviderLabel } from '../services/modelApi.js'
import { CellThumb } from './CellThumb.jsx'

const ACTIVE_STATUSES = new Set(['uploading', 'processing', 'queued', 'running', 'pending'])
const READY_STATUSES = new Set(['success', 'local'])

export function LeftSidebar({ selectedCell, setSelectedCell, customCells, onDeleteCustomCell, onRetryGeneration, language = 'zh' }) {
  const libraryCells = getPrimaryCells(customCells).filter((cell) => cell.custom && !cell.reference)
  const activeModel = getCell(selectedCell, customCells)
  const activeIsCustom = Boolean(activeModel.custom && !activeModel.reference)
  const recentCells = libraryCells.filter((cell) => cell.id !== activeModel.id)
  const starterCells = CELL_TYPES.filter((cell) => cell.id !== activeModel.id)
  const queueItems = libraryCells.filter((cell) => cell.generation)
  const storedCustomIds = new Set(customCells.map((cell) => cell.id))
  const queueCount = queueItems.filter((cell) => ACTIVE_STATUSES.has(String(cell.generation?.status || '').toLowerCase())).length || queueItems.length

  function renderCellRow(cell, { compact = false } = {}) {
    const canDelete = Boolean(cell.custom && storedCustomIds.has(cell.id))
    const generation = cell.generation || {}
    const providerLabel = cell.custom ? getProviderLabel(generation.provider || generation.requestedProvider) : t(language, 'starter')
    const status = cell.custom ? formatQueueStatus(String(generation.status || 'ready').toLowerCase(), generation.progress, language) : t(language, 'ready')

    return (
      <div key={cell.id} className={canDelete ? 'cell-row-shell can-delete' : 'cell-row-shell'}>
        <button
          type="button"
          className={`${selectedCell === cell.id ? 'cell-row active' : 'cell-row'}${compact ? ' compact' : ''}`}
          onClick={() => setSelectedCell(cell.id)}
        >
          <CellThumb cell={cell} selected={selectedCell === cell.id} />
          <span>
            <strong>{cell.name}</strong>
            <small>{providerLabel} · {status}</small>
          </span>
          {!canDelete && selectedCell === cell.id && <Heart size={13} fill="currentColor" />}
        </button>
        {canDelete && (
          <button type="button" className="cell-delete" aria-label={`Delete ${cell.name}`} onClick={() => onDeleteCustomCell?.(cell.id)}>
            <Trash2 size={12} />
          </button>
        )}
      </div>
    )
  }

  return (
    <aside className="left-rail">
      <section className="panel cell-types-panel">
        <header className="panel-title">
          <span>
            <SparklesIcon size={14} />
            {t(language, 'modelLibrary')}
          </span>
          <ChevronDown size={14} />
        </header>
        <div className="pinned-models">
          <div className="pinned-model-block">
            <span className="model-section-label">{activeIsCustom ? t(language, 'activeAsset') : t(language, 'activeStarter')}</span>
            {renderCellRow(activeModel)}
          </div>
        </div>
        <div className="cell-list">
          {recentCells.length > 0 && (
            <div className="recent-cells">
              <div className="recent-toggle" aria-expanded="true">
                <span>{t(language, 'savedAssets')}</span>
                <small>{recentCells.length}</small>
                <ChevronDown size={13} />
              </div>
              <div className="recent-cell-list">
                {recentCells.map((cell) => renderCellRow(cell, { compact: true }))}
              </div>
            </div>
          )}
          <div className="starter-cells">
            <span className="model-section-label">{t(language, 'starterModels')}</span>
            <div className="starter-cell-list">
              {starterCells.map((cell) => renderCellRow(cell, { compact: true }))}
            </div>
          </div>
          {libraryCells.length === 0 && (
            <div className="library-empty compact-empty">
              <SparklesIcon size={16} />
              <span>{t(language, 'noSavedUploads')}</span>
              <small>{t(language, 'useAssetSource')}</small>
            </div>
          )}
        </div>
      </section>

      <section className="panel organelles-panel">
        <header className="panel-title">
          <span>
            <Clock3 size={14} />
            {t(language, 'generationQueue')}
          </span>
          <small>{queueCount}</small>
        </header>
        {queueItems.length === 0 ? (
          <div className="queue-empty">
            <Clock3 size={15} />
            <span>{t(language, 'noJobs')}</span>
          </div>
        ) : (
          <div className="left-queue-list">
            {queueItems.map((cell) => {
              const generation = cell.generation || {}
              const status = String(generation.status || 'pending').toLowerCase()
              const failed = status === 'failed'
              const ready = READY_STATUSES.has(status)
              const active = ACTIVE_STATUSES.has(status)

              return (
                <div key={cell.id} className={selectedCell === cell.id ? 'left-queue-row active' : 'left-queue-row'}>
                  <button type="button" onClick={() => setSelectedCell(cell.id)}>
                    <CellThumb cell={cell} selected={selectedCell === cell.id} />
                    <span>
                      <strong>{cell.name}</strong>
                      <small>{getProviderLabel(generation.provider || generation.requestedProvider)} · {formatQueueStatus(status, generation.progress, language)}</small>
                    </span>
                  </button>
                  <span className={failed ? 'queue-state failed' : ready ? 'queue-state ready' : active ? 'queue-state active' : 'queue-state'}>
                    {failed ? <AlertTriangle size={13} /> : ready ? <CheckCircle2 size={13} /> : <Clock3 size={13} />}
                  </span>
                  {failed && (
                    <button type="button" className="queue-retry" onClick={() => onRetryGeneration?.(cell.id)} aria-label={`Retry ${cell.name}`}>
                      <RotateCcw size={12} />
                    </button>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </section>
    </aside>
  )
}

function formatQueueStatus(status, progress, language = 'zh') {
  if (status === 'success') return t(language, 'ready')
  if (status === 'local') return t(language, 'localReady')
  if (status === 'failed') return t(language, 'failed')
  if (Number.isFinite(progress)) return `${progress}%`
  if (status === 'uploading') return t(language, 'uploading')
  if (status === 'processing' || status === 'running') return t(language, 'generating')
  if (status === 'queued' || status === 'pending') return t(language, 'queued')
  return status || t(language, 'pending')
}
