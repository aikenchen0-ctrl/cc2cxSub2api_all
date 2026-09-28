import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from './api'
import { applyBranding, applyPageTitle, loadBranding, siteLogo, siteName } from './branding'

describe('instance branding', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.head.querySelectorAll('link[rel~="icon"]').forEach((item) => item.remove())
    document.title = ''
    applyBranding('AgentAPI', '/logo.svg')
  })

  it('loads the current instance title and favicon from same-origin settings', async () => {
    vi.spyOn(agentAPI, 'getPublicSettings').mockResolvedValue({
      site_name: '管理员 A 站点',
      site_logo: '/uploads/admin-a-logo.png',
      agent_name: '管理员 A',
      registration_enabled: true,
      payment_enabled: false,
      payment_provider: 'manual',
      payment_currency: 'CNY',
      payment_min_amount_cents: 100,
      payment_max_amount_cents: 100000,
    })

    await loadBranding()
    applyPageTitle('总览')

    expect(siteName.value).toBe('管理员 A 站点')
    expect(siteLogo.value).toBe('/uploads/admin-a-logo.png')
    expect(document.title).toBe('总览 · 管理员 A 站点')
    expect(document.querySelector<HTMLLinkElement>('link[rel~="icon"]')?.href).toBe(
      `${window.location.origin}/uploads/admin-a-logo.png`,
    )
  })
})
