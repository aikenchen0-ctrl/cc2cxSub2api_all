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

  it('keeps the copied model plaza public while model execution remains protected', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/model-plaza')

    expect(router.currentRoute.value.path).toBe('/model-plaza')
    expect(router.currentRoute.value.meta.requiresAuth).toBe(false)

    await router.push('/console?model=gpt-5.5')
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/console?model=gpt-5.5')
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

  it('keeps the copied batch image page behind the Agent session', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockRejectedValue({ status: 401, message: 'not signed in' })

    await router.push('/batch-image')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/batch-image')
  })

  it.each(['dashboard', 'agent-provisioning', 'ops', 'users', 'promo-codes', 'orders', 'orders/dashboard', 'orders/plans', 'subscriptions', 'channels', 'channels/pricing', 'satellite-billing', 'usage', 'audit-logs', 'announcements', 'content', 'backup', 'settings'])('rejects ordinary users at new admin route %s', async (section) => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    await router.push(`/admin/${section}`)
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('does not register main-site-only administrator modules', () => {
    const paths = router.getRoutes().map(route => route.path)
    for (const path of ['/admin/groups', '/admin/accounts', '/admin/plugins', '/admin/proxies', '/admin/redeem', '/admin/risk-control', '/admin/prompt-audit', '/admin/upstream-audit']) {
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

  it('redirects the legacy audit path to the main-site audit route', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })
    await router.push('/admin/audit')
    expect(router.currentRoute.value.path).toBe('/admin/audit-logs')
  })

  it('redirects the legacy model policy path to the copied main-site channel route', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'owner-1', agent_admin: true })
    await router.push('/admin/models')
    expect(router.currentRoute.value.path).toBe('/admin/channels')
  })

  it.each([
    ['/models', '/model-plaza'],
    ['/recharge', '/purchase'],
  ])('keeps old user route %s as a redirect to the main-site path', async (legacy, canonical) => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })
    await router.push(legacy)
    expect(router.currentRoute.value.path).toBe(canonical)
  })
})
