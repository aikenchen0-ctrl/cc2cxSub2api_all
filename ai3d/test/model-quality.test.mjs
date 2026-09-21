import assert from 'node:assert/strict'
import test from 'node:test'

import { formatBytes, formatDuration, formatNumber, getModelQuality } from '../src/lib/modelQuality.js'

test('model quality scoring', async (t) => {
  await t.test('keeps built-in starter models in the usable range', () => {
    const quality = getModelQuality({ id: 'plant', custom: false }, null, [])

    assert.equal(quality.score, 68)
    assert.equal(quality.verdict, 'Usable')
    assert.equal(quality.providerLabel, 'Built-in')
    assert.equal(quality.hasGlb, false)
  })

  await t.test('rewards generated GLB assets with geometry and textures', () => {
    const quality = getModelQuality(
      {
        id: 'custom-1',
        custom: true,
        generation: {
          provider: 'hyper3d',
          status: 'success',
          modelUrl: '/api/3d/local-model/custom-1.glb',
        },
      },
      {
        fileBytes: 2_400_000,
        meshCount: 12,
        textureCount: 5,
        triangleCount: 72_000,
      },
      [{ cellId: 'custom-1', status: 'success', durationMs: 92_000 }],
    )

    assert.equal(quality.score, 98)
    assert.equal(quality.verdict, 'Demo-ready')
    assert.equal(quality.hasGlb, true)
    assert.equal(quality.fileBytes, 2_400_000)
  })

  await t.test('keeps failed generations clearly below demo quality', () => {
    const quality = getModelQuality({
      id: 'custom-failed',
      custom: true,
      generation: {
        provider: 'tripo',
        status: 'failed',
        modelUrl: '',
      },
    })

    assert.equal(quality.score, 12)
    assert.equal(quality.verdict, 'Failed')
  })
})

test('model quality formatters', () => {
  assert.equal(formatBytes(0), 'n/a')
  assert.equal(formatBytes(2_400_000), '2.4 MB')
  assert.equal(formatDuration(92_000), '1m 32s')
  assert.equal(formatNumber(72_000), '72,000')
})
