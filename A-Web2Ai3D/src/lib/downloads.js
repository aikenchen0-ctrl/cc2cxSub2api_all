export function downloadJson(filename, payload) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  downloadBlob(filename, blob)
}

export function downloadText(filename, text, type = 'text/plain;charset=utf-8') {
  downloadBlob(filename, new Blob([text], { type }))
}

export function downloadBlob(filename, blob) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

export async function exportObjectAsGlb(object) {
  if (!object) {
    throw new Error('No exportable model is mounted.')
  }

  const { GLTFExporter } = await import('three/examples/jsm/exporters/GLTFExporter.js')

  return new Promise((resolve, reject) => {
    const exportRoot = object.clone(true)
    exportRoot.traverse((node) => {
      if (!node.isMesh && !node.isLine && !node.isLineSegments) return

      node.castShadow = false
      node.receiveShadow = false
      if (Array.isArray(node.material)) {
        node.material = node.material.map((material) => material.clone())
      } else if (node.material) {
        node.material = node.material.clone()
      }
    })

    const exporter = new GLTFExporter()
    exporter.parse(
      exportRoot,
      (result) => {
        if (result instanceof ArrayBuffer) {
          resolve(new Blob([result], { type: 'model/gltf-binary' }))
          return
        }

        resolve(new Blob([JSON.stringify(result)], { type: 'model/gltf+json' }))
      },
      (error) => reject(error),
      {
        binary: true,
        onlyVisible: true,
        trs: false,
      },
    )
  })
}

export function getCanvasImageDataUrl({ scale = 1, maxWidth = 0 } = {}) {
  const canvas = document.querySelector('.cell-viewer canvas')
  if (!canvas) return ''

  try {
    const outputScale = Math.max(1, Number(scale) || 1)
    const widthScale = maxWidth > 0 ? Math.min(outputScale, maxWidth / canvas.width) : outputScale
    const finalScale = Math.max(0.2, widthScale)
    const output = document.createElement('canvas')
    output.width = Math.max(1, Math.round(canvas.width * finalScale))
    output.height = Math.max(1, Math.round(canvas.height * finalScale))
    const context = output.getContext('2d')
    context.imageSmoothingEnabled = true
    context.imageSmoothingQuality = 'high'
    context.drawImage(canvas, 0, 0, output.width, output.height)
    return output.toDataURL('image/png')
  } catch {
    return ''
  }
}

export function downloadCanvasImage(filename, scale = 1) {
  const dataUrl = getCanvasImageDataUrl({ scale })
  if (!dataUrl) return false

  try {
    const link = document.createElement('a')
    link.href = dataUrl
    link.download = filename
    link.click()
    return true
  } catch {
    return false
  }
}
