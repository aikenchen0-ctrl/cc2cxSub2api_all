import { mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { applyBranding, applyHomeSettings } from '@/agent/branding'
import { useAgentSession } from '@/agent/session'
import AgentHomeView from './AgentHomeView.vue'

describe('AgentHomeView homepage modes', () => {
  let wrapper: VueWrapper | undefined
  beforeEach(() => {
    setActivePinia(createPinia())
    applyBranding('本站品牌', '/tenant.svg', 'https://docs.tenant.example/')
    applyHomeSettings({})
  })
  afterEach(() => {
    wrapper?.unmount()
    applyBranding('AgentAPI', '/logo.svg')
    applyHomeSettings({})
  })
  function render() {
    wrapper = mount(AgentHomeView, { global: { stubs: {
      LzParticleScene: true,
      RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
    } } })
    return wrapper
  }

  it('retains the particle home by default and applies the tenant subtitle', () => {
    applyHomeSettings({ site_subtitle: '本站模型\n统一接入' })
    const page = render()
    expect(page.find('.lz-home').exists()).toBe(true)
    expect(page.get('h1').text()).toBe('本站品牌')
    expect(page.get('.lz-subtitle').text()).toBe('本站模型\n统一接入')
    expect(page.get('img').attributes('src')).toBe('/tenant.svg')
  })

  it.each([
    { label: 'guest', user: null, path: '/login' },
    { label: 'main admin without tenant role', user: { id: '7', role: 'admin', agent_admin: false }, path: '/dashboard' },
    { label: 'tenant admin', user: { id: '8', role: 'user', agent_admin: true }, path: '/admin/dashboard' },
  ])('copies compact homepage navigation for $label', ({ user, path }) => {
    useAgentSession().$patch({ user })
    applyHomeSettings({ compact_home_enabled: true, site_subtitle: '独立品牌副标题' })
    const page = render()
    expect(page.find('[data-testid="compact-home"]').exists()).toBe(true)
    expect(page.find('.lz-home').exists()).toBe(false)
    expect(page.get('main a').attributes('href')).toBe(path)
    expect(page.get('h1').text()).toBe('本站品牌')
    expect(page.text()).toContain('独立品牌副标题')
    expect(page.findAll('img').every(image => image.attributes('src') === '/tenant.svg')).toBe(true)
    expect(page.get('a[title="查看文档"]').attributes('href')).toBe('https://docs.tenant.example/')
    expect(page.find('a[href="/model-plaza"]').exists()).toBe(false)
    expect(page.find('a[href="/downloads"]').exists()).toBe(true)
  })

  it('prioritizes an external homepage over compact mode without granting same-origin access', () => {
    applyHomeSettings({ compact_home_enabled: true, home_content: ' https://tenant.example/home ' })
    const page = render()
    const frame = page.get('iframe')
    expect(frame.attributes('src')).toBe('https://tenant.example/home')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('sandbox')).not.toContain('allow-same-origin')
    expect(page.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('preserves custom HTML styles in an isolated frame and restores the selected home when cleared', async () => {
    const html = '<html><head><style>h1{color:red}</style></head><body><h1>本站首页</h1><script>alert(1)</script></body></html>'
    applyHomeSettings({ compact_home_enabled: true, home_content: html })
    const page = render()
    const frame = page.get('iframe')
    expect(frame.attributes('srcdoc')).toBe(html)
    expect(frame.attributes('sandbox')).not.toContain('allow-scripts')
    expect(frame.attributes('sandbox')).not.toContain('allow-same-origin')
    expect(page.find('script').exists()).toBe(false)

    applyHomeSettings({ compact_home_enabled: true, home_content: '' })
    await nextTick()
    expect(page.find('iframe').exists()).toBe(false)
    expect(page.find('[data-testid="compact-home"]').exists()).toBe(true)
  })

  it.each(['javascript:alert(1)', 'data:text/html,<script>alert(1)</script>', 'https://user:secret@example.com'])('does not embed invalid custom URL %s', (home_content) => {
    applyHomeSettings({ compact_home_enabled: true, home_content })
    const page = render()
    expect(page.find('iframe').exists()).toBe(false)
    expect(page.find('[data-testid="compact-home"]').exists()).toBe(true)
  })
})
