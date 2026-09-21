const DB_NAME = 'model-studio-3d-assets'
const DB_VERSION = 1
const STORE_NAME = 'models'

export async function listStoredModels() {
  if (!canUseIndexedDb()) return []

  try {
    const db = await openModelDb()
    const models = await requestToPromise(db.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).getAll())
    return sortModels(models)
  } catch {
    return []
  }
}

export async function saveStoredModels(models) {
  if (!canUseIndexedDb()) return false

  try {
    const db = await openModelDb()
    const transaction = db.transaction(STORE_NAME, 'readwrite')
    const done = transactionToPromise(transaction)
    const store = transaction.objectStore(STORE_NAME)
    store.clear()
    ;(Array.isArray(models) ? models : []).forEach((model, index) => {
      store.put({
        ...model,
        libraryOrder: index,
        savedAt: model.savedAt || new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      })
    })
    await done
    return true
  } catch {
    return false
  }
}

function canUseIndexedDb() {
  return typeof window !== 'undefined' && Boolean(window.indexedDB)
}

function openModelDb() {
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

function transactionToPromise(transaction) {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error)
    transaction.onabort = () => reject(transaction.error)
  })
}

function sortModels(models) {
  return [...(Array.isArray(models) ? models : [])].sort((a, b) => {
    if (Number.isFinite(a.libraryOrder) && Number.isFinite(b.libraryOrder)) {
      return a.libraryOrder - b.libraryOrder
    }
    return String(b.savedAt || '').localeCompare(String(a.savedAt || ''))
  })
}
