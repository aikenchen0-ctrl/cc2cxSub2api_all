import { createHmac, timingSafeEqual, randomBytes, createHash } from 'node:crypto'
import { readFile, writeFile, mkdir, rename } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { hasConfiguredSecret } from './config.mjs'

export const SESSION_COOKIE = 'ai3d_session'
export const SESSION_TTL_SECONDS = 3 * 24 * 60 * 60

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const DB_FILE = path.resolve(ROOT, process.env.AI3D_AUTH_DB || '.auth/sessions.json')
let taskOwnerWrite = Promise.resolve()
let dbMutation = Promise.resolve()

function decodePart(value) {
  const normalized = String(value || '').replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - String(value || '').length % 4) % 4)
  return Buffer.from(normalized, 'base64')
}

function encodePart(value) {
  return Buffer.from(value).toString('base64url')
}

function hash(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

async function readDb() {
  try {
    const parsed = JSON.parse(await readFile(DB_FILE, 'utf8'))
    return {
      identities: parsed.identities && typeof parsed.identities === 'object' ? parsed.identities : {},
      consumed: parsed.consumed && typeof parsed.consumed === 'object' ? parsed.consumed : {},
      sessions: parsed.sessions && typeof parsed.sessions === 'object' ? parsed.sessions : {},
      taskOwners: parsed.taskOwners && typeof parsed.taskOwners === 'object' ? parsed.taskOwners : {},
    }
  } catch {
    return { identities: {}, consumed: {}, sessions: {}, taskOwners: {} }
  }
}

async function writeDb(db) {
  await mkdir(path.dirname(DB_FILE), { recursive: true })
  const temp = `${DB_FILE}.${process.pid}.${Date.now()}.tmp`
  await writeFile(temp, JSON.stringify(db), 'utf8')
  await rename(temp, DB_FILE)
}

// All auth/task-owner updates are read-modify-write operations on one JSON
// file. Serializing the whole transaction prevents concurrent SSO callbacks,
// session creation, and task-owner writes from silently overwriting each
// other's identities or ownership records.
function withDbMutation(mutator) {
  const operation = dbMutation.then(async () => {
    const db = await readDb()
    const result = await mutator(db)
    await writeDb(db)
    return result
  })
  dbMutation = operation.catch(() => {})
  return operation
}

export function safeNext(value) {
  const next = typeof value === 'string' ? value.trim() : ''
  const hasControlCharacter = [...next].some((character) => character.charCodeAt(0) < 0x20)
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.includes('\\') || next.length > 2048 || hasControlCharacter) return '/'
  return next
}

export function verifyTicket(raw, now = Math.floor(Date.now() / 1000)) {
  const secret = String(process.env.SUB2API_SSO_SECRET || '').trim()
  if (!hasConfiguredSecret(secret) || secret.length < 32) throw Object.assign(new Error('SSO is not configured'), { status: 503 })
  if (typeof raw !== 'string' || raw.length > 8192) throw Object.assign(new Error('Invalid SSO ticket'), { status: 400 })
  let encoded, signature, payload
  try {
    const parts = raw.split('.')
    if (parts.length !== 2) throw new Error('malformed')
    encoded = parts[0]
    signature = parts[1]
    if (!encoded || !signature) throw new Error('malformed')
    const expected = createHmac('sha256', secret).update(encoded).digest()
    const actual = decodePart(signature)
    if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) throw new Error('signature')
    payload = JSON.parse(decodePart(encoded).toString('utf8'))
  } catch (error) {
    throw Object.assign(new Error('Invalid SSO ticket'), { status: 400, cause: error })
  }
  if (!payload || typeof payload !== 'object' || payload.iss !== 'sub2api' || payload.aud !== 'ai3d') throw Object.assign(new Error('Invalid SSO audience'), { status: 400 })
  if ('rk' in payload || 'api_key' in payload || 'super_key' in payload) throw Object.assign(new Error('Credential material is not allowed'), { status: 400 })
  if (typeof payload.sub !== 'string' || !payload.sub.trim() || payload.sub.length > 160 || typeof payload.jti !== 'string' || !payload.jti.trim()) throw Object.assign(new Error('Invalid SSO identity'), { status: 400 })
  if (!Number.isInteger(payload.iat) || !Number.isInteger(payload.exp) || payload.iat <= 0 || payload.exp <= payload.iat || payload.exp <= now || payload.exp - payload.iat > 120 || payload.iat > now + 30) throw Object.assign(new Error('Expired SSO ticket'), { status: 400 })
  return payload
}

export async function consumeTicket(payload) {
  return withDbMutation(async (db) => {
    const now = Math.floor(Date.now() / 1000)
    for (const [key, expiry] of Object.entries(db.consumed)) if (Number(expiry) < now) delete db.consumed[key]
    if (db.consumed[payload.jti]) throw Object.assign(new Error('SSO ticket already used'), { status: 400 })
    db.consumed[payload.jti] = payload.exp
    const identityKey = `sub2api:${payload.sub}`
    const identity = db.identities[identityKey] || {
      id: hash(identityKey).slice(0, 32),
      issuer: 'sub2api',
      subject: payload.sub,
      username: String(payload.username || ''),
      displayName: String(payload.displayName || ''),
      createdAt: now,
    }
    identity.username = String(payload.username || identity.username || '')
    identity.displayName = String(payload.displayName || identity.displayName || '')
    identity.updatedAt = now
    db.identities[identityKey] = identity
    return identity
  })
}

export async function createSession(identity, request, response) {
  const token = randomBytes(32).toString('base64url')
  const now = Math.floor(Date.now() / 1000)
  await withDbMutation(async (db) => {
    db.sessions[hash(token)] = { identityKey: `sub2api:${identity.subject}`, expiresAt: now + SESSION_TTL_SECONDS, createdAt: now }
  })
  const forwarded = String(request.headers['x-forwarded-proto'] || '').split(',')[0].trim().toLowerCase()
  const secure = request.socket?.encrypted || forwarded === 'https'
  const attrs = [`${SESSION_COOKIE}=${encodeURIComponent(token)}`, 'Path=/', 'HttpOnly', 'SameSite=Lax', `Max-Age=${SESSION_TTL_SECONDS}`]
  if (secure) attrs.push('Secure')
  response.setHeader('Set-Cookie', attrs.join('; '))
}

function readCookie(request, name) {
  const raw = String(request.headers.cookie || '')
  const match = raw.match(new RegExp(`(?:^|;\\s*)${name}=([^;]*)`))
  if (!match) return ''
  try {
    return decodeURIComponent(match[1])
  } catch {
    return ''
  }
}

export async function getIdentity(request) {
  const token = readCookie(request, SESSION_COOKIE)
  if (!token) return null
  const db = await readDb()
  const session = db.sessions[hash(token)]
  if (!session || Number(session.expiresAt) <= Math.floor(Date.now() / 1000)) return null
  return db.identities[session.identityKey] || null
}

export async function requireIdentity(request) {
  const identity = await getIdentity(request)
  if (!identity) throw Object.assign(new Error('Authentication required'), { status: 401 })
  return identity
}

// Generation task ownership is persisted alongside the local auth database so
// a process restart cannot turn an authenticated user's task id into a public
// lookup. Values are kept deliberately minimal: only the Sub2API subject is
// stored, never a credential or request payload.
export async function loadTaskOwners() {
  const db = await readDb()
  const owners = {}
  for (const [taskId, value] of Object.entries(db.taskOwners || {})) {
    const subject = typeof value === 'string' ? value : value?.subject
    if (typeof subject === 'string' && subject.trim()) owners[String(taskId)] = subject.trim()
  }
  return owners
}

export function rememberTaskOwner(taskId, subject) {
  const id = String(taskId || '').trim()
  const owner = String(subject || '').trim()
  if (!id || !owner) return Promise.resolve()
  taskOwnerWrite = taskOwnerWrite.then(async () => {
    await withDbMutation(async (db) => {
      db.taskOwners[id] = { subject: owner, updatedAt: Math.floor(Date.now() / 1000) }
      const entries = Object.entries(db.taskOwners)
      if (entries.length > 10000) {
        entries.sort((a, b) => Number(a[1]?.updatedAt || 0) - Number(b[1]?.updatedAt || 0))
        for (const [staleId] of entries.slice(0, entries.length - 10000)) delete db.taskOwners[staleId]
      }
    })
  })
  return taskOwnerWrite
}

export function isManaged() {
  // An explicitly supplied placeholder must not silently select the local,
  // unauthenticated mode.  Keep the process managed and let the credential
  // validators fail closed with a 503 until real values are configured.
  return Boolean(String(process.env.SUB2API_SSO_SECRET || '').trim() || String(process.env.SUB2API_APP_CREDENTIAL || '').trim())
}

export function relayBaseUrl() {
  let raw = String(process.env.SUB2API_RELAY_BASE_URL || process.env.LINK || '').trim()
  if (raw && !raw.includes('://')) raw = `http://${raw}`
  raw = raw.replace(/\/$/, '')
  return raw.endsWith('/v1') ? raw : raw ? `${raw}/v1` : ''
}

export function satelliteHeaders(subject, { required = true } = {}) {
  const credential = String(process.env.SUB2API_APP_CREDENTIAL || '').trim()
  const user = String(subject || '').trim()
  if (required && (!hasConfiguredSecret(credential) || !user)) throw Object.assign(new Error('Sub2API satellite credentials are not configured'), { status: 503 })
  if (!hasConfiguredSecret(credential) || !user) return {}
  return {
    Authorization: `Bearer ${credential}`,
    'X-Sub2API-On-Behalf-Of': user,
    'X-Sub2API-Satellite': 'ai3d',
  }
}

export function sub2apiV1Base() {
  let base = String(process.env.SUB2API_RELAY_BASE_URL || process.env.LINK || '').trim().replace(/\/+$/, '')
  if (base && !/^https?:\/\//i.test(base)) base = `http://${base}`
  return base ? (base.endsWith('/v1') ? base : `${base}/v1`) : ''
}

export function sub2apiPurchaseUrl() {
  const raw = String(process.env.LINK || '').trim()
  if (!raw) return null
  try {
    const url = new URL(/^https?:\/\//i.test(raw) ? raw : `http://${raw}`)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return null
    url.pathname = '/purchase'
    url.search = ''
    url.hash = ''
    return url.toString().replace(/\/$/, '')
  } catch {
    return null
  }
}

export { encodePart }
