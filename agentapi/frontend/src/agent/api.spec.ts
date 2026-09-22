import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from './api'
import { agentClient } from './client'

describe('AgentAPI browser boundary', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the HttpOnly cookie session for model calls', async () => {
    const post = vi.spyOn(agentClient, 'post').mockResolvedValue({
      data: { id: 'chat-1', choices: [{ message: { role: 'assistant', content: 'hello' } }] },
    } as never)

    const messages = [{ role: 'user', content: 'hello' }]
    await expect(agentAPI.model.chat('gpt-5.5', messages)).resolves.toMatchObject({
      id: 'chat-1',
    })

    expect(agentClient.defaults.withCredentials).toBe(true)
    expect(agentClient.defaults.headers.common.Authorization).toBeUndefined()
    expect(post).toHaveBeenCalledWith('/v1/chat/completions', {
      model: 'gpt-5.5',
      messages,
    }, { baseURL: '' })
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('keeps the AgentAPI key lifecycle on the local gateway', async () => {
    const create = vi.spyOn(agentClient, 'post').mockResolvedValue({
      data: { item: { id: 1, name: 'desktop', prefix: 'sk-agent-abc', status: 'active' }, key: 'sk-agent-secret' },
    } as never)

    const response = await agentAPI.keys.create('desktop')

    expect(response.key).toBe('sk-agent-secret')
    expect(create).toHaveBeenCalledWith('/api-keys', { name: 'desktop' })
    expect(response.key).not.toMatch(/^sk-super-/)
  })
})
