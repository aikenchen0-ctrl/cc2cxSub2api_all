import { describe, expect, it } from 'vitest'
import { agentPanelRouteDescriptions, resolveAgentRoutePresentation } from './routePresentation'

describe('agent route presentation', () => {
  it('covers every primary user and tenant-admin panel route with a specific description', () => {
    const expected = [
      '/dashboard', '/model-plaza', '/console', '/batch-image',
      '/keys', '/usage', '/purchase', '/orders', '/affiliate', '/profile',
      '/admin/dashboard', '/admin/agent-provisioning', '/admin/ops', '/admin/users',
      '/admin/promo-codes', '/admin/subscriptions',
      '/admin/affiliates/invites', '/admin/affiliates/rebates', '/admin/affiliates/transfers',
      '/admin/orders/dashboard', '/admin/orders', '/admin/orders/plans', '/admin/channels',
      '/admin/channels/pricing', '/admin/satellite-billing', '/admin/usage',
      '/admin/audit-logs', '/admin/announcements', '/admin/content', '/admin/backup', '/admin/settings',
    ]

    expect(Object.keys(agentPanelRouteDescriptions).sort()).toEqual(expect.arrayContaining(expected.sort()))
    for (const path of expected) {
      const presentation = resolveAgentRoutePresentation(path, '页面标题')
      expect(presentation.title).toBe('页面标题')
      expect(presentation.description.length).toBeGreaterThan(12)
      expect(presentation.description).not.toContain('SuperKey')
      expect(presentation.description).not.toContain('管理员 Key')
    }
  })

  it('uses a tenant-content description for dynamic custom pages and a safe fallback elsewhere', () => {
    expect(resolveAgentRoutePresentation('/custom/terms', '服务条款')).toEqual({
      title: '服务条款',
      description: '查看当前代理站管理员发布的租户专属内容。',
    })
    expect(resolveAgentRoutePresentation('/unknown', '控制台').description).toBe('使用当前代理站提供的用户控制台能力。')
  })
})
