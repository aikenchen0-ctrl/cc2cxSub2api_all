export function getGeneratedModelUrl(cell) {
  if (!cell?.custom) return ''
  const generation = cell.generation || {}
  const modelUrl = generation.modelUrl || ''
  const taskId = generation.taskId
  if (taskId && (/\/api\/3d\/model\?/i.test(modelUrl) || /^https?:\/\//i.test(modelUrl))) {
    return `/api/3d/local-model/${encodeURIComponent(taskId)}.glb`
  }
  return modelUrl
}
