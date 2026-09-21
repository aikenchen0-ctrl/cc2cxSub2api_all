import { useEffect, useMemo, useState } from 'react'
import { Box, Camera, CircleDot, Eye, Gauge, Layers3, Move3D, RotateCcw, Upload } from 'lucide-react'
import { getCell, getOrganelleDetail } from '../domain/cellCatalog.js'
import { getGeneratedModelUrl } from '../domain/modelUrl.js'
import { modeLabel, t } from '../i18n.js'
import { downloadCanvasImage } from '../lib/downloads.js'
import { getSceneProfile } from '../lib/assetIntelligence.js'
import { downloadLayeredPngSnapshot } from '../lib/imagePipeline.js'
import { formatBytes, formatDuration, formatNumber, getModelQuality, inspectModelUrl } from '../lib/modelQuality.js'
import { inferMotionProfile } from '../lib/motionProfiles.js'
import { canUseWebGL } from '../lib/webgl.js'
import { getProviderLabel } from '../services/modelApi.js'
import { CellFallback, CellScene, CinematicLayerVisual, ViewerErrorBoundary } from '../viewer/CellViewer.jsx'

function ViewerControls({ crossSection, setCrossSection, viewMode, setViewMode, supportsPartControls, onModeChange, language = 'zh' }) {
  const modes = [
    { id: 'solid', icon: Box, label: t(language, 'solidView'), status: t(language, 'solid') },
    { id: 'layers', icon: Layers3, label: t(language, 'xrayView'), status: t(language, 'xray') },
    { id: 'focus', icon: CircleDot, label: t(language, 'inspectView'), status: t(language, 'inspect') },
  ]

  return (
    <div className="viewer-controls">
      <span>{t(language, 'viewMode')}</span>
      <div className="mode-buttons">
        {modes.map((mode) => {
          const Icon = mode.icon
          return (
            <button
              key={mode.id}
              type="button"
              className={viewMode === mode.id ? 'active' : ''}
              onClick={() => {
                setViewMode(mode.id)
                onModeChange?.(mode.status)
              }}
              title={mode.label}
              aria-label={mode.label}
            >
              <Icon size={17} />
            </button>
          )
        })}
      </div>
      {supportsPartControls && (
        <label className="toggle-row" title={t(language, 'cutIntoStarter')}>
          <span>{t(language, 'crossSection')}</span>
          <input
            type="checkbox"
            checked={crossSection}
            onChange={(event) => setCrossSection(event.target.checked)}
          />
          <i />
        </label>
      )}
    </div>
  )
}

export function CenterStage({
  selectedCell,
  selectedOrganelle,
  setSelectedOrganelle,
  crossSection,
  setCrossSection,
  renderQuality,
  screenshotScale = 1,
  customCells,
  generationHistory = [],
  demoMode = false,
  onNotify,
  onExport,
  exportAvailable,
  exportReason,
  onExporterReady,
  onRetryGeneration,
  onOpenInspector,
  language = 'zh',
}) {
  const [viewMode, setViewMode] = useState('solid')
  const [autoRotate, setAutoRotate] = useState(false)
  const [isIsolated, setIsIsolated] = useState(false)
  const [hideOthers, setHideOthers] = useState(false)
  const [proofMode, setProofMode] = useState(false)
  const [resetNonce, setResetNonce] = useState(0)
  const [capturePulse, setCapturePulse] = useState(false)
  const [viewerError, setViewerError] = useState(null)
  const [modelMetrics, setModelMetrics] = useState(null)
  const cell = getCell(selectedCell, customCells)
  const modelCellId = cell.custom ? cell.template : selectedCell
  const referenceImageUrl = cell.custom ? cell.imageUrl || cell.thumbnailUrl || '' : ''
  const generatedModelUrl = getGeneratedModelUrl(cell)
  const generation = cell.custom ? cell.generation : null
  const generationProviderLabel = getProviderLabel(generation?.provider)
  const sketchGeneration = generation?.requestedProvider === 'hunyuan-sketch'
  const generationModeLabel = sketchGeneration ? modeLabel(language, 'hunyuan-sketch', '混元 · 草图+提示') : generationProviderLabel
  const generationFailureTitle = generation?.requestedProvider === 'auto' ? t(language, 'generationFailed') : `${generationModeLabel} ${t(language, 'generationFailedShort')}`
  const isCinematicCell = cell.custom && generation?.provider === 'cinematic'
  const supportsPartControls = !cell.custom && !generatedModelUrl && !isCinematicCell
  const effectiveAutoRotate = autoRotate || demoMode
  const effectiveHideOthers = demoMode || !supportsPartControls ? false : hideOthers || viewMode === 'focus'
  const effectiveIsolated = demoMode ? false : isIsolated || viewMode === 'focus'
  const effectiveProofMode = demoMode ? false : proofMode
  const effectiveViewMode = demoMode ? 'solid' : viewMode
  const effectiveCrossSection = supportsPartControls && (crossSection || effectiveViewMode === 'layers')
  const effectiveStatusMode = effectiveViewMode === 'layers' ? t(language, 'xray') : effectiveViewMode === 'focus' ? t(language, 'inspect') : t(language, 'solid')
  const detail = getOrganelleDetail(selectedCell, selectedOrganelle, customCells)
  const webglAvailable = canUseWebGL()
  const generationPending = cell.custom && !generatedModelUrl && generation?.status && !['failed', 'local'].includes(generation.status)
  const generationFailed = cell.custom && !generatedModelUrl && generation?.status === 'failed'
  const orbitLabel = effectiveAutoRotate ? t(language, 'autoOrbit') : t(language, 'manualOrbit')
  const rotateLabel = effectiveAutoRotate || effectiveProofMode ? t(language, 'autoRotate') : t(language, 'manualOrbit')
  const stageStatusText = isCinematicCell
    ? `${t(language, 'jsRelief')} · ${orbitLabel} · ${effectiveStatusMode}`
    : `${generatedModelUrl ? `${generationModeLabel} ${t(language, 'glbLoaded')}` : generationFailed ? `${generationModeLabel} ${t(language, 'failedSourceShown')}` : referenceImageUrl ? `${generationModeLabel} ${generation?.status || t(language, 'pending')}` : webglAvailable ? t(language, 'webglLive') : t(language, 'fallbackImage')} · ${rotateLabel} · ${effectiveStatusMode}`
  const referenceLabel = isCinematicCell
    ? t(language, 'sourceForJsDepth')
    : generatedModelUrl
    ? `${t(language, 'sourceImageUsedFor')} ${generationModeLabel} ${t(language, 'gen3d')}`
    : `${t(language, 'sourceImageFor')} ${generationModeLabel} ${t(language, 'gen3d')}`
  const viewerResetKey = `${selectedCell}-${generatedModelUrl}-${generation?.provider || 'built-in'}-${resetNonce}`
  const activeViewerError = viewerError?.key === viewerResetKey ? viewerError.message : ''
  const activeModelMetrics = modelMetrics?.url === generatedModelUrl ? modelMetrics.data : null
  const quality = useMemo(() => getModelQuality(cell, activeModelMetrics, generationHistory), [activeModelMetrics, cell, generationHistory])
  const motionProfile = useMemo(() => inferMotionProfile(cell), [cell])
  const sceneProfile = useMemo(() => getSceneProfile(cell), [cell])
  const viewerFallback = (
    <CellFallback
      selectedCell={selectedCell}
      modelCellId={modelCellId}
      referenceImageUrl={referenceImageUrl}
      selectedOrganelle={selectedOrganelle}
      onSelectOrganelle={setSelectedOrganelle}
    />
  )

  function handleRotate() {
    const next = !autoRotate
    setAutoRotate(next)
    onNotify(next ? t(language, 'autoRotateOn') : t(language, 'autoRotateOff'))
  }

  function handleIsolate() {
    if (!supportsPartControls) return
    const next = !isIsolated
    setIsIsolated(next)
    if (next) setViewMode('focus')
    onNotify(next ? `${detail.title} ${t(language, 'inspect')}` : t(language, 'focusModeOff'))
  }

  function handleHideOthers() {
    if (!supportsPartControls) return
    const next = !hideOthers
    setHideOthers(next)
    onNotify(next ? `${detail.title}` : t(language, 'allStructures'))
  }

  function handleResetView() {
    setAutoRotate(false)
    setIsIsolated(false)
    setHideOthers(false)
    setProofMode(false)
    setViewMode('solid')
    setResetNonce((value) => value + 1)
    onNotify(t(language, 'viewReset'))
  }

  function handleProofMode() {
    const next = !proofMode
    setProofMode(next)
    if (next) {
      setViewMode('focus')
      setHideOthers(false)
      setAutoRotate(true)
      onOpenInspector?.()
    }
    onNotify(next ? t(language, 'inspectOn') : t(language, 'inspectOff'))
  }

  function handleViewModeChange(modeLabel) {
    onNotify(`${modeLabel} ${t(language, 'viewEnabled')}`)
  }

  async function handleScreenshot() {
    const ok = isCinematicCell && referenceImageUrl
      ? (webglAvailable ? downloadCanvasImage(`${selectedCell}-${selectedOrganelle}.png`, screenshotScale) : await downloadLayeredPngSnapshot(referenceImageUrl, `${selectedCell}-${selectedOrganelle}.png`))
      : downloadCanvasImage(`${selectedCell}-${selectedOrganelle}.png`, screenshotScale)
    setCapturePulse(true)
    window.setTimeout(() => setCapturePulse(false), 280)
    onNotify(ok ? t(language, 'screenshotDownloaded') : t(language, 'screenshotUnavailable'))
  }

  function handleViewerError(error) {
    console.error(error)
    const message = error instanceof Error ? error.message : t(language, 'previewUnavailable')
    setViewerError({ key: viewerResetKey, message })
    onExporterReady?.(null)
    onNotify(t(language, 'previewFallback'))
  }

  useEffect(() => {
    if (isCinematicCell) onExporterReady?.(null)
  }, [isCinematicCell, onExporterReady])

  useEffect(() => {
    let cancelled = false

    if (!generatedModelUrl) return undefined

    inspectModelUrl(generatedModelUrl)
      .then((metrics) => {
        if (!cancelled) setModelMetrics({ url: generatedModelUrl, data: metrics })
      })
      .catch((error) => {
        if (!cancelled) {
          setModelMetrics({ url: generatedModelUrl, data: { error: error instanceof Error ? error.message : 'Model metrics unavailable.' } })
        }
      })

    return () => {
      cancelled = true
    }
  }, [generatedModelUrl])

  return (
    <section className={`stage-panel motion-${motionProfile.id} scene-${sceneProfile.id}`}>
      <div className="stage-title">
        <div>
          <h1>{cell.name}</h1>
          <p>{cell.type}</p>
        </div>
      </div>
      <ViewerControls
        crossSection={crossSection}
        setCrossSection={setCrossSection}
        viewMode={viewMode}
        setViewMode={setViewMode}
        supportsPartControls={supportsPartControls}
        onModeChange={handleViewModeChange}
        language={language}
      />
      {demoMode && <PresentationMotionField profile={sceneProfile.id} />}
      {demoMode && <DemoShowcaseOverlay cell={cell} quality={quality} referenceImageUrl={referenceImageUrl} motionProfile={motionProfile} sceneProfile={sceneProfile} language={language} />}
      {!demoMode && <ModelQualityCard quality={quality} language={language} />}
      <div className={`cell-viewer ${effectiveViewMode} ${effectiveIsolated ? 'is-isolated' : ''} ${generatedModelUrl ? 'has-glb' : ''} ${webglAvailable ? 'webgl-ready' : ''} ${isCinematicCell ? 'cinematic-viewer' : ''}`}>
        <ViewerErrorBoundary resetKey={viewerResetKey} onError={handleViewerError} fallback={viewerFallback}>
          {isCinematicCell ? (
            <CinematicLayerVisual
              imageUrl={referenceImageUrl}
              selectedOrganelle={selectedOrganelle}
              onSelectOrganelle={setSelectedOrganelle}
              autoRotate={effectiveAutoRotate || effectiveProofMode}
              presentationMode={demoMode}
              motionProfile={sceneProfile.id}
              viewMode={effectiveViewMode}
            />
          ) : (
            <>
              <CellFallback selectedCell={selectedCell} modelCellId={modelCellId} referenceImageUrl={referenceImageUrl} selectedOrganelle={selectedOrganelle} onSelectOrganelle={setSelectedOrganelle} />
              {!generationFailed && (
                <CellScene
                  key={`${selectedCell}-${resetNonce}`}
                  selectedCell={selectedCell}
                  modelCellId={modelCellId}
                  referenceImageUrl={referenceImageUrl}
                  generatedModelUrl={generatedModelUrl}
                  selectedOrganelle={selectedOrganelle}
                  crossSection={effectiveCrossSection}
                  autoRotate={effectiveAutoRotate}
                  hideOthers={effectiveHideOthers}
                  proofMode={effectiveProofMode}
                  viewMode={effectiveViewMode}
                  renderQuality={renderQuality}
                  presentationMode={demoMode}
                  motionProfile={sceneProfile.id}
                  onSelectOrganelle={setSelectedOrganelle}
                  onExporterReady={onExporterReady}
                />
              )}
            </>
          )}
        </ViewerErrorBoundary>
      </div>
      {referenceImageUrl && (
        <div className="custom-reference-layer">
          <img src={referenceImageUrl} alt={`${cell.name} uploaded reference`} />
          <span>{referenceLabel}</span>
        </div>
      )}
      {generationPending && (
        <div className="generation-overlay">
          <strong>{sketchGeneration ? '正在用混元草图模式生成' : generation.status === 'uploading' ? `${t(language, 'uploadingTo')} ${generationProviderLabel}` : `${t(language, 'generatingWith')} ${generationProviderLabel}`}</strong>
          <span>{sketchGeneration ? '正在用混元草图模式生成，请不要刷新。' : generation.message || t(language, 'waitingGlb')}</span>
          <div className="generation-meter">
            <i />
          </div>
        </div>
      )}
      {generationFailed && (
        <div className="generation-overlay failed">
          <strong>{generationFailureTitle}</strong>
          <span>{generation.message || t(language, 'generationFailedMsg')}</span>
          <button type="button" onClick={() => onRetryGeneration?.(cell.id)}>{t(language, 'retryGeneration')}</button>
        </div>
      )}
      {activeViewerError && !generationFailed && (
        <div className="generation-overlay failed">
          <strong>{t(language, 'previewUnavailable')}</strong>
          <span>{generatedModelUrl ? t(language, 'glbLoadFailed') : activeViewerError}</span>
          {generatedModelUrl ? (
            <button type="button" onClick={handleResetView}>{t(language, 'reloadPreview')}</button>
          ) : (
            cell.custom && !cell.reference && cell.imageUrl && <button type="button" onClick={() => onRetryGeneration?.(cell.id)}>{t(language, 'retryGeneration')}</button>
          )}
        </div>
      )}
      <div className="stage-status">
        {stageStatusText}
      </div>
      {capturePulse && <div className="capture-pulse" />}
      <div className={`stage-toolbar ${supportsPartControls ? 'with-structure' : 'compact-tools'}`}>
        <button type="button" className={autoRotate ? 'active' : ''} onClick={handleRotate} aria-pressed={autoRotate}>
          <Move3D size={14} />
          {t(language, 'rotate')}
        </button>
        {supportsPartControls && (
          <button
            type="button"
            className={isIsolated ? 'active' : ''}
            onClick={handleIsolate}
            aria-pressed={isIsolated}
            title={t(language, 'focusStarterPart')}
          >
            <Eye size={14} />
            {t(language, 'focusPart')}
          </button>
        )}
        {supportsPartControls && (
          <button
            type="button"
            className={hideOthers ? 'active' : ''}
            onClick={handleHideOthers}
            aria-pressed={hideOthers}
            title={t(language, 'hideStarterParts')}
          >
            <Layers3 size={14} />
            {t(language, 'hideParts')}
          </button>
        )}
        <button type="button" onClick={handleResetView}>
          <RotateCcw size={14} />
          {t(language, 'resetView')}
        </button>
        <button type="button" className={proofMode ? 'active proof-active' : ''} onClick={handleProofMode} aria-pressed={proofMode}>
          <Box size={14} />
          {t(language, 'inspect')}
        </button>
        <span />
        <button type="button" onClick={handleScreenshot}>
          <Camera size={14} />
          {t(language, 'screenshot')}
        </button>
        <button type="button" onClick={onExport} disabled={!exportAvailable} title={exportReason}>
          <Upload size={14} />
          {t(language, 'export3d')}
        </button>
      </div>
    </section>
  )
}

function PresentationMotionField({ profile }) {
  if (!['road', 'aircraft', 'vessel', 'artifact', 'product', 'specimen'].includes(profile)) return null

  return (
    <div className={`presentation-motion-field ${profile}`} aria-hidden="true">
      <span />
      <span />
      <span />
      <span />
      <span />
      <span />
    </div>
  )
}

function ModelQualityCard({ quality, language = 'zh' }) {
  return (
    <aside className="model-quality-card" aria-label={t(language, 'qualityScore')}>
      <div className="quality-score">
        <Gauge size={16} />
        <strong>{quality.score}</strong>
        <span>{quality.verdict}</span>
      </div>
      <div className="quality-stats">
        <span><strong>{quality.hasGlb ? t(language, 'yes') : t(language, 'no')}</strong><small>GLB</small></span>
        <span><strong>{quality.loadingMetrics ? '...' : formatBytes(quality.fileBytes)}</strong><small>{t(language, 'file')}</small></span>
        <span><strong>{quality.loadingMetrics ? '...' : formatNumber(quality.triangleCount)}</strong><small>{t(language, 'tris')}</small></span>
        <span><strong>{quality.loadingMetrics ? '...' : quality.textureCount}</strong><small>{t(language, 'textures')}</small></span>
      </div>
    </aside>
  )
}

function DemoShowcaseOverlay({ cell, quality, referenceImageUrl, motionProfile, sceneProfile, language = 'zh' }) {
  return (
    <div className="demo-showcase-overlay">
      <div className="demo-showcase-title">
        <span>{t(language, 'studioTitle')}</span>
        <strong>{cell.name}</strong>
        <small>{quality.providerLabel} · {quality.hasGlb ? t(language, 'glbAsset') : quality.status} · {quality.verdict} · {motionProfile.label}</small>
        <p>{sceneProfile.summary}</p>
        <div className="demo-scene-badges">
          {sceneProfile.badges.map((badge) => (
            <em key={badge}>{badge}</em>
          ))}
        </div>
      </div>
      <div className="demo-metric-strip">
        <span><strong>{quality.score}</strong><small>{t(language, 'score')}</small></span>
        <span><strong>{formatBytes(quality.fileBytes)}</strong><small>{t(language, 'file')}</small></span>
        <span><strong>{formatNumber(quality.triangleCount)}</strong><small>{t(language, 'triangles')}</small></span>
        <span><strong>{quality.textureCount}</strong><small>{t(language, 'textures')}</small></span>
        <span><strong>{formatDuration(quality.durationMs)}</strong><small>{t(language, 'time')}</small></span>
      </div>
      {referenceImageUrl && (
        <div className="demo-source-thumb">
          <img src={referenceImageUrl} alt={`${cell.name} source reference`} />
          <span>{t(language, 'source')}</span>
        </div>
      )}
    </div>
  )
}
