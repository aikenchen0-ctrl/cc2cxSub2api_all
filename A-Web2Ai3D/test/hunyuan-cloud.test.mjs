import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { buildHunyuanCloudSubmitBody } from '../server/providers/hunyuan-cloud.mjs'

const IMAGE_BASE64 = 'data:image/png;base64,ZmFrZS1pbWFnZQ=='

describe('混元云端草图请求体', () => {
  it('草图模式固定使用 3.0，并同时发送线稿和用户提示词', () => {
    const body = buildHunyuanCloudSubmitBody({
      imageDataUrl: IMAGE_BASE64,
      generateType: 'Sketch',
      model: '3.1',
      prompt: '一只上皮细胞，光滑表面，细胞核清晰',
    })

    assert.equal(body.Model, '3.0')
    assert.equal(body.GenerateType, 'Sketch')
    assert.equal(body.ImageBase64, IMAGE_BASE64)
    assert.equal(body.Prompt, '一只上皮细胞，光滑表面，细胞核清晰')
  })

  it('草图模式未填写提示词时使用中文默认模板', () => {
    const body = buildHunyuanCloudSubmitBody({
      imageDataUrl: IMAGE_BASE64,
      generateType: 'Sketch',
      prompt: '   ',
    })

    assert.equal(body.Model, '3.0')
    assert.equal(body.GenerateType, 'Sketch')
    assert.equal(typeof body.Prompt, 'string')
    assert.ok(body.Prompt.length > 0)
    assert.match(body.Prompt, /[\u3400-\u9fff]/)
  })

  it('普通照片模式不发送提示词字段', () => {
    const body = buildHunyuanCloudSubmitBody({
      imageDataUrl: IMAGE_BASE64,
      generateType: 'Normal',
      model: '3.1',
      prompt: '这段内容不应发送',
    })

    assert.equal(body.Model, '3.1')
    assert.equal(body.GenerateType, 'Normal')
    assert.equal(body.ImageBase64, IMAGE_BASE64)
    assert.equal('Prompt' in body, false)
  })
})
