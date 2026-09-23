import { readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const PROJECT_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
export const DIST_ROOT = path.join(PROJECT_ROOT, 'dist')

const MIME_TYPES = new Map([
  ['.css', 'text/css; charset=utf-8'],
  ['.gif', 'image/gif'],
  ['.glb', 'model/gltf-binary'],
  ['.gltf', 'model/gltf+json'],
  ['.html', 'text/html; charset=utf-8'],
  ['.ico', 'image/x-icon'],
  ['.jpeg', 'image/jpeg'],
  ['.jpg', 'image/jpeg'],
  ['.js', 'text/javascript; charset=utf-8'],
  ['.json', 'application/json; charset=utf-8'],
  ['.map', 'application/json; charset=utf-8'],
  ['.mp4', 'video/mp4'],
  ['.png', 'image/png'],
  ['.svg', 'image/svg+xml'],
  ['.webp', 'image/webp'],
  ['.woff', 'font/woff'],
  ['.woff2', 'font/woff2'],
])

export async function serveStaticApp(request, response, url, root = DIST_ROOT) {
  if (!['GET', 'HEAD'].includes(request.method) || url.pathname.startsWith('/api/')) return false

  const pathname = decodePathname(url.pathname)
  if (pathname === null) {
    sendPlain(response, 400, 'Bad request')
    return true
  }

  const requested = resolveInside(root, pathname === '/' ? 'index.html' : pathname.slice(1))
  if (!requested) {
    sendPlain(response, 400, 'Bad request')
    return true
  }

  if (await isFile(requested)) {
    await sendFile(request, response, requested)
    return true
  }

  const acceptsHtml = String(request.headers.accept || '').includes('text/html')
  if (acceptsHtml && !path.extname(pathname)) {
    const indexFile = path.join(root, 'index.html')
    if (await isFile(indexFile)) {
      await sendFile(request, response, indexFile)
      return true
    }
  }

  sendPlain(response, 404, 'Not found')
  return true
}

function decodePathname(value) {
  try {
    const decoded = decodeURIComponent(value)
    if (decoded.includes('\\') || decoded.includes('\0')) return null
    return decoded
  } catch {
    return null
  }
}

function resolveInside(root, relativePath) {
  const absoluteRoot = path.resolve(root)
  const resolved = path.resolve(absoluteRoot, relativePath)
  return resolved === absoluteRoot || resolved.startsWith(`${absoluteRoot}${path.sep}`) ? resolved : null
}

async function isFile(filePath) {
  try {
    return (await stat(filePath)).isFile()
  } catch {
    return false
  }
}

async function sendFile(request, response, filePath) {
  const body = await readFile(filePath)
  const extension = path.extname(filePath).toLowerCase()
  response.statusCode = 200
  response.setHeader('Content-Type', MIME_TYPES.get(extension) || 'application/octet-stream')
  response.setHeader('Content-Length', body.length)
  response.setHeader('X-Content-Type-Options', 'nosniff')
  response.setHeader(
    'Cache-Control',
    path.basename(filePath) === 'index.html' ? 'no-cache' : 'public, max-age=31536000, immutable',
  )
  response.end(request.method === 'HEAD' ? undefined : body)
}

function sendPlain(response, status, message) {
  response.statusCode = status
  response.setHeader('Content-Type', 'text/plain; charset=utf-8')
  response.setHeader('Cache-Control', 'no-store')
  response.end(message)
}
