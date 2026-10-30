import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from './api'
import { applyBranding, applyHomeSettings, applyPageTitle, compactHomeEnabled, homeContent, loadBranding, siteContactInfo, siteDocURL, siteLogo, siteName, siteSubtitle } from './branding'

describe('instance branding', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.head.querySelectorAll('link[rel~="icon"]').forEach((item) => item.remove())
    document.title = ''
    applyPageTitle('')
    applyBranding('AgentAPI', '/logo.svg')
    applyHomeSettings({})
  })

  it('falls back to the session-bound tenant when public settings cannot resolve a shared host', async () => {
    vi.spyOn(agentAPI, 'getPublicSettings').mockRejectedValue({ status: 421, message: 'tenant not found' })
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue({
      agent: {
        agent_id: 'agent-a',
        domain: '',
        name: '内部标识',
        site_name: '会话站点',
        site_logo: '/session.svg',
        doc_url: 'https://docs.session.example.com/',
        contact_info: 'session@example.com',
        site_subtitle: '会话副标题',
        compact_home_enabled: true,
        home_content: '<h1>会话首页</h1>',
        status: 'active',
        billing_mode: 'user_upstream',
        main_balance_cents: 0,
        billing_status: 'ok',
        wallet_available_cents: 0,
        wallet_allocated_cents: 0,
      },
      authenticated: true,
      is_agent_admin: true,
    })

    applyPageTitle('系统设置')
    await loadBranding()

    expect(siteName.value).toBe('会话站点')
    expect(siteLogo.value).toBe('/session.svg')
    expect(siteDocURL.value).toBe('https://docs.session.example.com/')
    expect(siteContactInfo.value).toBe('session@example.com')
    expect(siteSubtitle.value).toBe('会话副标题')
    expect(compactHomeEnabled.value).toBe(true)
    expect(homeContent.value).toBe('<h1>会话首页</h1>')
    expect(document.title).toBe('系统设置 · 会话站点')
  })

  it('keeps the safe default branding when neither public settings nor tenant context resolves', async () => {
    applyBranding('旧站点', '/old.svg')
    vi.spyOn(agentAPI, 'getPublicSettings').mockRejectedValue({ status: 421, message: 'tenant not found' })
    vi.spyOn(agentAPI, 'getContext').mockRejectedValue({ status: 421, message: 'tenant not found' })

    await loadBranding()

    expect(siteName.value).toBe('AgentAPI')
    expect(siteLogo.value).toBe('/logo.svg')
  })

  it('loads the current instance title and favicon from same-origin settings', async () => {
    vi.spyOn(agentAPI, 'getPublicSettings').mockResolvedValue({
      site_name: '管理员 A 站点',
      site_logo: '/uploads/admin-a-logo.png',
      doc_url: 'https://docs.admin-a.example.com/start',
      contact_info: 'support@admin-a.example.com',
      site_subtitle: '管理员 A 的模型站',
      compact_home_enabled: true,
      home_content: '<h1>A 站</h1>',
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
    expect(siteDocURL.value).toBe('https://docs.admin-a.example.com/start')
    expect(siteContactInfo.value).toBe('support@admin-a.example.com')
    expect(siteSubtitle.value).toBe('管理员 A 的模型站')
    expect(compactHomeEnabled.value).toBe(true)
    expect(homeContent.value).toBe('<h1>A 站</h1>')
    expect(document.title).toBe('总览 · 管理员 A 站点')
    expect(document.querySelector<HTMLLinkElement>('link[rel~="icon"]')?.href).toBe(
      `${window.location.origin}/uploads/admin-a-logo.png`,
    )
  })

  it('drops unsafe public documentation URLs before rendering a link', () => {
    applyBranding('AgentAPI', '/logo.svg', 'javascript:alert(1)')
    expect(siteDocURL.value).toBe('')
  })

  it('resets omitted homepage settings when another tenant configuration is applied', () => {
    applyHomeSettings({ site_subtitle: 'A', compact_home_enabled: true, home_content: '<h1>A</h1>' })
    applyHomeSettings({})
    expect(siteSubtitle.value).toBe('AI API Gateway Platform')
    expect(compactHomeEnabled.value).toBe(false)
    expect(homeContent.value).toBe('')
  })

  it('keeps tenant contact information as bounded plain text', () => {
    applyBranding('AgentAPI', '/logo.svg', '', 'support@example.com')
    expect(siteContactInfo.value).toBe('support@example.com')

    applyBranding('AgentAPI', '/logo.svg', '', 'support@example.com\n<script>')
    expect(siteContactInfo.value).toBe('')
  })
})
