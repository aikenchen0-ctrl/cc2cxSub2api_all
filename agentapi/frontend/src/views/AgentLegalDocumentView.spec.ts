import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentLegalDocumentView from './AgentLegalDocumentView.vue'

describe('AgentLegalDocumentView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders tenant markdown using the copied legal layout and sanitizes unsafe HTML', async () => {
    vi.spyOn(agentAPI.contentPages, 'get').mockResolvedValue({
      id: 1, slug: 'terms', kind: 'legal', title: '本站条款',
      content: '# 欢迎\n\n<script>window.pwned = true</script><img src=x onerror="window.pwned=true">安全正文',
      status: 'active', sort_order: 0, created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:00Z',
    })
    const router = createRouter({ history: createMemoryHistory(), routes: [
      { path: '/home', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      { path: '/legal/:slug', component: AgentLegalDocumentView },
    ] })
    await router.push('/legal/terms')
    const wrapper = mount(AgentLegalDocumentView, { global: { plugins: [router] } })
    await flushPromises()

    expect(agentAPI.contentPages.get).toHaveBeenCalledWith('legal', 'terms')
    expect(wrapper.text()).toContain('本站条款')
    expect(wrapper.text()).toContain('安全正文')
    expect(wrapper.html()).not.toContain('<script')
    expect(wrapper.html()).not.toContain('onerror=')
  })
})
