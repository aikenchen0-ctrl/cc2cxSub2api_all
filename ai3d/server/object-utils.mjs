export function findFirstValue(value, keys) {
  if (!value || typeof value !== 'object') return ''

  for (const key of keys) {
    if (typeof value[key] === 'string' && value[key]) return value[key]
  }

  for (const child of Object.values(value)) {
    if (Array.isArray(child)) {
      for (const item of child) {
        const found = findFirstValue(item, keys)
        if (found) return found
      }
    } else if (child && typeof child === 'object') {
      const found = findFirstValue(child, keys)
      if (found) return found
    }
  }

  return ''
}

export function findModelUrl(value) {
  const urls = []
  collectUrls(value, urls)

  const glb = urls.find((url) => /\.glb(?:[?#]|$)/i.test(url))
  if (glb) return glb

  return urls.find((url) => /\.gltf(?:[?#]|$)/i.test(url)) || ''
}

export function isSuccessStatus(status) {
  return ['success', 'succeeded', 'completed', 'complete', 'done', 'finish', 'finished'].includes(String(status || '').toLowerCase())
}

function collectUrls(value, urls) {
  if (!value) return

  if (typeof value === 'string') {
    if (/^https?:\/\//i.test(value)) urls.push(value)
    return
  }

  if (Array.isArray(value)) {
    value.forEach((item) => collectUrls(item, urls))
    return
  }

  if (typeof value === 'object') {
    Object.values(value).forEach((item) => collectUrls(item, urls))
  }
}
