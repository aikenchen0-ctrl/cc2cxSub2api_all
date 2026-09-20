import {
  DEFAULT_SETTINGS,
  FAL_MODEL_IDS,
  GENERATION_MODE_IDS,
  GENERATION_PROVIDER_IDS,
  LANGUAGE_IDS,
  SCREENSHOT_SCALE_OPTIONS,
  SETTINGS_STORAGE_VERSION,
  UI_STATE_STORAGE_VERSION,
} from '../config/appConfig.js'
import { MICROSCOPE_IMAGES } from './cellData.js'
import { getCellProfile } from './cellCatalog.js'

export function normalizeSettings(value) {
  const stored = value && typeof value === 'object' ? value : {}
  const next = { ...DEFAULT_SETTINGS, ...stored }
  if (stored.settingsVersion !== SETTINGS_STORAGE_VERSION) {
    next.generationProvider = DEFAULT_SETTINGS.generationProvider
    next.generationMode = DEFAULT_SETTINGS.generationMode
    next.falModelId = FAL_MODEL_IDS.has(stored.falModelId) ? stored.falModelId : DEFAULT_SETTINGS.falModelId
    next.screenshotScale = normalizeScreenshotScale(stored.screenshotScale)
    next.language = stored.settingsVersion >= 6 && LANGUAGE_IDS.has(stored.language)
      ? stored.language
      : DEFAULT_SETTINGS.language
  }

  if (!GENERATION_PROVIDER_IDS.has(next.generationProvider)) {
    next.generationProvider = DEFAULT_SETTINGS.generationProvider
  }

  if (!GENERATION_MODE_IDS.has(next.generationMode)) {
    next.generationMode = DEFAULT_SETTINGS.generationMode
  }

  if (!FAL_MODEL_IDS.has(next.falModelId)) {
    next.falModelId = DEFAULT_SETTINGS.falModelId
  }

  next.screenshotScale = normalizeScreenshotScale(next.screenshotScale)
  if (!LANGUAGE_IDS.has(next.language)) {
    next.language = DEFAULT_SETTINGS.language
  }

  next.settingsVersion = SETTINGS_STORAGE_VERSION
  return next
}

function normalizeScreenshotScale(value) {
  const scale = Number(value)
  return SCREENSHOT_SCALE_OPTIONS.some((option) => option.id === scale) ? scale : DEFAULT_SETTINGS.screenshotScale
}

export function normalizeUiState(value) {
  const stored = value && typeof value === 'object' ? value : {}
  const selectedMicroscope = MICROSCOPE_IMAGES.some((item) => item.label === stored.selectedMicroscope)
    ? stored.selectedMicroscope
    : MICROSCOPE_IMAGES[0].label
  return {
    selectedCell: typeof stored.selectedCell === 'string' ? stored.selectedCell : 'plant',
    selectedOrganelle: typeof stored.selectedOrganelle === 'string' ? stored.selectedOrganelle : 'nucleus',
    selectedMicroscope,
    compareCell: typeof stored.compareCell === 'string' ? stored.compareCell : getCellProfile('plant').compareTarget,
    crossSection: typeof stored.crossSection === 'boolean' ? stored.crossSection : true,
    favoriteKey: typeof stored.favoriteKey === 'string' ? stored.favoriteKey : '',
    uiStateVersion: UI_STATE_STORAGE_VERSION,
  }
}
