import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { getAssetMetadata } from '../src/lib/assetMetadata.js'

describe('asset metadata inference', () => {
  it('describes museum artifacts from Chinese artifact keywords', () => {
    const metadata = getAssetMetadata({
      id: 'sanxingdui-mask',
      name: '戴金面罩青铜人头像',
      sourceFileName: '三星堆-戴金面罩青铜人头像.png',
      type: 'Uploaded 3D Asset',
      custom: true,
      generation: { provider: 'rodin', status: 'success', modelUrl: '/api/3d/local-model/mask.glb' },
    })

    assert.equal(metadata.subtitle, 'Museum Artifact')
    assert.match(metadata.description, /museum-style artifact/i)
    assert.deepEqual(metadata.facts.find(([label]) => label === 'Scene'), ['Scene', 'Museum Turntable'])
    assert.ok(metadata.tags.includes('artifact'))
  })

  it('keeps generated supercars on the road presentation path', () => {
    const metadata = getAssetMetadata({
      id: 'custom-supercar',
      name: 'hyper3d supercar test',
      fullName: 'hyper3d supercar test',
      sourceFileName: 'red-supercar.png',
      custom: true,
      template: 'animal',
      generation: { provider: 'rodin', status: 'success', modelUrl: '/api/3d/local-model/car.glb' },
    })

    assert.equal(metadata.subtitle, 'Performance Vehicle')
    assert.match(metadata.value, /Road push-in/)
    assert.ok(metadata.tags.includes('low camera'))
  })

  it('classifies aircraft carriers as vessels instead of aircraft', () => {
    const metadata = getAssetMetadata({
      id: 'carrier',
      name: 'chinese aircraft carrier',
      sourceFileName: 'chinese-aircraft-carrier.png',
      custom: true,
      generation: { provider: 'fal', status: 'success', modelUrl: '/api/3d/local-model/carrier.glb' },
    })

    assert.equal(metadata.subtitle, 'Naval Vessel')
    assert.match(metadata.value, /Naval cruise/)
  })

  it('does not label built-in starter scenes as Hyper3D assets', () => {
    const metadata = getAssetMetadata({
      id: 'plant',
      name: 'Plant Specimen',
      type: 'Starter Asset',
      template: 'plant',
      custom: false,
    })

    assert.deepEqual(metadata.facts.find(([label]) => label === 'Provider'), ['Provider', 'Built-in'])
  })
})
