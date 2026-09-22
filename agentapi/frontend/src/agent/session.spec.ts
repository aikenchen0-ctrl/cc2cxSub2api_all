import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from './api'
import { useAgentSession } from './session'

describe('AgentAPI session', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hydrates identity through /auth/me without a browser token', async () => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue({
      id: 'main-42',
      email: 'user@example.com',
      agent_admin: false,
    })

    const session = useAgentSession()
    await session.checkAuth()

    expect(session.isAuthenticated).toBe(true)
    expect(session.user?.id).toBe('main-42')
    expect(session.isAgentAdmin).toBe(false)
    expect(localStorage.getItem('access_token')).toBeNull()
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('clears the local identity after cookie logout', async () => {
    vi.spyOn(agentAPI.auth, 'login').mockResolvedValue({
      access_token: '',
      refresh_token: '',
      expires_in: 259200,
      token_type: 'Cookie',
      user: { id: 'admin-1', agent_admin: true },
    })
    const logout = vi.spyOn(agentAPI.auth, 'logout').mockResolvedValue()

    const session = useAgentSession()
    await session.login('admin@example.com', 'password')
    expect(session.isAgentAdmin).toBe(true)

    await session.logout()

    expect(logout).toHaveBeenCalledOnce()
    expect(session.isAuthenticated).toBe(false)
  })
})
