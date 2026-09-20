export function seeded(index) {
  const value = Math.sin(index * 12.9898 + 78.233) * 43758.5453
  return value - Math.floor(value)
}

export function clamp(value, min = 0, max = 1) {
  return Math.min(max, Math.max(min, value))
}

export function pickSpherePoint(index, radius = 1) {
  const theta = seeded(index * 3) * Math.PI * 2
  const phi = Math.acos(2 * seeded(index * 3 + 1) - 1)
  const spread = radius * (0.86 + seeded(index * 3 + 2) * 0.16)

  return [
    Math.sin(phi) * Math.cos(theta) * spread,
    Math.sin(phi) * Math.sin(theta) * spread,
    Math.cos(phi) * spread,
  ]
}
