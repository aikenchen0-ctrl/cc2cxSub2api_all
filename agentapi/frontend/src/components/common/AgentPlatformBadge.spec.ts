import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentPlatformBadge from './AgentPlatformBadge.vue'

describe('AgentPlatformBadge', () => {
  it.each([
    ['openai', 'OpenAI', 'text-green-600'],
    ['anthropic', 'Anthropic', 'text-orange-600'],
    ['gemini', 'Gemini', 'text-blue-600'],
    ['deepseek', 'DeepSeek', 'text-teal-600'],
  ])('renders the main-site %s platform treatment', (platform, label, colorClass) => {
    const wrapper = mount(AgentPlatformBadge, { props: { platform } })
    expect(wrapper.attributes('data-platform')).toBe(platform)
    expect(wrapper.text()).toBe(label)
    expect(wrapper.classes()).toContain(colorClass)
    expect(wrapper.find('svg').exists()).toBe(true)
  })

  it('preserves an unknown backend label with a neutral fallback', () => {
    const wrapper = mount(AgentPlatformBadge, { props: { platform: 'vendor-x' } })
    expect(wrapper.text()).toBe('vendor-x')
    expect(wrapper.classes()).toContain('text-slate-600')
  })
})
