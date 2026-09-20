import { AlertTriangle, CheckCircle2, Clock3, RotateCcw } from 'lucide-react'

import { getProviderLabel } from '../services/modelApi.js'
import { CellThumb } from './CellThumb.jsx'

const ACTIVE_STATUSES = new Set(['uploading', 'processing', 'queued', 'running', 'pending'])
const SUCCESS_STATUSES = new Set(['success', 'local'])

export function GenerationTaskCenter({ customCells = [], generationHistory = [], selectedCell, onOpenCell, onRetryGeneration, onRunProviderCompare }) {
  const tasks = customCells
    .filter((cell) => cell.generation && !cell.reference)
    .slice(0, 6)
  const selectedCustomCell = customCells.find((cell) => cell.id === selectedCell && cell.imageUrl)
  const recentHistory = generationHistory.slice(0, 4)

  const activeCount = tasks.filter((cell) => ACTIVE_STATUSES.has(String(cell.generation?.status || '').toLowerCase())).length

  return (
    <section className="panel task-panel">
      <header className="panel-title">
        <span>Generation Queue</span>
        <small>{activeCount || tasks.length}</small>
      </header>
      {selectedCustomCell && (
        <div className="task-actions">
          <button type="button" onClick={() => onRunProviderCompare(selectedCustomCell.id)}>Compare Providers</button>
        </div>
      )}
      {tasks.length === 0 ? (
        <div className="task-empty">
          <Clock3 size={15} />
          <span>Upload an image or GLB to start a model job.</span>
        </div>
      ) : (
        <div className="task-list">
          {tasks.map((cell) => {
            const generation = cell.generation || {}
            const status = String(generation.status || 'pending').toLowerCase()
            const failed = status === 'failed'
            const done = SUCCESS_STATUSES.has(status)
            const active = ACTIVE_STATUSES.has(status)
            const providerLabel = getProviderLabel(generation.provider || generation.requestedProvider)

            return (
              <div key={cell.id} className={selectedCell === cell.id ? 'task-row active' : 'task-row'}>
                <button type="button" className="task-open" onClick={() => onOpenCell(cell.id)}>
                  <CellThumb cell={cell} selected={selectedCell === cell.id} />
                  <span>
                    <strong>{cell.name}</strong>
                    <small>{providerLabel} · {formatTaskStatus(status, generation.progress)}</small>
                  </span>
                </button>
                <span className={failed ? 'task-state failed' : done ? 'task-state done' : active ? 'task-state active' : 'task-state'}>
                  {failed ? <AlertTriangle size={14} /> : done ? <CheckCircle2 size={14} /> : <Clock3 size={14} />}
                </span>
                {failed && (
                  <button type="button" className="task-retry" onClick={() => onRetryGeneration(cell.id)} aria-label={`Retry ${cell.name}`}>
                    <RotateCcw size={13} />
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}
      {recentHistory.length > 0 && (
        <div className="task-history">
          <strong>History</strong>
          {recentHistory.map((item) => (
            <button key={item.id} type="button" className="task-history-row" onClick={() => onOpenCell(item.cellId)}>
              <span>{item.cellName}</span>
              <small>{getProviderLabel(item.provider)} · {formatTaskStatus(String(item.status || '').toLowerCase(), item.progress)}</small>
            </button>
          ))}
        </div>
      )}
    </section>
  )
}

function formatTaskStatus(status, progress) {
  if (status === 'success') return 'ready'
  if (status === 'local') return 'local ready'
  if (status === 'failed') return 'failed'
  if (Number.isFinite(progress)) return `${progress}%`
  if (status === 'uploading') return 'uploading'
  if (status === 'processing' || status === 'running') return 'generating'
  if (status === 'queued' || status === 'pending') return 'queued'
  return status || 'pending'
}
