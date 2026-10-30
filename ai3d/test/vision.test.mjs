import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { DEFAULT_SUB2API_RELAY_MODEL, managedRelayModel } from '../server/config.mjs'
import { normalizeVisionInsight } from '../server/providers/vision.mjs'

const ALLOWED_CATEGORIES = new Set(['artifact', 'road', 'vessel', 'aircraft', 'product', 'specimen'])

describe('视觉模型默认值', () => {
  it('托管模式默认使用可调度的 gpt-6-astra，而不是已下线的 gpt-5.5', () => {
    assert.equal(DEFAULT_SUB2API_RELAY_MODEL, 'gpt-6-astra')

    const previous = process.env.SUB2API_RELAY_MODEL
    try {
      delete process.env.SUB2API_RELAY_MODEL
      assert.equal(managedRelayModel(), 'gpt-6-astra')

      // 部署方仍可显式覆盖。
      process.env.SUB2API_RELAY_MODEL = 'gpt-5.6-sol'
      assert.equal(managedRelayModel(), 'gpt-5.6-sol')

      // 空白值不应把模型名清空，否则上游会收到空 model 字段。
      process.env.SUB2API_RELAY_MODEL = '   '
      assert.equal(managedRelayModel(), 'gpt-6-astra')
    } finally {
      if (previous === undefined) delete process.env.SUB2API_RELAY_MODEL
      else process.env.SUB2API_RELAY_MODEL = previous
    }
  })
})

describe('视觉分类兜底', () => {
  it('把不在允许清单里的 categoryId 交给模型自己的描述推断', () => {
    // 实测 gpt-6-astra 会返回 "platform" 这类自造分类。
    const insight = normalizeVisionInsight({
      objectName: 'Chinese aircraft carrier',
      categoryId: 'platform',
      description: 'A grey naval vessel with a flat flight deck and island superstructure.',
      tags: ['warship'],
    })

    assert.equal(insight.categoryId, 'vessel')
    assert.equal(insight.categoryLabel, 'Naval Vessel')
  })

  it('按中文描述识别生物标本', () => {
    const insight = normalizeVisionInsight({
      objectName: '上皮细胞',
      categoryId: 'unknown',
      description: '一只上皮细胞，表面光滑，细胞核清晰可见。',
    })

    assert.equal(insight.categoryId, 'specimen')
  })

  it('描述不足以判断时回落到 product', () => {
    const insight = normalizeVisionInsight({
      objectName: 'Rounded square slab',
      categoryId: 'platform',
      description: 'Two rounded-square slabs separated vertically.',
    })

    assert.equal(insight.categoryId, 'product')
    assert.equal(insight.categoryLabel, 'Product Object')
  })

  it('允许清单内的分类不被描述文本改写', () => {
    const insight = normalizeVisionInsight({
      objectName: 'Bronze mask',
      categoryId: 'artifact',
      // 描述里出现 vessel 关键词也不应抢走明确给出的分类。
      description: 'A ritual vessel used in ancient ceremonies.',
      categoryLabel: 'Museum Artifact',
    })

    assert.equal(insight.categoryId, 'artifact')
  })

  it('任何输入都只会得到允许清单里的分类', () => {
    const samples = [
      { categoryId: 'platform' },
      { categoryId: '', description: 'a supercar on a road' },
      { categoryId: 'nonsense', objectName: '飞机模型' },
      {},
    ]

    for (const sample of samples) {
      const insight = normalizeVisionInsight(sample)
      assert.ok(ALLOWED_CATEGORIES.has(insight.categoryId), `unexpected category: ${insight.categoryId}`)
    }
  })
})
