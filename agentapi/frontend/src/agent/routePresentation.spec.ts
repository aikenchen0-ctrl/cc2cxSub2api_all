import { describe, expect, it } from 'vitest'
import { agentPanelRouteDescriptions, resolveAgentRoutePresentation } from './routePresentation'

describe('agent route presentation', () => {
  it('covers every primary user and tenant-admin panel route with a specific description', () => {
    const expected = [
      '/dashboard',
      '/keys', '/usage', '/purchase', '/orders',
      '/admin/dashboard',
      '/admin/users', '/admin/usage',
      '/admin/announcements', '/admin/settings',
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

  it('uses a safe fallback for routes outside the retained console', () => {
    expect(resolveAgentRoutePresentation('/unknown', '控制台').description).toBe('使用本站提供的用户控制台能力。')
  })
})
