import * as THREE from 'three'
import { downloadBlob } from './downloads.js'
import { clamp, seeded } from './math.js'
import {
  COMPACT_PERSISTED_IMAGE_EDGE,
  MAX_PERSISTED_IMAGE_CHARS,
  MAX_PERSISTED_IMAGE_EDGE,
} from '../config/appConfig.js'

function fileToDataUrl(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result)
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function loadImageFromUrl(url) {
  return new Promise((resolve, reject) => {
    const image = new window.Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('Image could not be decoded.'))
    image.src = url
  })
}

function getCanvasDataUrl(canvas) {
  const webp = canvas.toDataURL('image/webp', 0.9)
  if (webp.startsWith('data:image/webp')) return webp
  return canvas.toDataURL('image/png')
}

function getCanvasPngDataUrl(canvas) {
  return canvas.toDataURL('image/png')
}

function resampleCanvas(sourceCanvas, maxEdge) {
  const scale = Math.min(1, maxEdge / Math.max(sourceCanvas.width, sourceCanvas.height))
  if (scale >= 1) return sourceCanvas

  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(sourceCanvas.width * scale))
  canvas.height = Math.max(1, Math.round(sourceCanvas.height * scale))
  const context = canvas.getContext('2d')
  context.imageSmoothingEnabled = true
  context.imageSmoothingQuality = 'high'
  context.drawImage(sourceCanvas, 0, 0, canvas.width, canvas.height)
  return canvas
}

function trimTransparentCanvas(sourceCanvas, padding = 34) {
  const context = sourceCanvas.getContext('2d')
  const imageData = context.getImageData(0, 0, sourceCanvas.width, sourceCanvas.height)
  const { data, width, height } = imageData
  let minX = width
  let minY = height
  let maxX = -1
  let maxY = -1

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const alpha = data[(y * width + x) * 4 + 3]
      if (alpha < 10) continue
      minX = Math.min(minX, x)
      minY = Math.min(minY, y)
      maxX = Math.max(maxX, x)
      maxY = Math.max(maxY, y)
    }
  }

  if (maxX < minX || maxY < minY) return sourceCanvas

  const cropX = Math.max(0, minX - padding)
  const cropY = Math.max(0, minY - padding)
  const cropW = Math.min(width - cropX, maxX - minX + padding * 2)
  const cropH = Math.min(height - cropY, maxY - minY + padding * 2)
  const cropRatio = (cropW * cropH) / (width * height)
  if (cropRatio > 0.94) return sourceCanvas

  const canvas = document.createElement('canvas')
  canvas.width = cropW
  canvas.height = cropH
  canvas.getContext('2d').drawImage(sourceCanvas, cropX, cropY, cropW, cropH, 0, 0, cropW, cropH)
  return canvas
}

function removeLightBackground(canvas) {
  const context = canvas.getContext('2d')
  const imageData = context.getImageData(0, 0, canvas.width, canvas.height)
  const { data, width, height } = imageData
  const sampleStep = Math.max(1, Math.floor(Math.min(width, height) / 90))
  let edgeSamples = 0
  let lightEdgeSamples = 0

  function isLightNeutral(index) {
    const r = data[index]
    const g = data[index + 1]
    const b = data[index + 2]
    const brightness = (r + g + b) / 3
    const chroma = Math.max(r, g, b) - Math.min(r, g, b)
    return brightness > 232 && chroma < 42
  }

  for (let x = 0; x < width; x += sampleStep) {
    edgeSamples += 2
    if (isLightNeutral(x * 4)) lightEdgeSamples += 1
    if (isLightNeutral(((height - 1) * width + x) * 4)) lightEdgeSamples += 1
  }

  for (let y = 0; y < height; y += sampleStep) {
    edgeSamples += 2
    if (isLightNeutral((y * width) * 4)) lightEdgeSamples += 1
    if (isLightNeutral((y * width + width - 1) * 4)) lightEdgeSamples += 1
  }

  const shouldRemove = edgeSamples > 0 && lightEdgeSamples / edgeSamples > 0.42
  if (!shouldRemove) return canvas

  for (let index = 0; index < data.length; index += 4) {
    const r = data[index]
    const g = data[index + 1]
    const b = data[index + 2]
    const brightness = (r + g + b) / 3
    const chroma = Math.max(r, g, b) - Math.min(r, g, b)

    if (brightness > 242 && chroma < 36) {
      data[index + 3] = 0
    } else if (brightness > 224 && chroma < 46) {
      const keep = Math.max(0, Math.min(1, (242 - brightness) / 18 + chroma / 92))
      data[index + 3] = Math.round(data[index + 3] * keep)
    }
  }

  context.putImageData(imageData, 0, 0)
  return trimTransparentCanvas(canvas)
}

async function buildPersistentImageDataUrl(sourceUrl, maxEdge = MAX_PERSISTED_IMAGE_EDGE) {
  const image = await loadImageFromUrl(sourceUrl)
  const scale = Math.min(1, maxEdge / Math.max(image.naturalWidth || image.width, image.naturalHeight || image.height))
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round((image.naturalWidth || image.width) * scale))
  canvas.height = Math.max(1, Math.round((image.naturalHeight || image.height) * scale))

  const context = canvas.getContext('2d')
  context.imageSmoothingEnabled = true
  context.imageSmoothingQuality = 'high'
  context.drawImage(image, 0, 0, canvas.width, canvas.height)

  const cutoutCanvas = removeLightBackground(canvas)
  const dataUrl = getCanvasDataUrl(cutoutCanvas)
  if (dataUrl.length <= MAX_PERSISTED_IMAGE_CHARS || maxEdge <= COMPACT_PERSISTED_IMAGE_EDGE) return dataUrl

  return getCanvasDataUrl(resampleCanvas(cutoutCanvas, COMPACT_PERSISTED_IMAGE_EDGE))
}

export async function prepareImageForUpload(file) {
  const sourceUrl = await fileToDataUrl(file)
  if (typeof sourceUrl !== 'string' || !file.type.startsWith('image/')) {
    return { displayUrl: sourceUrl, generationUrl: sourceUrl }
  }

  try {
    return {
      displayUrl: await buildPersistentImageDataUrl(sourceUrl),
      generationUrl: sourceUrl,
    }
  } catch (error) {
    console.warn(error)
    return { displayUrl: sourceUrl, generationUrl: sourceUrl }
  }
}

export async function createImageThumbnailDataUrl(sourceUrl, maxEdge = 160) {
  if (!sourceUrl) return ''

  try {
    return await buildPersistentImageDataUrl(sourceUrl, maxEdge)
  } catch (error) {
    console.warn(error)
    return ''
  }
}

function createTransparentCanvas(width, height) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}

async function createCutoutCanvasFromUrl(sourceUrl, maxEdge = COMPACT_PERSISTED_IMAGE_EDGE) {
  const image = await loadImageFromUrl(sourceUrl)
  const sourceWidth = image.naturalWidth || image.width
  const sourceHeight = image.naturalHeight || image.height
  const scale = Math.min(1, maxEdge / Math.max(sourceWidth, sourceHeight))
  const canvas = createTransparentCanvas(Math.max(1, Math.round(sourceWidth * scale)), Math.max(1, Math.round(sourceHeight * scale)))
  const context = canvas.getContext('2d', { willReadFrequently: true })
  context.imageSmoothingEnabled = true
  context.imageSmoothingQuality = 'high'
  context.drawImage(image, 0, 0, canvas.width, canvas.height)
  return removeLightBackground(canvas)
}

function createDerivedPngLayer(sourceCanvas, derivePixel) {
  const inputContext = sourceCanvas.getContext('2d', { willReadFrequently: true })
  const source = inputContext.getImageData(0, 0, sourceCanvas.width, sourceCanvas.height)
  const outputCanvas = createTransparentCanvas(sourceCanvas.width, sourceCanvas.height)
  const outputContext = outputCanvas.getContext('2d')
  const output = outputContext.createImageData(sourceCanvas.width, sourceCanvas.height)
  const { data } = source
  const target = output.data

  for (let y = 0; y < sourceCanvas.height; y += 1) {
    for (let x = 0; x < sourceCanvas.width; x += 1) {
      const index = (y * sourceCanvas.width + x) * 4
      const alpha = data[index + 3]
      if (alpha < 4) continue
      const pixel = derivePixel(data[index], data[index + 1], data[index + 2], alpha, x, y, sourceCanvas.width, sourceCanvas.height)
      if (!pixel) continue
      target[index] = pixel[0]
      target[index + 1] = pixel[1]
      target[index + 2] = pixel[2]
      target[index + 3] = pixel[3]
    }
  }

  outputContext.putImageData(output, 0, 0)
  return getCanvasPngDataUrl(outputCanvas)
}

function createRimPngLayer(sourceCanvas) {
  const inputContext = sourceCanvas.getContext('2d', { willReadFrequently: true })
  const source = inputContext.getImageData(0, 0, sourceCanvas.width, sourceCanvas.height)
  const outputCanvas = createTransparentCanvas(sourceCanvas.width, sourceCanvas.height)
  const outputContext = outputCanvas.getContext('2d')
  const output = outputContext.createImageData(sourceCanvas.width, sourceCanvas.height)
  const { data } = source
  const target = output.data
  const { width, height } = sourceCanvas

  function alphaAt(x, y) {
    if (x < 0 || x >= width || y < 0 || y >= height) return 0
    return data[(y * width + x) * 4 + 3]
  }

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const index = (y * width + x) * 4
      const alpha = data[index + 3]
      if (alpha < 18) continue
      const edgeStrength = Math.max(0, alpha - Math.min(alphaAt(x - 2, y), alphaAt(x + 2, y), alphaAt(x, y - 2), alphaAt(x, y + 2)))
      if (edgeStrength < 18) continue
      target[index] = 120
      target[index + 1] = 176
      target[index + 2] = 210
      target[index + 3] = Math.min(170, edgeStrength * 1.8)
    }
  }

  outputContext.putImageData(output, 0, 0)
  return getCanvasPngDataUrl(outputCanvas)
}

function createHighlightPngLayer(sourceCanvas) {
  const canvas = createTransparentCanvas(sourceCanvas.width, sourceCanvas.height)
  const context = canvas.getContext('2d')
  const main = context.createRadialGradient(sourceCanvas.width * 0.34, sourceCanvas.height * 0.24, 0, sourceCanvas.width * 0.34, sourceCanvas.height * 0.24, sourceCanvas.width * 0.34)
  main.addColorStop(0, 'rgba(255,255,255,0.62)')
  main.addColorStop(0.34, 'rgba(255,255,255,0.16)')
  main.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = main
  context.fillRect(0, 0, canvas.width, canvas.height)

  const secondary = context.createRadialGradient(sourceCanvas.width * 0.68, sourceCanvas.height * 0.66, 0, sourceCanvas.width * 0.68, sourceCanvas.height * 0.66, sourceCanvas.width * 0.28)
  secondary.addColorStop(0, 'rgba(122,190,214,0.24)')
  secondary.addColorStop(1, 'rgba(122,190,214,0)')
  context.fillStyle = secondary
  context.fillRect(0, 0, canvas.width, canvas.height)

  context.globalCompositeOperation = 'destination-in'
  context.drawImage(sourceCanvas, 0, 0)
  return getCanvasPngDataUrl(canvas)
}

function createParticlePngLayer(sourceCanvas) {
  const canvas = createTransparentCanvas(sourceCanvas.width, sourceCanvas.height)
  const context = canvas.getContext('2d')
  const colors = ['rgba(132,80,184,0.72)', 'rgba(223,112,70,0.62)', 'rgba(108,164,198,0.66)', 'rgba(125,176,92,0.56)']

  for (let index = 0; index < 34; index += 1) {
    const x = sourceCanvas.width * (0.16 + seeded(index + 800) * 0.68)
    const y = sourceCanvas.height * (0.14 + seeded(index + 860) * 0.72)
    const radius = 2.4 + seeded(index + 920) * 7.5
    const gradient = context.createRadialGradient(x - radius * 0.28, y - radius * 0.32, 0, x, y, radius)
    gradient.addColorStop(0, 'rgba(255,255,255,0.82)')
    gradient.addColorStop(0.38, colors[index % colors.length])
    gradient.addColorStop(1, 'rgba(255,255,255,0)')
    context.fillStyle = gradient
    context.beginPath()
    context.arc(x, y, radius, 0, Math.PI * 2)
    context.fill()
  }

  return getCanvasPngDataUrl(canvas)
}

export function createImageReliefGeometry(image) {
  const sourceWidth = Math.max(1, image?.naturalWidth || image?.width || 1)
  const sourceHeight = Math.max(1, image?.naturalHeight || image?.height || 1)
  const aspect = sourceWidth / sourceHeight
  const specimenWidth = aspect >= 1 ? 3.9 : 3.9 * aspect
  const specimenHeight = aspect >= 1 ? 3.9 / aspect : 3.9
  const sampleScale = Math.min(1, 190 / Math.max(sourceWidth, sourceHeight))
  const sampleWidth = Math.max(24, Math.round(sourceWidth * sampleScale))
  const sampleHeight = Math.max(24, Math.round(sourceHeight * sampleScale))
  const canvas = createTransparentCanvas(sampleWidth, sampleHeight)
  const context = canvas.getContext('2d', { willReadFrequently: true })
  context.imageSmoothingEnabled = true
  context.imageSmoothingQuality = 'high'
  context.drawImage(image, 0, 0, sampleWidth, sampleHeight)

  const { data } = context.getImageData(0, 0, sampleWidth, sampleHeight)
  const segmentsX = Math.max(44, Math.min(96, Math.round(sampleWidth / 3.1)))
  const segmentsY = Math.max(44, Math.min(96, Math.round(sampleHeight / 3.1)))
  const geometry = new THREE.PlaneGeometry(specimenWidth, specimenHeight, segmentsX, segmentsY)
  const slabGeometry = new THREE.PlaneGeometry(specimenWidth, specimenHeight, 1, 1)
  const positions = geometry.attributes.position
  const uvs = geometry.attributes.uv

  function sampleAlpha(x, y) {
    const px = Math.max(0, Math.min(sampleWidth - 1, x))
    const py = Math.max(0, Math.min(sampleHeight - 1, y))
    return data[(py * sampleWidth + px) * 4 + 3] / 255
  }

  for (let index = 0; index < positions.count; index += 1) {
    const u = uvs.getX(index)
    const v = uvs.getY(index)
    const px = Math.max(0, Math.min(sampleWidth - 1, Math.round(u * (sampleWidth - 1))))
    const py = Math.max(0, Math.min(sampleHeight - 1, Math.round((1 - v) * (sampleHeight - 1))))
    const dataIndex = (py * sampleWidth + px) * 4
    const r = data[dataIndex]
    const g = data[dataIndex + 1]
    const b = data[dataIndex + 2]
    const rawAlpha = data[dataIndex + 3] / 255
    const alpha = rawAlpha < 0.16 ? 0 : clamp((rawAlpha - 0.16) / 0.84)
    const brightness = (r + g + b) / 765
    const saturation = (Math.max(r, g, b) - Math.min(r, g, b)) / 255
    const radial = clamp(1 - Math.hypot((u - 0.5) / 0.57, (v - 0.52) / 0.55))
    const neighborAlpha = Math.min(
      sampleAlpha(px - 2, py),
      sampleAlpha(px + 2, py),
      sampleAlpha(px, py - 2),
      sampleAlpha(px, py + 2),
    )
    const contour = clamp((alpha - neighborAlpha) * 2.4)
    const cellularNoise = Math.sin(u * 24 + v * 13) * 0.018 + Math.sin(u * 47 - v * 29) * 0.012
    const depth = alpha <= 0
      ? -0.16
      : alpha * (0.1 + radial * 0.58 + saturation * 0.22 + (1 - Math.abs(brightness - 0.58)) * 0.1 + cellularNoise) + contour * 0.2
    positions.setZ(index, depth)
  }

  geometry.computeVertexNormals()

  return {
    aspect,
    geometry,
    slabGeometry,
  }
}

export async function buildLayeredPngVisual(sourceUrl) {
  const cutoutCanvas = await createCutoutCanvasFromUrl(sourceUrl)
  const aspect = cutoutCanvas.width / cutoutCanvas.height
  const bodyUrl = getCanvasPngDataUrl(cutoutCanvas)
  const shadowUrl = createDerivedPngLayer(cutoutCanvas, (r, g, b, a) => [42, 55, 62, Math.round(a * 0.34)])
  const depthUrl = createDerivedPngLayer(cutoutCanvas, (r, g, b, a) => [
    Math.round(r * 0.72 + 84 * 0.28),
    Math.round(g * 0.72 + 124 * 0.28),
    Math.round(b * 0.72 + 148 * 0.28),
    Math.round(a * 0.52),
  ])
  const coreUrl = createDerivedPngLayer(cutoutCanvas, (r, g, b, a, x, y, width, height) => {
    const nx = (x / width - 0.5) / 0.44
    const ny = (y / height - 0.48) / 0.4
    const mask = Math.max(0, 1 - Math.sqrt(nx * nx + ny * ny))
    if (mask <= 0) return null
    return [
      Math.min(255, Math.round(r * 1.08 + 8)),
      Math.min(255, Math.round(g * 1.04 + 6)),
      Math.min(255, Math.round(b * 1.12 + 12)),
      Math.round(a * Math.min(0.9, mask * 1.35)),
    ]
  })
  const frontUrl = createDerivedPngLayer(cutoutCanvas, (r, g, b, a, x, y, width, height) => {
    const brightness = (r + g + b) / 3
    const saturation = Math.max(r, g, b) - Math.min(r, g, b)
    const detail = Math.max(0, Math.min(1, (saturation - 28) / 110 + (brightness - 116) / 260))
    const upper = Math.max(0, 1 - Math.hypot((x / width - 0.56) / 0.42, (y / height - 0.38) / 0.46))
    const mask = Math.max(detail * 0.85, upper * 0.52)
    if (mask <= 0.08) return null
    return [
      Math.min(255, Math.round(r * 1.18 + 12)),
      Math.min(255, Math.round(g * 1.12 + 8)),
      Math.min(255, Math.round(b * 1.1 + 10)),
      Math.round(a * Math.min(0.82, mask)),
    ]
  })

  return {
    aspect,
    layers: [
      { id: 'shadow', className: 'layer-shadow', url: shadowUrl, z: -130, shiftX: -28, shiftY: -18, scale: 1.1, opacity: 0.92, snapshotX: -18, snapshotY: 20 },
      { id: 'depth', className: 'layer-depth', url: depthUrl, z: -70, shiftX: -18, shiftY: -10, scale: 1.04, opacity: 0.78, snapshotX: -10, snapshotY: 8 },
      { id: 'rim', className: 'layer-rim', url: createRimPngLayer(cutoutCanvas), z: -20, shiftX: -8, shiftY: -4, scale: 1.025, opacity: 0.82, snapshotX: -3, snapshotY: 2 },
      { id: 'body', className: 'layer-body', url: bodyUrl, z: 18, shiftX: 8, shiftY: 5, scale: 1, opacity: 1, snapshotX: 0, snapshotY: 0 },
      { id: 'core', className: 'layer-core', url: coreUrl, z: 74, shiftX: 22, shiftY: 13, scale: 1.018, opacity: 0.94, snapshotX: 8, snapshotY: -3 },
      { id: 'front', className: 'layer-front', url: frontUrl, z: 128, shiftX: 34, shiftY: 22, scale: 1.036, opacity: 0.92, snapshotX: 16, snapshotY: -8 },
      { id: 'particles', className: 'layer-particles', url: createParticlePngLayer(cutoutCanvas), z: 170, shiftX: 46, shiftY: 28, scale: 1.08, opacity: 0.96, snapshotX: 24, snapshotY: -13 },
      { id: 'highlight', className: 'layer-highlight', url: createHighlightPngLayer(cutoutCanvas), z: 210, shiftX: 54, shiftY: 32, scale: 1.03, opacity: 0.78, snapshotX: 12, snapshotY: -10 },
    ],
  }
}

function canvasToBlob(canvas, type = 'image/png', quality) {
  return new Promise((resolve) => {
    canvas.toBlob(resolve, type, quality)
  })
}

async function drawImageToCanvas(context, url, x, y, width, height, opacity = 1, filter = 'none') {
  const image = await loadImageFromUrl(url)
  context.save()
  context.globalAlpha = opacity
  context.filter = filter
  context.drawImage(image, x, y, width, height)
  context.restore()
}

export async function downloadLayeredPngSnapshot(imageUrl, filename) {
  const visual = await buildLayeredPngVisual(imageUrl)
  const canvas = createTransparentCanvas(1400, 900)
  const context = canvas.getContext('2d')
  const backdrop = context.createLinearGradient(0, 0, canvas.width, canvas.height)
  backdrop.addColorStop(0, '#fbf5e8')
  backdrop.addColorStop(1, '#edf6f0')
  context.fillStyle = backdrop
  context.fillRect(0, 0, canvas.width, canvas.height)

  const specimenWidth = visual.aspect >= 1 ? 760 : 760 * visual.aspect
  const specimenHeight = visual.aspect >= 1 ? 760 / visual.aspect : 760
  const originX = (canvas.width - specimenWidth) / 2
  const originY = (canvas.height - specimenHeight) / 2 + 10

  for (const layer of visual.layers) {
    await drawImageToCanvas(
      context,
      layer.url,
      originX + layer.snapshotX,
      originY + layer.snapshotY,
      specimenWidth,
      specimenHeight,
      layer.opacity,
      layer.id === 'shadow' ? 'blur(16px)' : 'none',
    )
  }

  const blob = await canvasToBlob(canvas)
  if (!blob) return false
  downloadBlob(filename, blob)
  return true
}
