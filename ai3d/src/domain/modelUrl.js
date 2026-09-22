export function getGeneratedModelUrl(cell) {
  if (!cell?.custom) return ''
  const generation = cell.generation || {}
  // Keep the server-provided URL.  A successful provider task normally
  // points at a cached local model, while a cache miss deliberately returns
  // the authenticated `/api/3d/model?...` proxy.  Rewriting that proxy to a
  // guessed `<taskId>.glb` path makes the owner's model 404 after a cache
  // failure or process restart.
  return generation.modelUrl || ''
}
