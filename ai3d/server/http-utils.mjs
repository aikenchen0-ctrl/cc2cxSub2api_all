import { BODY_LIMIT, MODEL_UPLOAD_LIMIT } from './config.mjs'

export function setCorsHeaders(response) {
  response.setHeader('Access-Control-Allow-Origin', process.env.CORS_ORIGIN || '*')
  response.setHeader('Access-Control-Allow-Methods', 'GET,POST,OPTIONS')
  response.setHeader('Access-Control-Allow-Headers', 'Content-Type,Authorization')
}

export function assertLocalDiagnosticsRequest(request) {
  const remoteAddress = normalizeAddress(request.socket?.remoteAddress)
  const origin = request.headers.origin
  const referer = request.headers.referer

  if (!isLocalHost(remoteAddress)) {
    throw Object.assign(new Error('Diagnostics logs are only available from this machine.'), { status: 403 })
  }

  if (origin && !isLocalUrl(origin)) {
    throw Object.assign(new Error('Diagnostics logs are only available to localhost pages.'), { status: 403 })
  }

  if (!origin && referer && !isLocalUrl(referer)) {
    throw Object.assign(new Error('Diagnostics logs are only available to localhost pages.'), { status: 403 })
  }
}

export function sendJson(response, status, payload) {
  response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' })
  response.end(JSON.stringify(payload))
}

export function readJsonBody(request) {
  return new Promise((resolve, reject) => {
    const chunks = []
    let size = 0

    request.on('data', (chunk) => {
      size += chunk.length
      if (size > BODY_LIMIT) {
        reject(Object.assign(new Error('Image payload is too large.'), { status: 413 }))
        request.destroy()
        return
      }
      chunks.push(chunk)
    })

    request.on('end', () => {
      try {
        const raw = Buffer.concat(chunks).toString('utf8')
        resolve(raw ? JSON.parse(raw) : {})
      } catch {
        reject(Object.assign(new Error('Invalid JSON payload.'), { status: 400 }))
      }
    })

    request.on('error', reject)
  })
}

export function readRawBody(request, limit = MODEL_UPLOAD_LIMIT) {
  return new Promise((resolve, reject) => {
    const chunks = []
    let size = 0

    request.on('data', (chunk) => {
      size += chunk.length
      if (size > limit) {
        reject(Object.assign(new Error('Model payload is too large.'), { status: 413 }))
        request.destroy()
        return
      }
      chunks.push(chunk)
    })

    request.on('end', () => {
      resolve(Buffer.concat(chunks))
    })

    request.on('error', reject)
  })
}

export function parseDataUrl(dataUrl) {
  if (typeof dataUrl !== 'string') {
    throw Object.assign(new Error('imageDataUrl is required.'), { status: 400 })
  }

  const match = dataUrl.match(/^data:(image\/(?:png|jpe?g|webp));base64,(.+)$/)
  if (!match) {
    throw Object.assign(new Error('Only PNG, JPEG, or WebP image data URLs are supported.'), { status: 400 })
  }

  const mime = match[1]
  const buffer = Buffer.from(match[2], 'base64')
  const ext = mime.includes('png') ? 'png' : mime.includes('webp') ? 'webp' : 'jpg'

  if (buffer.length < 1024) {
    throw Object.assign(new Error('Image is too small for 3D generation.'), { status: 400 })
  }

  return { mime, buffer, ext }
}

export function sanitizeFileName(fileName) {
  const baseName = String(fileName).split(/[\\/]/).pop() || ''
  return baseName.replace(/[^\w.\- ]+/g, '').replace(/^\.+/, '').trim() || 'asset-reference.png'
}

function normalizeAddress(address = '') {
  return String(address).replace(/^::ffff:/, '')
}

function isLocalHost(hostname = '') {
  const normalized = String(hostname).toLowerCase()
  return normalized === 'localhost' || normalized === '127.0.0.1' || normalized === '::1'
}

function isLocalUrl(value) {
  try {
    return isLocalHost(new URL(value).hostname)
  } catch {
    return false
  }
}
