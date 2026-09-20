import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { getGenerationRequestOptions, getProviderPlan } from '../src/services/modelApi.js'

describe('前端混元生成模式映射', () => {
  it('将草图提示模式映射到混元 3.0 Sketch 且保留中文提示词', () => {
    assert.deepEqual(getGenerationRequestOptions('hunyuan-sketch', '画一个带细胞核的上皮细胞'), {
      provider: 'hunyuan',
      model: '3.0',
      generateType: 'Sketch',
      prompt: '画一个带细胞核的上皮细胞',
    })
    assert.deepEqual(getProviderPlan('hunyuan-sketch'), ['hunyuan'])
  })

  it('草图提示为空时使用中文默认词，普通照片模式不带提示词', () => {
    const sketch = getGenerationRequestOptions('hunyuan-sketch', '  ')
    const photo = getGenerationRequestOptions('hunyuan')

    assert.match(sketch.prompt, /[\u3400-\u9fff]/)
    assert.equal(sketch.model, '3.0')
    assert.equal(sketch.generateType, 'Sketch')
    assert.deepEqual(photo, { provider: 'hunyuan', model: '3.1', generateType: 'Normal', prompt: '' })
  })
})
