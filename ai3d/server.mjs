import http from 'node:http'
import { API_HOST, API_PORT, FAL_API_KEY, HUNYUAN_API_BASE, HUNYUAN_CLOUD_API_KEY, RODIN_API_KEY, TRIPO_API_KEY, hasConfiguredSecret } from './server/config.mjs'
import { assertLocalDiagnosticsRequest, readJsonBody, sendJson, setCorsHeaders } from './server/http-utils.mjs'
import { createRequestId, logEvent, ownerLogId, readRecentLogs, summarizeError, summarizePayload } from './server/logger.mjs'
import { importLocalModel, proxyModel, serveLocalModel } from './server/model-store.mjs'
import { createFalTask, getFalHealth, getFalTask } from './server/providers/fal.mjs'
import { createHunyuanTask, getHunyuanHealth, getHunyuanTask } from './server/providers/hunyuan.mjs'
import { createRodinTask, getRodinHealth, getRodinTask } from './server/providers/rodin.mjs'
import { createTripoTask, getTripoHealth, getTripoTask } from './server/providers/tripo.mjs'
import { analyzeAssetImage, getVisionHealth } from './server/providers/vision.mjs'
import { consumeTicket, createSession, getIdentity, isManaged, loadTaskOwners, rememberTaskOwner, requireIdentity, safeNext, verifyTicket } from './server/auth-sso.mjs'
import { serveStaticApp } from './server/static-app.mjs'

const DEFAULT_GENERATION_PROVIDER = 'rodin'
const taskOwners = new Map()
const taskOwnersReady = loadTaskOwners().then((owners) => {
  for (const [taskId, subject] of Object.entries(owners)) taskOwners.set(taskId, subject)
}).catch(() => {})

const server = http.createServer(async (request, response) => {
  const requestId = createRequestId()
  const startedAt = Date.now()
  let url = null

  try {
    setCorsHeaders(response, request)
    response.setHeader('X-Request-Id', requestId)

    if (request.method === 'OPTIONS') {
      response.writeHead(204)
      response.end()
      return
    }

    url = new URL(request.url, `http://${request.headers.host}`)
    await taskOwnersReady
    const query = Object.fromEntries(url.searchParams.entries())
    // The one-time SSO ticket is identity material, not an application
    // credential, but it must still not be retained in the satellite log.
    if (query.ticket) query.ticket = '[redacted]'
    await logEvent('info', 'http.request', {
      requestId,
      method: request.method,
      path: url.pathname,
      query,
    })

    if (request.method === 'GET' && url.pathname === '/api/auth/sso/callback') {
      let payload
      try {
        payload = verifyTicket(url.searchParams.get('ticket') || '')
        const identity = await consumeTicket(payload)
        response.setHeader('Cache-Control', 'no-store')
        response.setHeader('Referrer-Policy', 'no-referrer')
        await createSession(identity, request, response)
        response.statusCode = 303
        response.setHeader('Location', safeNext(payload.next))
        response.end()
      } catch (error) {
        response.statusCode = error.status || 400
        response.setHeader('Cache-Control', 'no-store')
        response.end(JSON.stringify({ error: error.message || 'SSO failed' }))
      }
      return
    }

    // Liveness is intentionally unauthenticated and does not reveal provider
    // configuration. Authenticated provider health remains /api/3d/health.
    if (request.method === 'GET' && url.pathname === '/health') {
      sendJson(response, 200, { ok: true, service: 'ai3d' })
      return
    }

    if (request.method === 'GET' && url.pathname === '/api/auth/me') {
      if (isManaged()) {
        const me = await requireIdentity(request)
        sendJson(response, 200, { id: me.id, username: me.username || '', displayName: me.displayName || '' })
      } else {
        sendJson(response, 200, { id: 'local', username: '', displayName: '' })
      }
      return
    }

    // Production serves the built React app from this process so the UI,
    // SSO callback, and API always share one origin. Static assets remain
    // public; the app itself gates user data on the HttpOnly session above.
    if (await serveStaticApp(request, response, url)) return

    const identity = isManaged() ? await requireIdentity(request) : (await getIdentity(request)) || { subject: '' }
    request.identity = identity

    if (request.method === 'GET' && url.pathname === '/api/3d/health') {
      const payload = {
        ok: true,
        providers: {
          tripo: getTripoHealth(),
          rodin: getRodinHealth(),
          hunyuan: getHunyuanHealth(),
          fal: getFalHealth(),
          vision: getVisionHealth(),
        },
      }
      sendJson(response, 200, payload)
      await logEvent('info', 'http.response', { requestId, path: url.pathname, status: 200, durationMs: Date.now() - startedAt, ...ownerLogFields(identity.subject) })
      return
    }

    if (request.method === 'GET' && url.pathname === '/api/3d/logs') {
      assertLocalDiagnosticsRequest(request)
      const payload = await readRecentLogs(url.searchParams.get('limit') || 100, identity.subject)
      sendJson(response, 200, payload)
      await logEvent('info', 'http.response', { requestId, path: url.pathname, status: 200, durationMs: Date.now() - startedAt, entries: payload.entries.length, ...ownerLogFields(identity.subject) })
      return
    }

    if (request.method === 'POST' && url.pathname === '/api/3d/analyze') {
      const payload = await readJsonBody(request)
      payload._sub2apiSubject = identity.subject
      await logEvent('info', 'asset.analyze.start', {
        requestId,
        payload: summarizePayload(payload),
        ...ownerLogFields(identity.subject),
      })
      const insight = await analyzeAssetImage(payload)

      sendJson(response, 200, insight)
      await logEvent('info', 'asset.analyze.success', {
        requestId,
        provider: insight.provider,
        configured: insight.configured,
        status: insight.status,
        categoryId: insight.categoryId,
        durationMs: Date.now() - startedAt,
        ...ownerLogFields(identity.subject),
      })
      return
    }

    if (request.method === 'POST' && url.pathname === '/api/3d/generate') {
      const payload = await readJsonBody(request)
      payload._sub2apiSubject = identity.subject
      const provider = payload.provider || DEFAULT_GENERATION_PROVIDER
      await logEvent('info', 'generation.create.start', {
        requestId,
        provider,
        payload: summarizePayload(payload),
        ...ownerLogFields(identity.subject),
      })
      const task = await createGenerationTask(provider, payload)
      if (task?.taskId && identity.subject) {
        taskOwners.set(String(task.taskId), identity.subject)
        await rememberTaskOwner(task.taskId, identity.subject)
      }

      sendJson(response, 200, task)
      await logEvent('info', 'generation.create.success', {
        requestId,
        provider,
        taskId: task.taskId,
        status: task.status,
        durationMs: Date.now() - startedAt,
        ...ownerLogFields(identity.subject),
      })
      return
    }

    if (request.method === 'GET' && url.pathname.startsWith('/api/3d/status/')) {
      const taskId = decodeURIComponent(url.pathname.replace('/api/3d/status/', ''))
      if (isManaged() && (!taskOwners.has(taskId) || taskOwners.get(taskId) !== identity.subject)) {
        throw Object.assign(new Error('Model task does not belong to this user.'), { status: 403 })
      }
      const provider = url.searchParams.get('provider') || DEFAULT_GENERATION_PROVIDER
      const task = await getGenerationTask(provider, taskId)
      // Providers may cache a completed task under a normalized provider id
      // (for example Rodin's UUID or Fal's `fal-${requestId}`) while the
      // public task id remains an encoded wrapper. Register the returned local
      // asset alias before the browser follows its modelUrl, otherwise the
      // ownership check would reject the owner's own completed model.
      if (task?.modelUrl && identity.subject) {
        await rememberLocalModelOwner(task.modelUrl, identity.subject)
      }

      sendJson(response, 200, task)
      await logEvent('info', 'generation.status', {
        requestId,
        provider,
        taskId,
        status: task.status,
        progress: task.progress,
        hasModelUrl: Boolean(task.modelUrl),
        error: task.error,
        durationMs: Date.now() - startedAt,
        ...ownerLogFields(identity.subject),
      })
      return
    }

    if (request.method === 'GET' && url.pathname === '/api/3d/model') {
      if (isManaged()) {
        const taskId = String(url.searchParams.get('taskId') || '').trim()
        if (!taskId || !taskOwners.has(taskId) || taskOwners.get(taskId) !== identity.subject) {
          throw Object.assign(new Error('Model asset does not belong to this user.'), { status: 403 })
        }
      }
      await proxyModel(url, response)
      await logEvent('info', 'model.proxy.success', { requestId, durationMs: Date.now() - startedAt, ...ownerLogFields(identity.subject) })
      return
    }

    if (request.method === 'POST' && url.pathname === '/api/3d/local-model') {
      const model = await importLocalModel(request, url, identity.subject)
      if (model?.taskId && identity.subject) {
        taskOwners.set(String(model.taskId), identity.subject)
        await rememberTaskOwner(model.taskId, identity.subject)
      }
      sendJson(response, 200, model)
      await logEvent('info', 'model.import.success', {
        requestId,
        taskId: model.taskId,
        modelUrl: model.modelUrl,
        fileName: model.fileName,
        durationMs: Date.now() - startedAt,
        ...ownerLogFields(identity.subject),
      })
      return
    }

    if (request.method === 'GET' && url.pathname.startsWith('/api/3d/local-model/')) {
      const requested = decodeURIComponent(url.pathname.replace('/api/3d/local-model/', '')).replace(/\.(?:glb|gltf)$/i, '')
      if (isManaged() && (!taskOwners.has(requested) || taskOwners.get(requested) !== identity.subject)) {
        throw Object.assign(new Error('Model asset does not belong to this user.'), { status: 403 })
      }
      await serveLocalModel(url, response)
      await logEvent('info', 'model.local.success', { requestId, path: url.pathname, durationMs: Date.now() - startedAt, ...ownerLogFields(identity.subject) })
      return
    }

    sendJson(response, 404, { error: 'Not found' })
    await logEvent('warn', 'http.not_found', { requestId, path: url.pathname, durationMs: Date.now() - startedAt, ...ownerLogFields(request.identity?.subject) })
  } catch (error) {
    if (response.headersSent) {
      await logEvent('error', 'http.stream_error', {
        requestId,
        path: url?.pathname,
        durationMs: Date.now() - startedAt,
        error: summarizeError(error),
        ...ownerLogFields(request.identity?.subject),
      })
      response.destroy(error)
      return
    }

    const status = error.status || 500
    sendJson(response, status, {
      error: error.message || 'Server error',
      detail: error.detail,
    })
    await logEvent(status >= 500 ? 'error' : 'warn', 'http.error', {
      requestId,
      method: request.method,
      path: url?.pathname,
      status,
      durationMs: Date.now() - startedAt,
      error: summarizeError(error),
      ...ownerLogFields(request.identity?.subject),
    })
  }
})

server.listen(API_PORT, API_HOST, () => {
  console.log(`cc2cx AI3D API running at http://${API_HOST}:${API_PORT}`)
  console.log(hasConfiguredSecret(TRIPO_API_KEY) ? 'Tripo API key loaded from environment.' : 'TRIPO_API_KEY is missing. Add it to .env.local.')
  console.log(hasConfiguredSecret(RODIN_API_KEY) ? 'Rodin API key loaded from environment.' : 'RODIN_API_KEY is missing. Add it to .env.local.')
  console.log(hasConfiguredSecret(FAL_API_KEY) ? 'Fal API key loaded from environment.' : 'FAL_API_KEY is missing. Add it to .env.local.')
  console.log(getVisionHealth().configured ? 'Vision analysis provider configured.' : 'Vision analysis is not configured. Add OPENAI_API_KEY to .env.local.')
  console.log(hasConfiguredSecret(HUNYUAN_CLOUD_API_KEY) ? 'Hunyuan cloud API key loaded from environment.' : `Hunyuan3D local provider: ${HUNYUAN_API_BASE || 'not configured'}`)
  logEvent('info', 'api.start', {
    host: API_HOST,
    port: API_PORT,
    providers: {
      tripo: Boolean(TRIPO_API_KEY),
      rodin: Boolean(RODIN_API_KEY),
      fal: Boolean(FAL_API_KEY),
      hunyuan: Boolean(hasConfiguredSecret(HUNYUAN_CLOUD_API_KEY) || HUNYUAN_API_BASE),
      hunyuanCloud: hasConfiguredSecret(HUNYUAN_CLOUD_API_KEY),
      vision: getVisionHealth().configured,
    },
  })
})

function createGenerationTask(provider, payload) {
  if (provider === 'hunyuan') return createHunyuanTask(payload)
  if (provider === 'fal') return createFalTask(payload)
  if (provider === 'tripo') return createTripoTask(payload)
  if (provider === 'rodin') return createRodinTask(payload)
  throw Object.assign(new Error(`Unsupported 3D provider: ${provider || '(missing)'}`), { status: 400 })
}

function getGenerationTask(provider, taskId) {
  if (provider === 'hunyuan') return getHunyuanTask(taskId)
  if (provider === 'fal') return getFalTask(taskId)
  if (provider === 'tripo') return getTripoTask(taskId)
  if (provider === 'rodin') return getRodinTask(taskId)
  throw Object.assign(new Error(`Unsupported 3D provider: ${provider || '(missing)'}`), { status: 400 })
}

function ownerLogFields(subject) {
  const ownerId = ownerLogId(subject)
  return ownerId ? { ownerId } : {}
}

async function rememberLocalModelOwner(modelUrl, subject) {
  const owner = String(subject || '').trim()
  if (!owner || typeof modelUrl !== 'string') return

  let pathname
  try {
    pathname = new URL(modelUrl, 'http://ai3d.local').pathname
  } catch {
    return
  }
  const prefix = '/api/3d/local-model/'
  if (!pathname.startsWith(prefix)) return
  const fileName = decodeURIComponent(pathname.slice(prefix.length))
  const modelId = fileName.replace(/\.(?:glb|gltf)$/i, '').trim()
  if (!modelId) return

  taskOwners.set(modelId, owner)
  await rememberTaskOwner(modelId, owner)
}
