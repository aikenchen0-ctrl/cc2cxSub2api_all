import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from './api'
import { agentClient } from './client'

describe('AgentAPI browser boundary', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps profile and password changes on same-origin AgentAPI cookie routes', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { id: '42', email: 'user@example.com', username: 'User', can_edit: true },
    } as never)
    const put = vi.spyOn(agentClient, 'put').mockResolvedValue({
      data: { message: 'password changed; sign in again' },
    } as never)

    await expect(agentAPI.profile.get()).resolves.toMatchObject({ id: '42', can_edit: true })
    await agentAPI.profile.update('Updated User')
    await agentAPI.profile.changePassword('old-password', 'new-password')

    expect(get).toHaveBeenCalledWith('/agent/profile')
    expect(put).toHaveBeenNthCalledWith(1, '/agent/profile', { username: 'Updated User' })
    expect(put).toHaveBeenNthCalledWith(2, '/agent/password', {
      old_password: 'old-password',
      new_password: 'new-password',
    })
    expect(agentClient.defaults.withCredentials).toBe(true)
    expect(agentClient.defaults.headers.common.Authorization).toBeUndefined()
    expect(localStorage.getItem('access_token')).toBeNull()
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('keeps the AgentAPI key lifecycle on the local gateway', async () => {
    const create = vi.spyOn(agentClient, 'post').mockResolvedValue({
      data: { item: { id: 1, name: 'desktop', prefix: 'sk-abc', status: 'active' }, key: 'sk-secret' },
    } as never)

    const response = await agentAPI.keys.create('desktop')

    expect(response.key).toBe('sk-secret')
    expect(create).toHaveBeenCalledWith('/api-keys', { name: 'desktop' })
    expect(response.key).not.toMatch(/^sk-super-/)
  })

  it('requests a bounded usage page from the AgentAPI backend', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { items: [], total: 52, page: 2, page_size: 25 },
    } as never)

    await expect(agentAPI.getUsage(2, 25)).resolves.toMatchObject({ total: 52, page: 2, page_size: 25 })
    expect(get).toHaveBeenCalledWith('/agent/usage', { params: { page: 2, page_size: 25 } })
  })

  it('does not expose removed station-owner management clients', () => {
    const boundary = agentAPI as unknown as Record<string, unknown>
    for (const key of ['adminProvisioning', 'adminPromoCodes', 'adminBackups', 'adminSubscriptions', 'adminContentPages', 'getAdminChannels', 'getTasks', 'getUsers', 'getAuditEvents', 'setMappedUserStatus']) {
      expect(boundary[key]).toBeUndefined()
    }
  })

})
