import { useRef } from 'react'
import { createPortal } from 'react-dom'
import { Box, Image, Upload } from 'lucide-react'

import { GENERATION_MODE_OPTIONS } from '../config/appConfig.js'
import { MICROSCOPE_IMAGES } from '../domain/cellData.js'
import { getCell } from '../domain/cellCatalog.js'
import { modeLabel, t } from '../i18n.js'
import { CellThumb } from './CellThumb.jsx'

export function BottomDeck({
  selectedCell,
  selectedMicroscope,
  setSelectedMicroscope,
  uploadedImage,
  generationMode,
  generationModes = GENERATION_MODE_OPTIONS,
  onGenerationModeChange,
  compareCell,
  customCells,
  latestUploadCell,
  onUploadImage,
  onCompare,
  onOpenGenerationCell,
  onNotify,
  sketchPrompt = '',
  pendingSketchUpload = null,
  pendingImageUpload = null,
  onSketchPromptChange,
  onConfirmSketchUpload,
  onCancelSketchUpload,
  onConfirmImageUpload,
  onCancelImageUpload,
  language = 'zh',
}) {
  const imageFileInputRef = useRef(null)
  const modelFileInputRef = useRef(null)
  const selected = getCell(selectedCell, customCells)
  const compareTarget = getCell(compareCell, customCells)
  const sketchMode = generationMode === 'hunyuan-sketch'
  const localMode = generationMode === 'local'
  const primaryModes = generationModes.filter((mode) => ['hunyuan', 'hunyuan-sketch'].includes(mode.id))
  const advancedModes = generationModes.filter((mode) => !['hunyuan', 'hunyuan-sketch', 'cinematic'].includes(mode.id))

  function handleMicroscopeSelect(item) {
    setSelectedMicroscope(item.label)
    onNotify(item.note)
  }

  function handleImageChange(event) {
    const file = event.target.files?.[0]
    if (file) onUploadImage(file)
    event.target.value = ''
  }

  function handleModelChange(event) {
    const file = event.target.files?.[0]
    if (file) onUploadImage(file)
    event.target.value = ''
  }

  function renderGenerationActions(className) {
    return (
      <div className={className}>
        {localMode ? (
          <button type="button" className="generation-action primary" onClick={() => modelFileInputRef.current?.click()}>
            <Upload size={17} />
            {t(language, 'importGlb')}
          </button>
        ) : (
          <button type="button" className="generation-action primary" onClick={() => imageFileInputRef.current?.click()}>
            <Upload size={17} />
            {sketchMode ? t(language, 'addSketch') : t(language, 'uploadImageGenerate')}
          </button>
        )}
        {!localMode && (
          <button type="button" className="generation-action secondary" onClick={() => modelFileInputRef.current?.click()}>
            <Box size={16} />
            {t(language, 'importGlb')}
          </button>
        )}
      </div>
    )
  }

  function renderUploadConfirmation() {
    const pending = pendingSketchUpload || pendingImageUpload
    if (!pending || typeof document === 'undefined') return null

    const isSketch = Boolean(pendingSketchUpload)
    const confirm = isSketch ? onConfirmSketchUpload : onConfirmImageUpload
    const cancel = isSketch ? onCancelSketchUpload : onCancelImageUpload
    const title = isSketch ? '确认这是线稿吗？' : '确认生成 3D？'
    const description = isSketch
      ? `${pending.name} 将按草图模式发送；照片可能生成失败。`
      : `${pending.name} 将发送到云端生成，通常需要 2–8 分钟。`

    return createPortal(
      <div className="upload-confirm-scrim">
        <div className="upload-confirm-dialog" role="dialog" aria-modal="true" aria-label={isSketch ? '确认草图' : '确认生成 3D'}>
          <div className="sketch-confirm-preview">
            <img src={pending.previewUrl} alt={`待确认文件：${pending.name}`} />
          </div>
          <div className="sketch-confirm-copy">
            <strong>{title}</strong>
            <span>{description}</span>
          </div>
          {!isSketch && (
            <div className="upload-confirm-facts">
              <span>普通图片</span>
              <span>预计 2–8 分钟</span>
              <span>会消耗积分</span>
              <span>输出：GLB</span>
            </div>
          )}
          <div className="sketch-confirm-actions">
            <button type="button" className="sketch-cancel" onClick={cancel}>取消</button>
            <button type="button" className="sketch-confirm-primary" onClick={confirm}>确认并生成</button>
          </div>
        </div>
      </div>,
      document.body,
    )
  }

  return (
    <>
    <section className="bottom-deck">
      <div className="panel media-panel">
        <header className="panel-title">
          <span>{t(language, 'assetSource')}</span>
          <small>{latestUploadCell ? 5 : 4}</small>
        </header>
        <div className="generation-mode-row">
          <span>{t(language, 'provider')}</span>
          <div className="generation-mode-pills">
            {primaryModes.map((mode) => {
              const selected = generationMode === mode.id
              return (
              <button
                key={mode.id}
                type="button"
                className={selected ? 'active' : ''}
                aria-pressed={selected}
                onClick={() => {
                  onGenerationModeChange(mode.id)
                  onNotify(mode.id === 'hunyuan-sketch' ? '请上传线稿，并填写提示词。' : `${modeLabel(language, mode.id, mode.label)} ${t(language, 'modeSelected')}`)
                }}
                title={mode.description}
              >
                {modeLabel(language, mode.id, mode.label)}
              </button>
              )
            })}
          </div>
        </div>
        {advancedModes.length > 0 && (
          <details className="advanced-generation-settings" open={advancedModes.some((mode) => mode.id === generationMode)}>
            <summary>高级设置</summary>
            <div className="generation-mode-pills advanced-mode-pills">
              {advancedModes.map((mode) => {
                const selected = generationMode === mode.id
                return (
                  <button
                    key={mode.id}
                    type="button"
                    className={selected ? 'active' : ''}
                    aria-pressed={selected}
                    onClick={() => {
                      onGenerationModeChange(mode.id)
                      onNotify(`${modeLabel(language, mode.id, mode.label)} ${t(language, 'modeSelected')}`)
                    }}
                    title={mode.description}
                  >
                    {modeLabel(language, mode.id, mode.label)}
                  </button>
                )
              })}
            </div>
          </details>
        )}
        {sketchMode && (
          <div className="sketch-mode-tools">
            <div className="sketch-mode-notice" role="note">
              <strong>混元草图模式</strong>
              <span>草图模式才能图+词一起发。上传线稿，不要传照片。提示词用中文写清立体结构。</span>
            </div>
            <label htmlFor="hunyuan-sketch-prompt">将与草图一起发送</label>
            <textarea
              id="hunyuan-sketch-prompt"
              value={sketchPrompt}
              maxLength={1024}
              onChange={(event) => onSketchPromptChange?.(event.target.value)}
              placeholder="一只上皮细胞，光滑表面，细胞核清晰"
              rows={3}
            />
            <div className="sketch-prompt-count">{sketchPrompt.length} / 1024</div>
          </div>
        )}
        <div className="micro-grid">
          {!sketchMode && MICROSCOPE_IMAGES.map((item) => (
            <button
              key={item.label}
              type="button"
              className={selectedMicroscope === item.label ? `micro-card ${item.tone} active` : `micro-card ${item.tone}`}
              onClick={() => handleMicroscopeSelect(item)}
            >
              <span />
              <small>{item.label}</small>
            </button>
          ))}
        </div>
        {renderGenerationActions('generation-actions')}
        <p className="generation-hint">{localMode ? t(language, 'uploadNew') : t(language, 'generationHint')}</p>
        {latestUploadCell && (
          <button
            type="button"
            className="latest-asset-link"
            onClick={() => onOpenGenerationCell(latestUploadCell.id)}
            title={t(language, 'openLatest')}
          >
            {uploadedImage?.url ? <Image size={14} /> : <Box size={14} />}
            <span>{latestUploadCell.name || uploadedImage?.name || t(language, 'latestAsset')}</span>
            <small>{t(language, 'openLatestAsset')}</small>
          </button>
        )}
        <input
          ref={imageFileInputRef}
          className="hidden-file-input"
          type="file"
          accept="image/*"
          onChange={handleImageChange}
        />
        <input
          ref={modelFileInputRef}
          className="hidden-file-input"
          type="file"
          accept=".glb,.gltf,model/gltf-binary,model/gltf+json"
          onChange={handleModelChange}
        />
      </div>

      <div className="panel compare-panel">
        <header className="panel-title">
          <span>{t(language, 'compareModels')}</span>
          <small>2</small>
        </header>
        <button type="button" className="compare-box" onClick={() => onCompare(compareTarget.id)}>
          <CellThumb cell={selected} selected />
          <div>
            <strong>{selected.name.replace(' Cell', '')}</strong>
            <small>{selected.type}</small>
          </div>
          <span className="versus">VS</span>
          <CellThumb cell={compareTarget} />
          <div>
            <strong>{compareTarget.name}</strong>
            <small>{compareTarget.type.replace('Human ', '')}</small>
          </div>
        </button>
      </div>

    </section>
    {typeof document !== 'undefined' && createPortal(
      renderGenerationActions('mobile-generation-actions'),
      document.body,
    )}
    {renderUploadConfirmation()}
    </>
  )
}
