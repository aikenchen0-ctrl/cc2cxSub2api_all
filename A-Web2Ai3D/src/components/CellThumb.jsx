export function CellThumb({ cell, selected }) {
  const previewUrl = cell.thumbnailUrl || cell.imageUrl || ''

  return (
    <span
      className={`cell-thumb ${cell.custom ? 'custom-cell' : cell.id} ${selected ? 'selected' : ''}`}
      style={{ '--cell-accent': cell.accent, '--thumb-image': previewUrl ? `url(${previewUrl})` : undefined }}
    >
      <span />
    </span>
  )
}
