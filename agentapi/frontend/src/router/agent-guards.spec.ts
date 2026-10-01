import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import router from './index'

describe('AgentAPI route guards', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('redirects an unauthenticated browser to login', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/agent-admin')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/admin/dashboard')
  })

  it('keeps the copied downloads page public', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/downloads')

    expect(router.currentRoute.value.path).toBe('/downloads')
    expect(router.currentRoute.value.meta.requiresAuth).toBe(false)
  })

  it('keeps the copied Key usage page public', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/key-usage')

    expect(router.currentRoute.value.path).toBe('/key-usage')
    expect(router.currentRoute.value.meta.requiresAuth).toBe(false)
  })

  it('keeps the copied WeChat payment callback public without exposing the purchase page', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/auth/wechat/payment/callback')

    expect(router.currentRoute.value.path).toBe('/auth/wechat/payment/callback')
    expect(router.currentRoute.value.meta.requiresAuth).toBe(false)
  })

  it('keeps the OAuth binding callback behind the existing Agent session', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/auth/oauth/binding/callback')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/auth/oauth/binding/callback')
  })

  it.each(['/login', '/register', '/email-verify', '/forgot-password', '/reset-password'])('uses the shared main-style authentication layout at %s', async (path) => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push(path)

    expect(router.currentRoute.value.path).toBe(path)
    expect(router.currentRoute.value.meta.authLayout).toBe(true)
  })

  it.each([
    '/auth/callback',
    '/auth/oauth/callback',
    '/auth/linuxdo/callback',
    '/auth/wechat/callback',
    '/auth/dingtalk/callback',
    '/auth/dingtalk/email-completion',
    '/auth/oidc/callback',
  ])('keeps the copied OAuth compatibility path public at %s', async (path) => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })
    await router.push(`${path}?code=private-code`)
    await router.isReady()
    expect(router.currentRoute.value.meta.requiresAuth).toBe(false)
    expect(router.currentRoute.value.meta.authLayout).toBe(true)
  })

  it('maps the main-site setup path to tenant provisioning and still requires an agent admin', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    await router.push('/setup')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('keeps non-admin users out of the Agent console', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })

    await router.push('/agent-admin')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('redirects the legacy Agent console entry to the copied main-site dashboard', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })

    await router.push('/agent-admin')

    expect(router.currentRoute.value.path).toBe('/admin/dashboard')
  })

  it.each(['users', 'usage', 'announcements', 'settings'])('rejects ordinary users at retained admin route %s', async (section) => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    await router.push('/home')
    await router.push(`/admin/${section}`)
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('does not register main-site-only administrator modules', () => {
    const paths = router.getRoutes().map(route => route.path)
    for (const path of ['/model-plaza', '/models', '/console', '/batch-image', '/admin/groups', '/admin/accounts', '/admin/orders', '/admin/orders/dashboard', '/admin/affiliates/invites', '/admin/agent-provisioning', '/admin/ops', '/admin/proxies', '/admin/promo-codes', '/admin/plugins', '/admin/audit', '/admin/audit-logs', '/admin/redeem', '/admin/risk-control', '/admin/prompt-audit', '/admin/upstream-audit', '/admin/subscriptions', '/admin/orders/plans', '/admin/channels', '/admin/channels/pricing', '/admin/satellite-billing', '/admin/content', '/admin/backup', '/affiliate', '/profile']) {
      expect(paths).not.toContain(path)
    }
  })

  it('keeps every concrete admin page behind authentication and the agent-admin guard', () => {
    const adminPages = router.getRoutes().filter((route) => (
      route.path.startsWith('/admin/') && Boolean(route.components?.default)
    ))

    expect(adminPages.length).toBeGreaterThan(0)
    for (const route of adminPages) {
      expect(route.meta.requiresAuth, `${route.path} must require authentication`).toBe(true)
      expect(route.meta.requiresAgentAdmin, `${route.path} must require the current agent admin`).toBe(true)
    }
  })

  it('keeps the copied main-site /admin entry path', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })
    await router.push('/admin')
    expect(router.currentRoute.value.path).toBe('/admin/dashboard')
  })

  it('leaves the removed audit module at the not-found page', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })
    await router.push('/admin/audit')
    expect(router.currentRoute.value.name).toBe('NotFound')
  })

  it.each(['/model-plaza', '/models', '/console', '/batch-image', '/admin/models', '/admin/ops', '/admin/subscriptions', '/admin/channels', '/admin/channels/pricing', '/admin/satellite-billing', '/admin/content', '/admin/backup'])('leaves removed route %s at the not-found page', async (path) => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })
    await router.push(path)
    expect(router.currentRoute.value.name).toBe('NotFound')
  })

  it('keeps the recharge compatibility redirect', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    await router.push('/recharge')
    expect(router.currentRoute.value.path).toBe('/purchase')
  })
})
