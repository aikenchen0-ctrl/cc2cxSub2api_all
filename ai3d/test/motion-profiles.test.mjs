import assert from 'node:assert/strict'
import test from 'node:test'

import { inferMotionProfile } from '../src/lib/motionProfiles.js'

test('infers object-aware demo motion profiles', () => {
  assert.equal(inferMotionProfile({ name: 'hyper3d supercar test' }).id, 'road')
  assert.equal(inferMotionProfile({ name: 'hyper3d supercar test', generation: { modelUrl: '/generated-models/aircraft-test.glb' } }).id, 'road')
  assert.equal(inferMotionProfile({ name: 'advanced fighter jet render' }).id, 'aircraft')
  assert.equal(inferMotionProfile({ name: 'Chinese aircraft carrier' }).id, 'vessel')
  assert.equal(inferMotionProfile({ name: 'chinese aircraft car...' }).id, 'vessel')
  assert.equal(inferMotionProfile({ name: '戴金面罩青铜人头像', sourceFileName: 'sanxingdui-bronze-mask.png' }).id, 'artifact')
  assert.equal(inferMotionProfile({ name: 'Plant Cell', template: 'plant' }).id, 'specimen')
  assert.equal(inferMotionProfile({ name: 'Luxury watch model' }).id, 'product')
})
