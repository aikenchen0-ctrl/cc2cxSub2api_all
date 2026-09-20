import { inferAssetCategory } from './assetIntelligence.js'

const MOTION_PROFILES = {
  artifact: {
    id: 'artifact',
    label: 'Museum turntable',
    durationMs: 9000,
    description: 'Dark-gallery turntable with close material inspection for artifacts.',
  },
  road: {
    id: 'road',
    label: 'Road push-in',
    durationMs: 7800,
    description: 'Low front dolly, forward drift, and showroom reveal for cars.',
  },
  aircraft: {
    id: 'aircraft',
    label: 'Flight pass',
    durationMs: 7200,
    description: 'Banked fly-by with a light tracking camera for aircraft.',
  },
  vessel: {
    id: 'vessel',
    label: 'Naval cruise',
    durationMs: 8600,
    description: 'Slow side tracking and heavy mass movement for ships and carriers.',
  },
  specimen: {
    id: 'specimen',
    label: 'Specimen orbit',
    durationMs: 8200,
    description: 'Close inspection orbit for biological and organic subjects.',
  },
  product: {
    id: 'product',
    label: 'Studio reveal',
    durationMs: 7600,
    description: 'Product turntable with a push-in and detail pause.',
  },
}

export function inferMotionProfile(cell = {}) {
  if (MOTION_PROFILES[cell.motionProfile]) return MOTION_PROFILES[cell.motionProfile]

  const category = inferAssetCategory(cell)
  return MOTION_PROFILES[category.motionProfile] || MOTION_PROFILES.product
}

export function getMotionProfile(profileId) {
  return MOTION_PROFILES[profileId] || MOTION_PROFILES.product
}
