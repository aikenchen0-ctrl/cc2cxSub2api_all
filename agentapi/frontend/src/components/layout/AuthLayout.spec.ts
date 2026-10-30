import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { applyBranding, applyHomeSettings } from '@/agent/branding'
import AuthLayout from './AuthLayout.vue'

describe('AuthLayout', () => {
  afterEach(() => {
    applyBranding('AgentAPI', '/logo.svg')
    applyHomeSettings({})
  })

  it('uses the active tenant subtitle instead of a fixed English fallback', () => {
    applyBranding('Example Site', '/branding/logos/example.png?v=1')
    applyHomeSettings({ site_subtitle: '当前站点的登录副标题' })

    const wrapper = mount(AuthLayout, { slots: { default: '<div>login form</div>' } })

    expect(wrapper.text()).toContain('Example Site')
    expect(wrapper.text()).toContain('当前站点的登录副标题')
    expect(wrapper.text()).not.toContain('Subscription to API Conversion Platform')
    wrapper.unmount()
  })
})
