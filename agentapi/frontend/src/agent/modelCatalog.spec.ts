import { describe, expect, it } from 'vitest'
import { groupCatalogModels, modelCapability, modelProvider } from './modelCatalog'

describe('agent model catalog adapter', () => {
  it('classifies public models without inventing upstream metadata', () => {
    expect(modelProvider('gpt-5.5')).toBe('OpenAI')
    expect(modelProvider('claude-sonnet-4')).toBe('Anthropic')
    expect(modelProvider('gemini-3.1-flash-image')).toBe('Google')
    expect(modelCapability('gpt-5.5')).toBe('text')
    expect(modelCapability('gpt-image-2')).toBe('image')
    expect(modelCapability('grok-imagine-video-1.5')).toBe('video')
  })

  it('groups and sorts the tenant-visible names only', () => {
    const groups = groupCatalogModels(['claude-sonnet-4', 'gpt-image-2', 'gpt-5.5', ''])
    expect(groups.map((group) => group.provider)).toEqual(['Anthropic', 'OpenAI'])
    expect(groups[1].models.map((model) => model.name)).toEqual(['gpt-5.5', 'gpt-image-2'])
  })
})
