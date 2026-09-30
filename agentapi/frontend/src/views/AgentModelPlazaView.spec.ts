import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentModelPlazaView from './AgentModelPlazaView.vue'

async function render() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/models', component: AgentModelPlazaView },
      { path: '/console', component: { template: '<div>console</div>' } },
      { path: '/home', component: { template: '<div>home</div>' } },
      { path: '/downloads', component: { template: '<div>downloads</div>' } },
      { path: '/login', component: { template: '<div>login</div>' } },
    ],
  })
  await router.push('/models')
  await router.isReady()
  const wrapper = mount(AgentModelPlazaView, { global: { plugins: [createPinia(), router] } })
  await flushPromises()
  return wrapper
}

afterEach(() => vi.restoreAllMocks())

describe('AgentModelPlazaView', () => {
  it('renders the enabled tenant catalog in main-site style groups', async () => {
    const list = vi.spyOn(agentAPI.model, 'publicList').mockResolvedValue([
      'gpt-5.5', 'gpt-image-2', 'claude-sonnet-4', 'grok-imagine-video-1.5',
    ])
    const wrapper = await render()

    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('模型广场')
    expect(wrapper.text()).toContain('登录')
    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('Anthropic')
    expect(wrapper.text()).toContain('费用以主站实际账单为准')
    expect(wrapper.findAll('article')).toHaveLength(4)
    const imageLink = wrapper.findAll('article').find((item) => item.text().includes('gpt-image-2'))?.get('a')
    expect(imageLink?.attributes('href')).toContain('/console?model=gpt-image-2')
    expect(imageLink?.attributes('href')).toContain('mode=image')
  })

  it('filters the tenant catalog by capability and model name', async () => {
    vi.spyOn(agentAPI.model, 'publicList').mockResolvedValue(['gpt-5.5', 'gpt-image-2', 'grok-imagine-video-1.5'])
    const wrapper = await render()

    const imageButton = wrapper.findAll('button').find((button) => button.text() === '图片')
    await imageButton?.trigger('click')
    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).not.toContain('gpt-5.5')

    await wrapper.get('input[aria-label="搜索模型"]').setValue('missing')
    expect(wrapper.text()).toContain('没有符合当前筛选条件的模型')

    await wrapper.get('button[aria-label="清空模型搜索"]').trigger('click')
    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.text()).toContain('gpt-image-2')
  })

  it('shows a retryable failure without stale model data', async () => {
    vi.spyOn(agentAPI.model, 'publicList').mockRejectedValue(new Error('network unavailable'))
    const wrapper = await render()
    expect(wrapper.get('[role="alert"]').text()).toContain('加载可用模型失败')
    expect(wrapper.findAll('article')).toHaveLength(0)
  })
})
