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
    expect(router.currentRoute.value.query.redirect).toBe('/agent-admin')
  })

  it('keeps non-admin users out of the Agent console', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({ id: 'user-1', agent_admin: false })

    await router.push('/agent-admin')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })
})
