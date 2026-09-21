import { fetch as undiciFetch } from 'undici'

import {
  OPENAI_API_BASE,
  OPENAI_API_KEY,
  OPENAI_VISION_MODEL,
  OUTBOUND_PROXY_AGENT,
  VISION_PROVIDER,
  hasOutboundProxy,
} from '../config.mjs'
import { parseDataUrl, sanitizeFileName } from '../http-utils.mjs'

const CATEGORY_IDS = new Set(['artifact', 'road', 'vessel', 'aircraft', 'product', 'specimen'])
const CATEGORY_LABELS = {
  artifact: 'Museum Artifact',
  road: 'Performance Vehicle',
  vessel: 'Naval Vessel',
  aircraft: 'Aircraft',
  product: 'Product Object',
  specimen: 'Organic Specimen',
}

export function getVisionHealth() {
  return {
    provider: VISION_PROVIDER,
    configured: VISION_PROVIDER === 'openai' && Boolean(OPENAI_API_KEY),
    model: VISION_PROVIDER === 'openai' ? OPENAI_VISION_MODEL : '',
    baseUrl: VISION_PROVIDER === 'openai' ? OPENAI_API_BASE : '',
  }
}

export async function analyzeAssetImage(payload = {}) {
  const image = parseDataUrl(payload.imageDataUrl)
  const fileName = sanitizeFileName(payload.fileName || `asset-reference.${image.ext}`)

  if (VISION_PROVIDER !== 'openai') {
    return unavailableInsight(fileName, `VISION_PROVIDER=${VISION_PROVIDER} is not supported yet.`)
  }

  if (!OPENAI_API_KEY) {
    return unavailableInsight(fileName, 'OPENAI_API_KEY is not configured on the backend.')
  }

  const raw = await openAiVisionRequest(payload.imageDataUrl, fileName)
  const content = raw?.choices?.[0]?.message?.content || ''
  const parsed = extractJsonObject(content)
  return normalizeVisionInsight(parsed, {
    fileName,
    provider: 'openai',
    model: OPENAI_VISION_MODEL,
    raw,
  })
}

export function normalizeVisionInsight(raw = {}, context = {}) {
  const categoryId = normalizeCategoryId(raw.categoryId || raw.category || raw.type)
  const objectName = cleanText(raw.objectName || raw.name || raw.title || context.fileName || 'Uploaded asset', 90)
  const tags = normalizeTags(raw.tags)

  return {
    provider: context.provider || 'openai',
    model: context.model || '',
    configured: true,
    status: 'success',
    objectName,
    categoryId,
    categoryLabel: cleanText(raw.categoryLabel || CATEGORY_LABELS[categoryId], 48),
    description: cleanText(raw.description || raw.summary, 420),
    material: cleanText(raw.material || raw.materials, 220),
    inspectionFocus: cleanText(raw.inspectionFocus || raw.structureFocus || raw.focus, 220),
    presentation: cleanText(raw.presentation || raw.demo || raw.scene, 320),
    generationPrompt: cleanText(raw.generationPrompt || raw.prompt, 520),
    tags,
    confidence: normalizeConfidence(raw.confidence),
    reason: cleanText(raw.reason || raw.rationale, 260),
    analyzedAt: new Date().toISOString(),
  }
}

export function extractJsonObject(content) {
  if (!content || typeof content !== 'string') {
    throw new Error('Vision model did not return text content.')
  }

  try {
    return JSON.parse(content)
  } catch {
    const match = content.match(/\{[\s\S]*\}/)
    if (!match) throw new Error('Vision model did not return a JSON object.')
    return JSON.parse(match[0])
  }
}

async function openAiVisionRequest(imageDataUrl, fileName) {
  let response
  try {
    response = await undiciFetch(`${OPENAI_API_BASE.replace(/\/$/, '')}/chat/completions`, {
      method: 'POST',
      ...(OUTBOUND_PROXY_AGENT ? { dispatcher: OUTBOUND_PROXY_AGENT } : {}),
      headers: {
        Authorization: `Bearer ${OPENAI_API_KEY}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: OPENAI_VISION_MODEL,
        response_format: { type: 'json_object' },
        temperature: 0.2,
        messages: [
          {
            role: 'system',
            content: [
              'You analyze a reference image for a 3D model studio.',
              'Return only a compact JSON object.',
              'Allowed categoryId values: artifact, road, vessel, aircraft, product, specimen.',
              'Choose vessel for aircraft carriers, warships, ships, or submarines, even if the word aircraft appears.',
              'Choose artifact for museum relics, bronze objects, masks, statues, ancient objects, or archaeological items.',
              'Describe what matters for making and presenting the 3D asset, not generic biology unless it is truly biological.',
            ].join(' '),
          },
          {
            role: 'user',
            content: [
              {
                type: 'text',
                text: [
                  `File name: ${fileName}`,
                  'Return JSON with these keys:',
                  'objectName, categoryId, categoryLabel, description, material, inspectionFocus, presentation, generationPrompt, tags, confidence, reason.',
                  'Keep objectName short and human-readable.',
                  'generationPrompt should help an image-to-3D model preserve one integrated object, correct silhouette, materials, and key structure.',
                ].join(' '),
              },
              {
                type: 'image_url',
                image_url: { url: imageDataUrl },
              },
            ],
          },
        ],
      }),
    })
  } catch (error) {
    const wrapped = new Error(`OpenAI vision network request failed: ${error.message}`)
    wrapped.detail = {
      cause: error.cause?.message || error.cause?.code || '',
      proxy: hasOutboundProxy(),
    }
    throw wrapped
  }

  const text = await response.text()
  let data
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    data = { error: { message: text || 'Non-JSON response from OpenAI.' } }
  }

  if (!response.ok || data.error) {
    const error = new Error(data.error?.message || data.message || `OpenAI vision request failed with ${response.status}.`)
    error.status = response.status || 502
    error.detail = sanitizeOpenAiRaw(data)
    throw error
  }

  return sanitizeOpenAiRaw(data)
}

function unavailableInsight(fileName, message) {
  return {
    provider: VISION_PROVIDER,
    model: VISION_PROVIDER === 'openai' ? OPENAI_VISION_MODEL : '',
    configured: false,
    status: 'unavailable',
    objectName: fileName.replace(/\.[^.]+$/, '').replace(/[-_]+/g, ' ').trim() || 'Uploaded asset',
    categoryId: '',
    categoryLabel: '',
    description: '',
    material: '',
    inspectionFocus: '',
    presentation: '',
    generationPrompt: '',
    tags: [],
    confidence: 0,
    reason: message,
    analyzedAt: new Date().toISOString(),
  }
}

function normalizeCategoryId(value) {
  const normalized = String(value || '').trim().toLowerCase().replace(/\s+/g, '-')
  if (CATEGORY_IDS.has(normalized)) return normalized

  if (['car', 'vehicle', 'automobile', 'supercar', 'truck'].includes(normalized)) return 'road'
  if (['ship', 'carrier', 'warship', 'naval', 'submarine'].includes(normalized)) return 'vessel'
  if (['plane', 'airplane', 'fighter', 'fighter-jet', 'jet'].includes(normalized)) return 'aircraft'
  if (['relic', 'museum', 'bronze', 'mask', 'statue'].includes(normalized)) return 'artifact'
  if (['cell', 'biology', 'organic', 'organism'].includes(normalized)) return 'specimen'
  return 'product'
}

function normalizeTags(tags) {
  const rawTags = Array.isArray(tags) ? tags : String(tags || '').split(/[,\n]/)
  return [...new Set(rawTags.map((tag) => cleanText(tag, 28).toLowerCase()).filter(Boolean))].slice(0, 8)
}

function normalizeConfidence(value) {
  const confidence = Number(value)
  if (!Number.isFinite(confidence)) return 0
  return Math.max(0, Math.min(1, confidence > 1 ? confidence / 100 : confidence))
}

function cleanText(value, maxLength) {
  const text = String(value || '').replace(/\s+/g, ' ').trim()
  if (!text) return ''
  return text.length > maxLength ? `${text.slice(0, maxLength - 1).trim()}…` : text
}

function sanitizeOpenAiRaw(raw) {
  if (!raw || typeof raw !== 'object') return raw
  return JSON.parse(JSON.stringify(raw, (key, value) => {
    if (['authorization', 'api_key', 'apiKey'].includes(String(key).toLowerCase())) return '[secret omitted]'
    return value
  }))
}
