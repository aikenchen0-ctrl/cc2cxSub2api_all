import { motion } from 'framer-motion'
import { Box, CheckCircle2, Clock3, Copy, Download, Edit3, Image, Layers3, RefreshCw, RotateCcw, Trash2, X } from 'lucide-react'

import { FAL_MODEL_OPTIONS, LANGUAGE_OPTIONS, SCREENSHOT_SCALE_OPTIONS } from '../config/appConfig.js'
import { CELL_TYPES } from '../domain/cellData.js'
import { getCell, getCellProfile, getOrganelleDetail } from '../domain/cellCatalog.js'
import { modeLabel, t } from '../i18n.js'
import { getAvailableGenerationModes, getProviderLabel } from '../services/modelApi.js'
import { CellThumb } from './CellThumb.jsx'

const READY_STATUSES = new Set(['success', 'local'])
const ACTIVE_STATUSES = new Set(['uploading', 'processing', 'queued', 'running', 'pending'])

function findCell(cells, cellId) {
  return cells.find((cell) => cell.id === cellId) ?? getCell(cellId)
}

function formatDate(value, language = 'zh') {
  if (!value) return t(language, 'notSaved')
  return new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function formatDuration(ms) {
  if (!Number.isFinite(ms)) return 'n/a'
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${Math.round(ms / 1000)} s`
}

function getModelUrl(cell) {
  const generation = cell.generation || {}
  const modelUrl = generation.modelUrl || ''
  const taskId = generation.taskId
  if (taskId && (/\/api\/3d\/model\?/i.test(modelUrl) || /^https?:\/\//i.test(modelUrl))) {
    return `/api/3d/local-model/${encodeURIComponent(taskId)}.glb`
  }
  return modelUrl
}

function getModelSource(cell) {
  if (cell.reference) return 'Khronos glTF Sample Models'
  if (cell.generation?.provider === 'local') return 'Local GLB import'
  if (cell.generation?.provider === 'cinematic') return 'Browser JS Depth'
  if (cell.custom) return `${cell.generation?.provider || 'AI'} generation`
  return 'Procedural Three.js scene'
}

function getQualityLabel(cell) {
  if (cell.reference) return 'Reference GLB'
  if (cell.generation?.modelUrl) return 'GLB ready'
  if (cell.generation?.status === 'failed') return 'Failed'
  if (cell.generation?.status) return cell.generation.status
  return 'Interactive'
}

function getAssetPreviewUrl(cell) {
  return cell.thumbnailUrl || cell.imageUrl || ''
}

function formatAssetStatus(cell) {
  const status = String(cell.generation?.status || '').toLowerCase()
  if (cell.reference) return 'reference'
  if (READY_STATUSES.has(status) || cell.generation?.modelUrl) return 'ready'
  if (status === 'failed') return 'failed'
  if (ACTIVE_STATUSES.has(status)) return 'generating'
  if (cell.custom) return 'queued'
  return 'starter'
}

function getAssetTone(cell) {
  const status = formatAssetStatus(cell)
  if (status === 'ready' || status === 'reference') return 'ready'
  if (status === 'failed') return 'failed'
  if (status === 'generating' || status === 'queued') return 'active'
  return 'starter'
}

function getAssetKind(cell) {
  if (cell.reference) return 'Reference GLB'
  if (cell.generation?.provider === 'local') return 'Local Import'
  if (cell.generation?.provider === 'cinematic') return 'JS Depth Preview'
  if (cell.generation?.modelUrl) return 'Generated GLB'
  if (cell.custom) return 'Generated Asset'
  return 'Starter Scene'
}

function getAssetRuntime(cell, generationHistory) {
  const match = generationHistory.find((entry) => entry.cellId === cell.id && Number.isFinite(entry.durationMs))
  return match ? formatDuration(match.durationMs) : 'n/a'
}

function formatLogSummary(entry) {
  const parts = [
    entry.method,
    entry.path,
    entry.provider,
    entry.status ? `status=${entry.status}` : '',
    entry.progress !== undefined && entry.progress !== null ? `progress=${entry.progress}` : '',
    entry.taskId ? `task=${String(entry.taskId).slice(0, 18)}` : '',
    entry.durationMs !== undefined ? `duration=${formatDuration(entry.durationMs)}` : '',
    entry.error?.message || entry.error || '',
  ].filter(Boolean)

  return parts.join(' · ') || JSON.stringify(entry).slice(0, 160)
}

export function WorkspaceDrawer({
  activePanel,
  selectedCell,
  selectedOrganelle,
  compareCell,
  allCells = CELL_TYPES,
  customCells = [],
  galleryItems,
  generationHistory = [],
  notes,
  settings,
  projects = [],
  crossSection,
  selectedMicroscope,
  uploadedImage,
  favoriteKey,
  onClose,
  onSelectCell,
  onSelectOrganelle,
  onSetCompareCell,
  onSaveGallery,
  onClearGallery,
  onRestoreGalleryItem,
  onRenameGalleryItem,
  onDeleteGalleryItem,
  onDownloadGalleryImage,
  onExportGallery,
  onDeleteCustomCell,
  onClearGenerationHistory,
  onUpdateNote,
  onGenerateNote,
  onCopyNote,
  onExportNote,
  onUpdateSettings,
  onSetCrossSection,
  onExport,
  exportAvailable,
  exportReason,
  apiHealth,
  serverLogs,
  onRefreshApiHealth,
  onRefreshServerLogs,
  onExportDiagnostics,
  onClearWorkspaceCache,
  onResetWorkspace,
  onSaveProject,
  onLoadProject,
  onDeleteProject,
  onExportProject,
  onRunProviderCompare,
  onCopyText,
}) {
  if (!activePanel) return null

  const language = settings?.language || 'zh'
  const cell = findCell(allCells, selectedCell)
  const compare = findCell(allCells, compareCell)
  const detail = getOrganelleDetail(selectedCell, selectedOrganelle, customCells)
  const profile = getCellProfile(selectedCell, customCells)
  const noteKey = `${selectedCell}:${selectedOrganelle}`
  const noteValue = notes[noteKey] ?? ''
  const savedFavorite = favoriteKey ? favoriteKey.replace(':', ' / ') : 'None'
  const generatedAssets = allCells.filter((item) => item.custom && !item.reference)
  const referenceAssets = allCells.filter((item) => item.reference)
  const starterAssets = allCells.filter((item) => !item.custom && !item.reference)
  const readyGeneratedAssets = generatedAssets.filter((item) => formatAssetStatus(item) === 'ready')
  const generationModes = getAvailableGenerationModes(apiHealth, settings.generationMode)

  function renderAssetCard(item, { compact = false } = {}) {
    const modelUrl = getModelUrl(item)
    const previewUrl = getAssetPreviewUrl(item)
    const providerLabel = getProviderLabel(item.generation?.provider || item.generation?.requestedProvider || (item.reference ? 'reference' : 'built-in'))
    const canDelete = customCells.some((candidate) => candidate.id === item.id) && !item.reference
    const canCompare = item.custom && !item.reference && Boolean(item.imageUrl)
    const status = formatAssetStatus(item)
    const taskId = item.generation?.taskId || ''

    return (
      <article key={item.id} className={`${selectedCell === item.id ? 'asset-library-card active' : 'asset-library-card'} tone-${getAssetTone(item)}${compact ? ' compact' : ''}`}>
        <button type="button" className="asset-preview-frame" onClick={() => onSelectCell(item.id)} aria-label={`Open ${item.name}`}>
          {previewUrl ? <img src={previewUrl} alt={`${item.name} source preview`} /> : <CellThumb cell={item} selected={selectedCell === item.id} />}
        </button>
        <div className="asset-library-body">
          <div className="asset-library-title">
            <span>
              <strong title={item.fullName || item.name}>{item.fullName || item.name}</strong>
              <small>{getAssetKind(item)} · {providerLabel}</small>
            </span>
            <span className={`asset-status-pill ${status}`}>
              {status === 'ready' || status === 'reference' ? <CheckCircle2 size={12} /> : status === 'failed' ? <X size={12} /> : <Clock3 size={12} />}
              {status}
            </span>
          </div>
          <div className="asset-stat-grid">
            <span><strong>{modelUrl ? 'GLB' : 'Preview'}</strong><small>asset</small></span>
            <span><strong>{getAssetRuntime(item, generationHistory)}</strong><small>runtime</small></span>
            <span><strong>{taskId ? String(taskId).slice(0, 8) : 'none'}</strong><small>task</small></span>
          </div>
          <code className="asset-model-url">{modelUrl || item.referenceSource || item.type || 'Procedural preview only'}</code>
          <div className="asset-library-actions">
            <button type="button" onClick={() => onSelectCell(item.id)}>{t(language, 'open')}</button>
            <button type="button" disabled={!modelUrl} onClick={() => onCopyText(modelUrl, 'Model URL copied')}>
              <Copy size={12} />
              URL
            </button>
            <button type="button" disabled={!canCompare} onClick={() => onRunProviderCompare(item.id)}>
              <RotateCcw size={12} />
              Compare
            </button>
            {canDelete && (
              <button type="button" className="danger" onClick={() => onDeleteCustomCell?.(item.id)}>
                <Trash2 size={12} />
              </button>
            )}
          </div>
        </div>
      </article>
    )
  }

  function renderContent() {
    if (activePanel === 'Gallery') {
      return (
        <div className="drawer-content">
          <div className="gallery-hero">
            <CellThumb cell={cell} selected />
            <div>
              <strong>{cell.name}</strong>
              <span>{detail.title} · {selectedMicroscope}</span>
            </div>
          </div>
          <div className="drawer-actions">
            <button type="button" className="drawer-primary" onClick={onSaveGallery}>{t(language, 'saveView')}</button>
            <button type="button" className="drawer-secondary" onClick={onExport} disabled={!exportAvailable} title={exportReason}>{t(language, 'exportGlb')}</button>
          </div>
          {uploadedImage && (
            <div className="uploaded-tile" style={{ '--upload-preview': `url(${uploadedImage.url})` }}>
              <span />
              <div>
                <strong>{uploadedImage.name}</strong>
                <small>{t(language, 'attachedSource')}</small>
              </div>
            </div>
          )}
          <div className="drawer-list">
            {galleryItems.length === 0 ? (
              <p className="empty-state">{t(language, 'noSavedViews')}</p>
            ) : (
              galleryItems.map((item) => {
                const itemCell = findCell(allCells, item.cellId)
                const itemDetail = getOrganelleDetail(item.cellId, item.organelleId, customCells)
                return (
                  <article key={item.id} className="gallery-shot-card">
                    <button type="button" className="gallery-shot-preview" onClick={() => onRestoreGalleryItem(item)}>
                      {item.thumbnailUrl ? <img src={item.thumbnailUrl} alt={`${item.title || itemCell.name} saved view`} /> : <CellThumb cell={itemCell} selected={item.cellId === selectedCell} />}
                    </button>
                    <div className="gallery-shot-body">
                      <strong>{item.title || `${itemCell.name} / ${itemDetail.title}`}</strong>
                      <small>{itemCell.name} · {itemDetail.title} · {item.microscope}</small>
                      <small>{getQualityLabel({ generation: { provider: item.generationProvider, modelUrl: item.modelUrl } })} · {formatDate(item.createdAt)}</small>
                    </div>
                    <div className="gallery-shot-actions">
                      <button type="button" onClick={() => onRestoreGalleryItem(item)}>{t(language, 'open')}</button>
                      <button
                        type="button"
                        onClick={() => {
                          const title = window.prompt('Rename saved view', item.title || `${itemCell.name} / ${itemDetail.title}`)
                          if (title !== null) onRenameGalleryItem(item.id, title)
                        }}
                      >
                        <Edit3 size={12} />
                      </button>
                      <button type="button" onClick={() => onDownloadGalleryImage(item)} disabled={!item.thumbnailUrl}>
                        <Download size={12} />
                      </button>
                      <button type="button" onClick={() => onDeleteGalleryItem(item.id)}>
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </article>
                )
              })
            )}
          </div>
          {galleryItems.length > 0 && (
            <div className="drawer-actions">
              <button type="button" className="drawer-secondary" onClick={onExportGallery}>{t(language, 'exportGallery')}</button>
              <button type="button" className="drawer-secondary" onClick={onClearGallery}>{t(language, 'clearGallery')}</button>
            </div>
          )}
        </div>
      )
    }

    if (activePanel === 'Library') {
      return (
        <div className="drawer-content asset-library-drawer">
          <div className="asset-library-summary">
            <span><strong>{generatedAssets.length}</strong><small>generated/imported</small></span>
            <span><strong>{readyGeneratedAssets.length}</strong><small>ready GLB</small></span>
            <span><strong>{referenceAssets.length}</strong><small>references</small></span>
          </div>

          <section className="asset-library-section">
            <header className="asset-section-head">
              <span>
                <Box size={15} />
                <strong>{t(language, 'generatedImportedAssets')}</strong>
              </span>
              <small>{readyGeneratedAssets.length}/{generatedAssets.length} {t(language, 'ready')}</small>
            </header>
            {generatedAssets.length === 0 ? (
              <div className="asset-library-empty">
                <Image size={18} />
                <span>{t(language, 'noGeneratedAssets')}</span>
                <small>{t(language, 'uploadFromAssetSource')}</small>
              </div>
            ) : (
              <div className="asset-card-grid">
                {generatedAssets.map((item) => renderAssetCard(item))}
              </div>
            )}
          </section>

          <section className="asset-library-section">
            <header className="asset-section-head">
              <span>
                <Layers3 size={15} />
                <strong>{t(language, 'khronosRef')}</strong>
              </span>
              <small>{t(language, 'materialChecks')}</small>
            </header>
            <div className="asset-card-grid compact">
              {referenceAssets.map((item) => renderAssetCard(item, { compact: true }))}
            </div>
          </section>

          <details className="asset-library-section starter-assets">
            <summary>
              <span>{t(language, 'starterScenes')}</span>
              <small>{starterAssets.length}</small>
            </summary>
            <div className="starter-asset-grid">
              {starterAssets.map((item) => (
                <button key={item.id} type="button" className={selectedCell === item.id ? 'starter-asset active' : 'starter-asset'} onClick={() => onSelectCell(item.id)}>
                  <CellThumb cell={item} selected={selectedCell === item.id} />
                  <span>
                    <strong>{item.name}</strong>
                    <small>{item.type}</small>
                  </span>
                </button>
              ))}
            </div>
          </details>
        </div>
      )
    }

    if (activePanel === 'Notebooks') {
      const noteEntries = Object.entries(notes)
      return (
        <div className="drawer-content">
          <label className="note-editor">
            <span>{cell.name} / {detail.title}</span>
            <textarea
              value={noteValue}
              onChange={(event) => onUpdateNote(noteKey, event.target.value)}
              placeholder="Record observations, questions, or narration notes..."
            />
          </label>
          <div className="drawer-actions three">
            <button type="button" className="drawer-primary" onClick={onGenerateNote}>{t(language, 'generateDraft')}</button>
            <button type="button" className="drawer-secondary" onClick={onCopyNote}>{t(language, 'copy')}</button>
            <button type="button" className="drawer-secondary" onClick={onExportNote}>{t(language, 'exportMd')}</button>
          </div>
          <div className="drawer-meta inline">
            <span>{noteValue.length} chars</span>
            <span>{t(language, 'autosaved')}</span>
            <span>{Object.keys(notes).length} notes</span>
          </div>
          <div className="note-archive">
            <strong>{t(language, 'archive')}</strong>
            {noteEntries.length === 0 ? (
              <p className="empty-state">{t(language, 'noArchivedNotes')}</p>
            ) : (
              noteEntries.slice(0, 8).map(([key, value]) => {
                const [cellId, organelleId] = key.split(':')
                const noteCell = findCell(allCells, cellId)
                const noteDetail = getOrganelleDetail(cellId, organelleId, customCells)
                return (
                  <button
                    key={key}
                    type="button"
                    className={key === noteKey ? 'note-archive-row active' : 'note-archive-row'}
                    onClick={() => {
                      onSelectCell(cellId)
                      onSelectOrganelle(organelleId)
                    }}
                  >
                    <span>
                      <strong>{noteCell.name} / {noteDetail.title}</strong>
                      <small>{value.slice(0, 90)}</small>
                    </span>
                  </button>
                )
              })
            )}
          </div>
        </div>
      )
    }

    if (activePanel === 'Logs') {
      const entries = serverLogs?.entries || []
      return (
        <div className="drawer-content">
          <div className="settings-health">
            <div>
              <strong>{t(language, 'diagnosticLogs')}</strong>
              <small>{serverLogs?.file || '.logs/3d-model-studio-api.log'} · {entries.length} entries</small>
            </div>
            <button type="button" className="drawer-secondary" onClick={onRefreshServerLogs}>
              <RefreshCw size={13} />
              {t(language, 'refresh')}
            </button>
            {serverLogs?.error && <p className="empty-state">{serverLogs.error}</p>}
            <div className="drawer-actions">
              <button type="button" className="drawer-primary" onClick={onExportDiagnostics}>{t(language, 'exportDiagnostics')}</button>
              <button type="button" className="drawer-secondary" onClick={onRefreshApiHealth}>{t(language, 'checkApi')}</button>
            </div>
          </div>
          <div className="log-list">
            {entries.length === 0 ? (
              <p className="empty-state">{t(language, 'noServerLogs')}</p>
            ) : (
              entries.slice().reverse().map((entry, index) => (
                <article key={`${entry.ts}-${entry.requestId || index}`} className={`log-row ${entry.level || 'info'}`}>
                  <div>
                    <strong>{entry.event || 'log.event'}</strong>
                    <small>{entry.ts ? formatDate(entry.ts) : 'unknown time'} · {entry.requestId || 'no request id'}</small>
                  </div>
                  <code>{formatLogSummary(entry)}</code>
                </article>
              ))
            )}
          </div>
          <div className="history-panel">
            <div className="project-manager-head">
              <div>
                <strong>{t(language, 'frontendHistory')}</strong>
                <small>{generationHistory.length} local generation records.</small>
              </div>
              <button type="button" className="drawer-secondary" disabled={generationHistory.length === 0} onClick={onClearGenerationHistory}>{t(language, 'clear')}</button>
            </div>
            {generationHistory.length === 0 ? (
              <p className="empty-state">{t(language, 'noFrontendHistory')}</p>
            ) : (
              <div className="history-list">
                {generationHistory.slice(0, 10).map((item) => (
                  <button key={item.id} type="button" className={`history-row ${item.status}`} onClick={() => onSelectCell(item.cellId)}>
                    <span>
                      <strong>{item.cellName || item.cellId}</strong>
                      <small>{item.provider} · {item.status} · {formatDuration(item.durationMs)}</small>
                    </span>
                    <small>{formatDate(item.finishedAt || item.startedAt)}</small>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )
    }

    if (activePanel === 'Settings') {
      return (
        <div className="drawer-content settings-list">
          <label className="settings-row">
            <span>
              <strong>{t(language, 'crossSection')}</strong>
              <small>{t(language, 'crossSectionHint')}</small>
            </span>
            <input type="checkbox" checked={crossSection} onChange={(event) => onSetCrossSection(event.target.checked)} />
          </label>
          <div className="settings-row">
            <span>
              <strong>{t(language, 'renderQuality')}</strong>
              <small>{t(language, 'renderQualityHint')}</small>
            </span>
            <div className="segmented">
              {['balanced', 'high'].map((quality) => (
                <button
                  key={quality}
                  type="button"
                  className={settings.quality === quality ? 'active' : ''}
                  onClick={() => onUpdateSettings({ ...settings, quality })}
                >
                  {t(language, quality)}
                </button>
              ))}
            </div>
          </div>
          <label className="settings-row">
            <span>
              <strong>{t(language, 'defaultGeneration')}</strong>
              <small>{t(language, 'defaultGenerationHint')}</small>
            </span>
            <select
              className="settings-select"
              value={generationModes.some((option) => option.id === settings.generationMode) ? settings.generationMode : (generationModes[0]?.id || 'hunyuan')}
              onChange={(event) => onUpdateSettings({ ...settings, generationMode: event.target.value, generationProvider: event.target.value })}
            >
              {generationModes.filter((option) => option.id !== 'cinematic').map((option) => (
                <option key={option.id} value={option.id}>{modeLabel(language, option.id, option.label)}</option>
              ))}
            </select>
          </label>
          <div className="settings-row">
            <span>
              <strong>{t(language, 'screenshotSize')}</strong>
              <small>{t(language, 'screenshotSizeHint')}</small>
            </span>
            <div className="segmented segmented-three">
              {SCREENSHOT_SCALE_OPTIONS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={settings.screenshotScale === option.id ? 'active' : ''}
                  onClick={() => onUpdateSettings({ ...settings, screenshotScale: option.id })}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
          <div className="settings-row">
            <span>
              <strong>{t(language, 'language')}</strong>
              <small>{t(language, 'languageHint')}</small>
            </span>
            <div className="segmented">
              {LANGUAGE_OPTIONS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={settings.language === option.id ? 'active' : ''}
                  onClick={() => onUpdateSettings({ ...settings, language: option.id })}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
          <label className="settings-row">
            <span>
              <strong>{t(language, 'compactUi')}</strong>
              <small>{t(language, 'compactHint')}</small>
            </span>
            <input type="checkbox" checked={settings.compactUi} onChange={(event) => onUpdateSettings({ ...settings, compactUi: event.target.checked })} />
          </label>
          <label className="settings-row">
            <span>
              <strong>{t(language, 'falModel')}</strong>
              <small>{t(language, 'falHint')}</small>
            </span>
            <select
              className="settings-select"
              value={settings.falModelId}
              onChange={(event) => onUpdateSettings({ ...settings, falModelId: event.target.value })}
            >
              {FAL_MODEL_OPTIONS.map((option) => (
                <option key={option.id} value={option.id} title={option.description}>{option.label}</option>
              ))}
            </select>
          </label>
          <div className="settings-health">
            <div>
              <strong>{t(language, 'apiHealth')}</strong>
              <small>{apiHealth?.checkedAt ? `${t(language, 'checkedAt')} ${formatDate(apiHealth.checkedAt, language)}` : t(language, 'notChecked')}</small>
            </div>
            <button type="button" className="drawer-secondary" onClick={onRefreshApiHealth}>
              <RefreshCw size={13} />
              {t(language, 'refresh')}
            </button>
            {apiHealth?.error ? (
              <p className="empty-state">{apiHealth.error}</p>
            ) : (
              <div className="health-grid">
                {Object.entries(apiHealth?.providers || {}).map(([id, provider]) => (
                  <span key={id} className={provider.configured ? 'healthy' : 'missing'}>
                    <strong>{id}</strong>
                    <small>{provider.configured ? t(language, 'configured') : t(language, 'missingKey')}</small>
                  </span>
                ))}
              </div>
            )}
          </div>
          <div className="drawer-actions">
            <button type="button" className="drawer-secondary" onClick={onClearWorkspaceCache}>{t(language, 'clearCache')}</button>
            <button type="button" className="drawer-secondary danger" onClick={onResetWorkspace}>{t(language, 'resetData')}</button>
          </div>
        </div>
      )
    }

    if (activePanel === 'Compare') {
      return (
        <div className="drawer-content">
          <div className="compare-drawer-grid">
            {[cell, compare].map((item) => {
              const itemProfile = getCellProfile(item.id, customCells)
              return (
                <div key={item.id} className="compare-card">
                  <CellThumb cell={item} selected={item.id === selectedCell} />
                  <strong>{item.name}</strong>
                  <small>{itemProfile.summary}</small>
                </div>
              )
            })}
          </div>
          <p className="drawer-copy">{profile.comparison}</p>
          <div className="cell-chip-grid">
            {allCells.filter((item) => item.id !== selectedCell).map((item) => (
              <button key={item.id} type="button" className={item.id === compareCell ? 'active' : ''} onClick={() => onSetCompareCell(item.id)}>
                {item.name.replace(' Cell', '')}
              </button>
            ))}
          </div>
          <div className="drawer-actions">
            <button type="button" className="drawer-primary" onClick={() => onSelectCell(compareCell)}>{t(language, 'openCompared')}</button>
            <button type="button" className="drawer-secondary" onClick={() => onSetCompareCell(profile.compareTarget)}>{t(language, 'resetTarget')}</button>
          </div>
        </div>
      )
    }

    const modelUrl = getModelUrl(cell)
    const latestHistory = generationHistory.slice(0, 6)

    return (
      <div className="drawer-content">
        <div className="profile-stats">
          <span><strong>{allCells.length}</strong><small>models</small></span>
          <span><strong>{galleryItems.length}</strong><small>saved</small></span>
          <span><strong>{generationHistory.length}</strong><small>runs</small></span>
        </div>
        <div className="model-inspector">
          <div>
            <strong>{t(language, 'modelInspector')}</strong>
            <small>{cell.name} · {getQualityLabel(cell)}</small>
          </div>
          <dl>
            <dt>{t(language, 'sourceLabel')}</dt>
            <dd>{getModelSource(cell)}</dd>
            <dt>{t(language, 'provider')}</dt>
            <dd>{cell.generation?.provider || 'built-in'}</dd>
            <dt>{t(language, 'status')}</dt>
            <dd>{cell.generation?.status || 'interactive'}</dd>
            <dt>{t(language, 'modelUrl')}</dt>
            <dd>{modelUrl || 'procedural scene'}</dd>
            <dt>{t(language, 'task')}</dt>
            <dd>{cell.generation?.taskId || t(language, 'none')}</dd>
          </dl>
          <div className="drawer-actions">
            <button type="button" className="drawer-secondary" disabled={!modelUrl} onClick={() => onCopyText(modelUrl, t(language, 'modelUrlCopied'))}>{t(language, 'copyUrl')}</button>
            <button type="button" className="drawer-primary" disabled={!cell.custom || !cell.imageUrl} onClick={() => onRunProviderCompare(cell.id)}>{t(language, 'providerCompare')}</button>
          </div>
        </div>
        <div className="project-manager">
          <div className="project-manager-head">
            <div>
              <strong>{t(language, 'projects')}</strong>
              <small>{t(language, 'projectsHint')}</small>
            </div>
            <button type="button" className="drawer-primary" onClick={onSaveProject}>{t(language, 'saveProject')}</button>
          </div>
          {projects.length === 0 ? (
            <p className="empty-state">{t(language, 'noProjects')}</p>
          ) : (
            <div className="project-list">
              {projects.map((project) => (
                <article key={project.id} className="project-row">
                  {project.thumbnailUrl ? <img src={project.thumbnailUrl} alt={`${project.name} project thumbnail`} /> : <CellThumb cell={cell} />}
                  <div>
                    <strong>{project.name}</strong>
                    <small>{project.summary || 'cc2cx AI3D生成工作区'} · {formatDate(project.savedAt)}</small>
                  </div>
                  <div className="project-actions">
                    <button type="button" onClick={() => onLoadProject(project.id)}>{t(language, 'load')}</button>
                    <button type="button" onClick={() => onExportProject(project)}>
                      <Download size={12} />
                    </button>
                    <button type="button" onClick={() => onDeleteProject(project.id)}>
                      <Trash2 size={12} />
                    </button>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
        <div className="history-panel">
          <div className="project-manager-head">
            <div>
              <strong>{t(language, 'generationHistory')}</strong>
              <small>{t(language, 'historyHint')}</small>
            </div>
            <button type="button" className="drawer-secondary" disabled={generationHistory.length === 0} onClick={onClearGenerationHistory}>{t(language, 'clear')}</button>
          </div>
          {latestHistory.length === 0 ? (
            <p className="empty-state">{t(language, 'noRuns')}</p>
          ) : (
            <div className="history-list">
              {latestHistory.map((item) => (
                <button key={item.id} type="button" className={`history-row ${item.status}`} onClick={() => onSelectCell(item.cellId)}>
                  <span>
                    <strong>{item.cellName || item.cellId}</strong>
                    <small>{item.provider} · {item.status} · {formatDuration(item.durationMs)}</small>
                  </span>
                  <small>{formatDate(item.finishedAt || item.startedAt)}</small>
                </button>
              ))}
            </div>
          )}
        </div>
        <p className="drawer-copy">{t(language, 'pinnedPart')}: {savedFavorite}</p>
        <p className="drawer-copy">{t(language, 'sourceLabel')}: {profile.occurs}</p>
      </div>
    )
  }

  return (
    <motion.section className={`workspace-drawer drawer-${String(activePanel).toLowerCase()}`} initial={{ opacity: 0, y: -8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
      <header>
        <div>
          <strong>{t(language, activePanel)}</strong>
          <span>{t(language, `${activePanel}Hint`)}</span>
        </div>
        <button type="button" onClick={onClose} aria-label={t(language, 'closePanel')}>
          <X size={15} />
        </button>
      </header>
      <div className="drawer-meta">
        <span>{cell.name}</span>
        <span>{detail.title}</span>
        <span>{t(language, 'viewerReady')}</span>
      </div>
      {renderContent()}
    </motion.section>
  )
}
