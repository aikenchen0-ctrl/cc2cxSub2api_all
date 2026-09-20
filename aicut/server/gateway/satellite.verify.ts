import assert from 'node:assert/strict';
import {
  satelliteCapsEnabled,
  satelliteImageModel,
  satelliteOrigin,
  satelliteV1Base,
  satelliteVideoModel,
} from './satellite.ts';

process.env.SUB2API_BASE = 'localhost:18080';
process.env.SUB2API_APP_CREDENTIAL = 'sat-secret';
assert.equal(satelliteOrigin(), 'http://localhost:18080');
assert.equal(satelliteV1Base(), 'http://localhost:18080/v1');
assert.equal(satelliteCapsEnabled(), true);
assert.equal(satelliteImageModel('grok-imagine'), 'grok-imagine-image-1.5');
assert.equal(satelliteImageModel('nano-banana'), 'gemini-3.1-flash-image');
assert.equal(satelliteImageModel('gpt-image-2'), 'gpt-image-2');
assert.equal(satelliteVideoModel('kling'), 'kling-v3');
assert.equal(satelliteVideoModel('seedance2'), 'seedance-2.0');
assert.equal(satelliteVideoModel('grok-imagine-video'), 'grok-imagine-video-1.5');
assert.equal(satelliteVideoModel('hailuo'), 'grok-imagine-video-1.5');
assert.equal(satelliteVideoModel('ofox'), 'grok-imagine-video-1.5');
delete process.env.SUB2API_BASE;
delete process.env.SUB2API_APP_CREDENTIAL;
assert.equal(satelliteCapsEnabled(), false);
console.log('aicut satellite mapping: origin, public image/video models hold');
