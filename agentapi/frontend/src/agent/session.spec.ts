import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentUser, type AuthenticatedResponse } from './api'
import { useAgentSession } from './session'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function authenticated(id: string): AuthenticatedResponse {
  return { access_token: '', refresh_token: '', expires_in: 259200, token_type: 'Cookie', user: { id, agent_admin: false } }
}

describe('AgentAPI session', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each([undefined, null, {}, { id: '' }, { id: 0 }, { id: 1.5 }])('rejects an invalid identity from me: %j', async (invalid) => {
    vi.spyOn(agentAPI.auth, 'me').mockResolvedValue(invalid as AgentUser)
    const session = useAgentSession()
    await session.checkAuth()
    expect(session.user).toBeNull()
    expect(session.isAuthenticated).toBe(false)
  })

  it.each(['login2FA', 'refresh'] as const)('rejects a missing user from %s without retaining authentication', async (method) => {
    vi.spyOn(agentAPI.auth, 'login').mockResolvedValue(authenticated('43'))
    vi.spyOn(agentAPI.auth, method).mockResolvedValue({ ...authenticated('43'), user: undefined } as unknown as AuthenticatedResponse)
    const session = useAgentSession()
    await session.login('user@example.com', 'password')
    const operation = method === 'refresh' ? session.refresh() : session.login2FA('proof', '123456')
    await expect(operation).rejects.toThrow('有效用户信息')
    expect(session.user).toBeNull()
    expect(session.isAuthenticated).toBe(false)
    expect(session.busy).toBe(false)
  })

  it('ignores an identity lookup that finishes after logout', async () => {
    const pending = deferred<AgentUser>()
    vi.spyOn(agentAPI.auth, 'me').mockReturnValue(pending.promise)
    vi.spyOn(agentAPI.auth, 'logout').mockResolvedValue()
    const session = useAgentSession()
    const checking = session.checkAuth()
    await session.logout()
    pending.resolve({ id: 'old-owner', agent_admin: true })
    await checking
    expect(session.user).toBeNull()
    expect(session.isAgentAdmin).toBe(false)
  })

  it('ignores a stale lookup failure after a newer login', async () => {
    const pending = deferred<AgentUser>()
    vi.spyOn(agentAPI.auth, 'me').mockReturnValue(pending.promise)
    vi.spyOn(agentAPI.auth, 'login').mockResolvedValue(authenticated('new-user'))
    const session = useAgentSession()
    const checking = session.checkAuth()
    await session.login('new@example.com', 'password')
    pending.reject(new Error('old lookup failed'))
    await checking
    expect(session.user?.id).toBe('new-user')
  })

  it('rejects a refresh result after local identity is cleared', async () => {
    const pending = deferred<AuthenticatedResponse>()
    vi.spyOn(agentAPI.auth, 'refresh').mockReturnValue(pending.promise)
    const session = useAgentSession()
    const refreshing = session.refresh()
    const rejected = expect(refreshing).rejects.toThrow('会话已变更')
    session.clear()
    pending.resolve(authenticated('old-user'))
    await rejected
    expect(session.user).toBeNull()
  })

  it('does not let an old login end the busy state of a newer login', async () => {
    const old = deferred<AuthenticatedResponse>()
    const current = deferred<AuthenticatedResponse>()
    vi.spyOn(agentAPI.auth, 'login').mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const session = useAgentSession()
    const first = session.login('old@example.com', 'password')
    const rejected = expect(first).rejects.toThrow('会话已变更')
    const second = session.login('new@example.com', 'password')
    old.resolve(authenticated('old-user'))
    await rejected
    expect(session.busy).toBe(true)
    expect(session.user).toBeNull()
    current.resolve(authenticated('new-user'))
    await second
    expect(session.busy).toBe(false)
    expect(session.user?.id).toBe('new-user')
  })

  it.each(['register', 'login2FA'] as const)('rejects a delayed %s result after clear', async (method) => {
    const pending = deferred<AuthenticatedResponse>()
    vi.spyOn(agentAPI.auth, method).mockReturnValue(pending.promise)
    const session = useAgentSession()
    const operation = method === 'register'
      ? session.register('new@example.com', 'password')
      : session.login2FA('temporary-proof', '123456')
    const rejected = expect(operation).rejects.toThrow('会话已变更')
    session.clear()
    pending.resolve(authenticated('old-user'))
    await rejected
    expect(session.user).toBeNull()
    expect(session.busy).toBe(false)
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
