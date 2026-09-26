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

  it('loads model-console options from the AgentAPI policy-filtered catalog', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { data: [{ id: 'gpt-image-2' }, { id: '' }, {}] },
    } as never)

    await expect(agentAPI.model.list()).resolves.toEqual(['gpt-image-2'])
    expect(get).toHaveBeenCalledWith('/v1/models', { baseURL: '' })
  })

  it('sends image generation and multipart edits through the same-origin AgentAPI session', async () => {
    const post = vi.spyOn(agentClient, 'post').mockResolvedValue({
      data: { data: [{ url: 'https://media.example/image.png' }] },
    } as never)
    const image = new File(['pixels'], 'reference.png', { type: 'image/png' })

    await agentAPI.model.generateImage('gpt-image-2', 'a paper crane')
    await agentAPI.model.editImage('gpt-image-2', 'make the background blue', image)

    expect(post).toHaveBeenNthCalledWith(1, '/v1/images/generations', {
      model: 'gpt-image-2', prompt: 'a paper crane',
    }, { baseURL: '', timeout: 180_000 })
    const [, body, config] = post.mock.calls[1]
    expect(post.mock.calls[1][0]).toBe('/v1/images/edits')
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('model')).toBe('gpt-image-2')
    expect((body as FormData).get('prompt')).toBe('make the background blue')
    expect((body as FormData).get('image')).toBe(image)
    expect(config).toMatchObject({ baseURL: '', timeout: 180_000, headers: { 'Content-Type': 'multipart/form-data' } })
    expect(agentClient.defaults.withCredentials).toBe(true)
    expect(agentClient.defaults.headers.common.Authorization).toBeUndefined()
  })

  it('creates and polls video tasks through the owner-scoped AgentAPI routes', async () => {
    const post = vi.spyOn(agentClient, 'post').mockResolvedValue({ data: { request_id: 'task/1', status: 'queued' } } as never)
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({ data: { id: 'task/1', status: 'done' } } as never)

    await expect(agentAPI.model.createVideo('grok-imagine-video-1.5', 'a quiet lake')).resolves.toMatchObject({ request_id: 'task/1' })
    await expect(agentAPI.model.getVideo('task/1')).resolves.toMatchObject({ status: 'done' })

    expect(post).toHaveBeenCalledWith('/v1/videos', {
      model: 'grok-imagine-video-1.5', prompt: 'a quiet lake', duration: 6, aspect_ratio: '16:9', resolution: '480p',
    }, { baseURL: '', timeout: 60_000 })
    expect(get).toHaveBeenCalledWith('/v1/videos/task%2F1', { baseURL: '', timeout: 60_000 })
    expect(agentClient.defaults.withCredentials).toBe(true)
    expect(agentClient.defaults.headers.common.Authorization).toBeUndefined()
  })

  it('creates, edits, and polls asynchronous image tasks through AgentAPI', async () => {
    const post = vi.spyOn(agentClient, 'post').mockResolvedValue({ data: { id: 'imgtask/1', status: 'processing' } } as never)
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({ data: { id: 'imgtask/1', status: 'completed' } } as never)
    const image = new File(['pixels'], 'reference.png', { type: 'image/png' })

    await expect(agentAPI.model.generateImageAsync('gpt-image-2', 'a paper crane')).resolves.toMatchObject({ id: 'imgtask/1' })
    await agentAPI.model.editImageAsync('gpt-image-2', 'make it blue', image)
    await expect(agentAPI.model.getImageTask('imgtask/1')).resolves.toMatchObject({ status: 'completed' })

    expect(post).toHaveBeenNthCalledWith(1, '/v1/images/generations/async', {
      model: 'gpt-image-2', prompt: 'a paper crane',
    }, { baseURL: '', timeout: 60_000 })
    const [, body, config] = post.mock.calls[1]
    expect(post.mock.calls[1][0]).toBe('/v1/images/edits/async')
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('model')).toBe('gpt-image-2')
    expect((body as FormData).get('image')).toBe(image)
    expect(config).toMatchObject({ baseURL: '', timeout: 60_000, headers: { 'Content-Type': 'multipart/form-data' } })
    expect(get).toHaveBeenCalledWith('/v1/images/tasks/imgtask%2F1', { baseURL: '', timeout: 60_000 })
    expect(agentClient.defaults.withCredentials).toBe(true)
    expect(agentClient.defaults.headers.common.Authorization).toBeUndefined()
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

  it('requests a bounded usage page from the AgentAPI backend', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { items: [], total: 52, page: 2, page_size: 25 },
    } as never)

    await expect(agentAPI.getUsage(2, 25)).resolves.toMatchObject({ total: 52, page: 2, page_size: 25 })
    expect(get).toHaveBeenCalledWith('/agent/usage', { params: { page: 2, page_size: 25 } })
  })

  it('requests the current user\'s persisted image and video task history', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { items: [{ task_type: 'image', task_id: 'task-1', request_id: 'request-1', status: 'processing' }], total: 1, page: 1, page_size: 25 },
    } as never)

    await expect(agentAPI.getTasks(1, 25)).resolves.toMatchObject({ total: 1, page: 1, page_size: 25 })
    expect(get).toHaveBeenCalledWith('/agent/tasks', { params: { page: 1, page_size: 25 } })
  })

  it('requests a bounded mapped-user page from the AgentAPI backend', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { items: [], total: 1200, page: 3, page_size: 50 },
    } as never)

    await expect(agentAPI.getUsers(3, 50, ' alice ')).resolves.toMatchObject({ total: 1200, page: 3, page_size: 50 })
    expect(get).toHaveBeenCalledWith('/agent/users', { params: { page: 3, page_size: 50, q: 'alice' } })
  })

  it('reads and updates model access through the Agent administrator API', async () => {
    const get = vi.spyOn(agentClient, 'get').mockResolvedValue({
      data: { catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: true },
    } as never)
    const put = vi.spyOn(agentClient, 'put').mockResolvedValue({
      data: { catalog: ['gpt-5.5'], enabled: [], customized: true },
    } as never)

    await expect(agentAPI.getAdminModelPolicy()).resolves.toMatchObject({ enabled: ['gpt-5.5'] })
    await expect(agentAPI.updateAdminModelPolicy([])).resolves.toMatchObject({ enabled: [] })
    expect(get).toHaveBeenCalledWith('/agent/admin/model-policy')
    expect(put).toHaveBeenCalledWith('/agent/admin/model-policy', { enabled: [] })
  })

  it('updates only the mapped user status through the Agent administrator API', async () => {
    const patch = vi.spyOn(agentClient, 'patch').mockResolvedValue({
      data: { agent_id: 'agent-1', main_user_id: 'user-1', status: 'disabled', balance_cents: 25 },
    } as never)

    await expect(agentAPI.setMappedUserStatus('user-1', 'disabled')).resolves.toMatchObject({ status: 'disabled' })
    expect(patch).toHaveBeenCalledWith('/agent/admin/users/user-1/status', { status: 'disabled' })
  })
})
