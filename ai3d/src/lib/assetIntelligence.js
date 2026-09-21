export const ASSET_CATEGORIES = [
  {
    id: 'artifact',
    label: 'Museum Artifact',
    motionProfile: 'artifact',
    sceneProfile: 'artifact',
    keywords: ['artifact', 'bronze', 'gold mask', 'golden mask', 'mask', 'statue', 'relic', 'sanxingdui', '三星堆', '青铜', '金器', '金面', '黄金面具', '面具', '文物', '器物', '人像'],
    strongKeywords: ['sanxingdui', '三星堆', 'bronze mask', 'gold mask', 'golden mask', '青铜人头像', '戴金面罩', '黄金面具'],
    material: 'Metal, patina, carved relief, and aged surface detail',
    scale: 'Reference-derived object scale',
    inspectionFocus: 'silhouette, patina, relief, edge profile',
    description: 'A museum-style artifact asset. The important visual signals are silhouette, surface relief, material aging, and readable symbolic details rather than mechanical precision.',
    value: 'Works best as a slow inspection demo with side lighting, close orbit, and detail pauses on the face, edge profile, and weathered material transitions.',
    tags: ['artifact', 'patina', 'relief detail', 'museum display', 'inspection'],
  },
  {
    id: 'road',
    label: 'Performance Vehicle',
    motionProfile: 'road',
    sceneProfile: 'road',
    keywords: ['supercar', 'sports car', 'race car', 'ferrari', 'lamborghini', 'porsche', 'vehicle', 'automobile', 'car', 'truck', 'suv', 'motorcycle', '跑车', '赛车', '汽车', '法拉利', '兰博基尼', '保时捷', '卡车', '摩托'],
    strongKeywords: ['supercar', 'sports car', 'race car', 'ferrari', 'lamborghini', 'porsche', '跑车', '赛车', '法拉利', '兰博基尼', '保时捷'],
    material: 'Painted body panels, glass, rubber, alloy, and dark trim',
    scale: 'Single vehicle asset',
    inspectionFocus: 'stance, wheels, glass canopy, front profile',
    description: 'A road-vehicle asset where stance, wheel placement, front profile, glass canopy, and body highlights decide whether the model feels believable.',
    value: 'Use a low camera, road push-in, and front three-quarter framing. The demo should sell motion, gloss, and mass instead of treating it like a static specimen.',
    tags: ['vehicle', 'road pass', 'low camera', 'paint gloss', 'showcase'],
  },
  {
    id: 'vessel',
    label: 'Naval Vessel',
    motionProfile: 'vessel',
    sceneProfile: 'vessel',
    keywords: ['aircraft carrier', 'aircraft car', 'carrier', 'warship', 'destroyer', 'ship', 'vessel', 'naval', 'submarine', '航母', '航空母舰', '军舰', '驱逐舰', '舰', '船', '潜艇'],
    strongKeywords: ['aircraft carrier', 'aircraft car', '航母', '航空母舰', 'warship', 'destroyer'],
    material: 'Painted steel, deck surfaces, tower forms, antenna detail, and waterline mass',
    scale: 'Large vessel asset',
    inspectionFocus: 'hull length, deck plane, island tower, waterline',
    description: 'A naval-vessel asset. The key read is the long hull, deck plane, island/superstructure, and heavy silhouette rather than small decorative parts.',
    value: 'Use a slow side cruise, broad camera distance, water/wake cues, and a heavier pacing so the object does not feel like a small toy.',
    tags: ['naval', 'hull', 'deck', 'waterline', 'slow cruise'],
  },
  {
    id: 'aircraft',
    label: 'Aircraft',
    motionProfile: 'aircraft',
    sceneProfile: 'aircraft',
    keywords: ['fighter jet', 'fighter', 'airplane', 'aeroplane', 'aircraft', 'plane', 'jet', 'drone', 'helicopter', 'missile', '战斗机', '飞机', '歼', '轰炸机', '无人机', '直升机', '导弹'],
    strongKeywords: ['fighter jet', 'fighter', 'airplane', 'aircraft', '战斗机', '飞机', '无人机'],
    material: 'Painted fuselage, canopy glass, wing edges, intakes, and exhaust geometry',
    scale: 'Single aircraft asset',
    inspectionFocus: 'fuselage, wings, tail, canopy, engine areas',
    description: 'An aircraft asset where the fuselage centerline, wings, tail, canopy, and engine areas must stay coherent from multiple angles.',
    value: 'Use a flight-pass camera with banking, contrails, and forward drift. The demo should make direction and lift obvious.',
    tags: ['aircraft', 'flight pass', 'banking', 'canopy', 'aero form'],
  },
  {
    id: 'product',
    label: 'Product Object',
    motionProfile: 'product',
    sceneProfile: 'product',
    keywords: ['watch', 'phone', 'camera', 'shoe', 'bag', 'chair', 'lamp', 'bottle', 'headphone', 'jewelry', 'ring', '手表', '手机', '相机', '鞋', '包', '椅子', '灯', '瓶', '耳机', '戒指'],
    strongKeywords: ['watch', 'phone', 'camera', 'shoe', 'handbag', '手表', '手机', '相机'],
    material: 'Mixed product materials, edge highlights, texture breaks, and brand-like surface zones',
    scale: 'Single product asset',
    inspectionFocus: 'silhouette, material zones, recognizable feature layout',
    description: 'A product asset. The model quality depends on whether the silhouette, primary material zones, and recognisable feature layout survived generation.',
    value: 'Use a clean studio turntable, soft reflections, and short zoom pauses on the recognisable product features.',
    tags: ['product', 'turntable', 'studio light', 'material zones', 'detail pause'],
  },
  {
    id: 'specimen',
    label: 'Organic Specimen',
    motionProfile: 'specimen',
    sceneProfile: 'specimen',
    keywords: ['cell', 'biology', 'biological', 'organism', 'specimen', 'plant', 'neuron', 'bacteria', 'blood', 'epithelial', 'muscle', 'mosquito', '细胞', '生物', '植物', '神经', '细菌', '肌肉'],
    strongKeywords: ['cell', 'biology', 'biological', 'specimen', '细胞', '生物'],
    material: 'Soft translucent surfaces, organic volume, color-separated internal forms',
    scale: 'Specimen-style asset',
    inspectionFocus: 'overall volume, translucent surface, internal clusters',
    description: 'An organic/specimen asset. The useful read is the overall volume, translucent surface, and clustered internal detail rather than exact biological accuracy.',
    value: 'Use close orbit, clean rim light, and slower zooms. This works best as an educational inspection view.',
    tags: ['specimen', 'organic', 'inspection orbit', 'soft volume', 'education'],
  },
]

export const SCENE_PROFILES = {
  road: {
    id: 'road',
    label: 'Road Showcase',
    summary: 'Low road deck, moving lane marks, and front push-in camera.',
    environment: 'road deck',
    badges: ['low camera', 'moving lane', 'gloss read'],
  },
  aircraft: {
    id: 'aircraft',
    label: 'Sky Flight Pass',
    summary: 'Bright sky volume, contrail streaks, and banked fly-by motion.',
    environment: 'sky pass',
    badges: ['banking', 'contrails', 'forward drift'],
  },
  vessel: {
    id: 'vessel',
    label: 'Naval Waterline',
    summary: 'Water plane, broad wake, and slow side-tracking camera.',
    environment: 'waterline',
    badges: ['wake', 'side cruise', 'heavy scale'],
  },
  artifact: {
    id: 'artifact',
    label: 'Museum Turntable',
    summary: 'Dark gallery stage, warm spotlights, and close material inspection.',
    environment: 'museum plinth',
    badges: ['spotlight', 'patina', 'close orbit'],
  },
  product: {
    id: 'product',
    label: 'Studio Turntable',
    summary: 'Clean reflective studio floor, softboxes, and controlled product reveal.',
    environment: 'studio sweep',
    badges: ['turntable', 'reflection', 'detail pause'],
  },
  specimen: {
    id: 'specimen',
    label: 'Specimen Lab',
    summary: 'Soft lab volume, microscope-style depth lines, and close orbit.',
    environment: 'lab orbit',
    badges: ['rim light', 'inspection', 'soft volume'],
  },
}

export function getAssetIntelligence(cell = {}) {
  const category = inferAssetCategory(cell)
  return {
    category,
    scene: getSceneProfile(category.sceneProfile),
  }
}

export function inferAssetCategory(cell = {}) {
  const overrideId = normalizeCategoryOverride(cell.intelligence?.categoryId)
  if (overrideId) return buildCategoryWithInsight(ASSET_CATEGORIES.find((rule) => rule.id === overrideId), cell.intelligence)

  const primaryText = normalizeSearchText([
    cell.id,
    cell.fullName,
    cell.sourceFileName,
    cell.name,
    cell.type,
    cell.template,
  ])
  const secondaryText = normalizeSearchText([
    cell.referenceSummary,
    cell.referenceSource,
    cell.imageUrl,
    cell.thumbnailUrl,
    cell.generation?.provider,
    cell.generation?.message,
    cell.generation?.modelUrl,
    cell.generation?.rawModelUrl,
  ])

  const scored = ASSET_CATEGORIES
    .map((rule) => ({
      rule,
      score:
        scoreKeywords(primaryText, rule.keywords, 6) +
        scoreKeywords(secondaryText, rule.keywords, 2) +
        scoreKeywords(primaryText, rule.strongKeywords, 18) +
        scoreKeywords(secondaryText, rule.strongKeywords, 5),
    }))
    .map((entry) => applyCategoryOverrides(entry, primaryText))
    .sort((a, b) => b.score - a.score)

  if (scored[0]?.score > 0) return buildCategoryWithInsight(scored[0].rule, cell.intelligence)
  return ASSET_CATEGORIES.find((rule) => rule.id === 'product')
}

export function getSceneProfile(input = 'product') {
  const profileId = typeof input === 'string'
    ? input
    : input.sceneProfile || input.motionProfile || inferAssetCategory(input).sceneProfile

  return SCENE_PROFILES[profileId] || SCENE_PROFILES.product
}

function applyCategoryOverrides(entry, primaryText) {
  const score = entry.score

  if (entry.rule.id === 'vessel' && hasKeyword(primaryText, ['aircraft carrier', 'aircraft car', '航母', '航空母舰'])) {
    return { ...entry, score: score + 32 }
  }

  if (entry.rule.id === 'aircraft' && hasKeyword(primaryText, ['aircraft carrier', 'aircraft car', '航母', '航空母舰'])) {
    return { ...entry, score: score - 18 }
  }

  if (entry.rule.id === 'road' && hasKeyword(primaryText, ['supercar', 'sports car', 'race car', 'ferrari', 'lamborghini', 'porsche', '跑车', '赛车', '法拉利', '兰博基尼', '保时捷'])) {
    return { ...entry, score: score + 24 }
  }

  if (entry.rule.id === 'artifact' && hasKeyword(primaryText, ['sanxingdui', '三星堆', 'bronze mask', 'gold mask', '青铜人头像', '戴金面罩'])) {
    return { ...entry, score: score + 28 }
  }

  return entry
}

function buildCategoryWithInsight(category, insight) {
  if (!category || !insight?.configured) return category

  return {
    ...category,
    label: insight.categoryLabel || category.label,
    material: insight.material || category.material,
    description: insight.description || category.description,
    value: insight.presentation || category.value,
    inspectionFocus: insight.inspectionFocus || category.inspectionFocus,
    tags: [...(Array.isArray(insight.tags) ? insight.tags : []), ...category.tags],
  }
}

function normalizeCategoryOverride(value) {
  const normalized = String(value || '').trim().toLowerCase()
  return ASSET_CATEGORIES.some((rule) => rule.id === normalized) ? normalized : ''
}

function normalizeSearchText(parts) {
  return parts
    .filter(Boolean)
    .join(' ')
    .replace(/[_-]+/g, ' ')
    .toLowerCase()
}

function hasKeyword(text, keywords) {
  return keywords.some((keyword) => matchesKeyword(text, keyword))
}

function scoreKeywords(text, keywords = [], weight) {
  return keywords.reduce((score, keyword) => (matchesKeyword(text, keyword) ? score + getKeywordWeight(keyword, weight) : score), 0)
}

function matchesKeyword(text, keyword) {
  if (!text || !keyword) return false
  const normalizedKeyword = keyword.toLowerCase()
  if (/[a-z0-9]/i.test(normalizedKeyword)) {
    const escaped = normalizedKeyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    return new RegExp(`(^|[^a-z0-9])${escaped}([^a-z0-9]|$)`, 'i').test(text)
  }

  return text.includes(normalizedKeyword)
}

function getKeywordWeight(keyword, baseWeight) {
  return keyword.length > 5 ? baseWeight + 2 : baseWeight
}
