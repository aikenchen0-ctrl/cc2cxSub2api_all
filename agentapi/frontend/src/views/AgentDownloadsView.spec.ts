import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { applyBranding } from '@/agent/branding'
import { downloadOptions } from '@/utils/ccswitch-downloads'
import AgentDownloadsView from './AgentDownloadsView.vue'

describe('AgentDownloadsView', () => {
  afterEach(() => applyBranding('AgentAPI', '/logo.svg'))

  it('uses tenant branding while retaining the main-site download layout', () => {
    applyBranding('本站客户端下载', '/tenant-logo.svg')
    const wrapper = mount(AgentDownloadsView, {
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
    })

    expect(wrapper.text()).toContain('本站客户端下载')
    expect(wrapper.text()).toContain('为当前设备推荐')
    expect(wrapper.text()).toContain('Windows 安装程序（EXE）')
    expect(wrapper.get('img').attributes('src')).toBe('/tenant-logo.svg')
    expect(wrapper.get('header a').attributes('href')).toBe('/home')
  })

  it('only exposes the copied public release links with safe new-tab attributes', () => {
    const wrapper = mount(AgentDownloadsView, {
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
    })
    const releaseLinks = wrapper.findAll('a[target="_blank"]')

    expect(releaseLinks).toHaveLength(downloadOptions.length + 1)
    expect(releaseLinks.every((link) => link.attributes('rel') === 'noopener noreferrer')).toBe(true)
    expect(releaseLinks.every((link) => link.attributes('href')?.startsWith('https://github.com/aikenchen0-ctrl/cc2cx/releases/'))).toBe(true)
  })
})
