import { PROJECT_FALLBACK_STORAGE_KEY } from '../config/appConfig.js'
import { loadStoredValue, storeValue } from './storage.js'

const DB_NAME = '3dcellforge-projects'
const DB_VERSION = 1
const STORE_NAME = 'projects'

export async function listProjects() {
  if (!canUseIndexedDb()) return getFallbackProjects()
  const db = await openProjectDb()

  return requestToPromise(db.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).getAll())
    .then((projects) => sortProjects(projects))
    .catch(() => getFallbackProjects())
}

export async function saveProject(project) {
  const next = {
    ...project,
    id: project.id || `project-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    savedAt: new Date().toISOString(),
    version: 1,
  }

  if (!canUseIndexedDb()) {
    saveFallbackProject(next)
    return next
  }

  try {
    const db = await openProjectDb()
    await requestToPromise(db.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).put(next))
    return next
  } catch {
    saveFallbackProject(next)
    return next
  }
}

export async function loadProject(projectId) {
  if (!canUseIndexedDb()) return getFallbackProjects().find((project) => project.id === projectId) || null

  try {
    const db = await openProjectDb()
    return await requestToPromise(db.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).get(projectId))
  } catch {
    return getFallbackProjects().find((project) => project.id === projectId) || null
  }
}

export async function deleteProject(projectId) {
  if (!canUseIndexedDb()) {
    storeValue(PROJECT_FALLBACK_STORAGE_KEY, getFallbackProjects().filter((project) => project.id !== projectId))
    return
  }

  try {
    const db = await openProjectDb()
    await requestToPromise(db.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).delete(projectId))
  } catch {
    storeValue(PROJECT_FALLBACK_STORAGE_KEY, getFallbackProjects().filter((project) => project.id !== projectId))
  }
}

function canUseIndexedDb() {
  return typeof window !== 'undefined' && Boolean(window.indexedDB)
}

function openProjectDb() {
  return new Promise((resolve, reject) => {
    const request = window.indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: 'id' })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function requestToPromise(request) {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function getFallbackProjects() {
  return sortProjects(loadStoredValue(PROJECT_FALLBACK_STORAGE_KEY, []))
}

function saveFallbackProject(project) {
  const projects = getFallbackProjects().filter((item) => item.id !== project.id)
  storeValue(PROJECT_FALLBACK_STORAGE_KEY, sortProjects([project, ...projects]).slice(0, 20))
}

function sortProjects(projects) {
  return [...(Array.isArray(projects) ? projects : [])].sort((a, b) => String(b.savedAt || '').localeCompare(String(a.savedAt || '')))
}
