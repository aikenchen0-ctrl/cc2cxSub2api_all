import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, buildApiUrl } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  buildApiUrl: vi.fn((path: string) => `/api/v1${path}`)
}))

vi.mock('../client', () => ({ apiClient: { get, post }, buildApiUrl }))

import {
  activateAgentProvisioningAgent,
  createAgentProvisioningAgent,
  listAgentProvisioningAgents,
  revokeAgentProvisioningAgent,
  retryAgentProvisioningAgent,
  streamAgentProvisioningUpdates
} from '@/api/admin/agentProvisioning'

describe('agent provisioning admin API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads filtered pages from the Sub2API management API', async () => {
    const result = { items: [], total: 0, page: 2, page_size: 10 }
    get.mockResolvedValue({ data: result })
    const signal = new AbortController().signal

    await expect(listAgentProvisioningAgents({ page: 2, page_size: 10, q: 'alpha', status: 'pending' }, signal))
      .resolves.toBe(result)
    expect(get).toHaveBeenCalledWith('/agent-provisioning/agents', {
      params: { page: 2, page_size: 10, q: 'alpha', status: 'pending' },
      signal
    })
  })

  it('creates, activates after readiness confirmation, and revokes records with idempotency keys', async () => {
    const agent = { agent_id: 'agt_123', status: 'pending' }
    post.mockResolvedValue({ data: agent })
    await createAgentProvisioningAgent({
      requested_slug: 'alpha',
      display_name: 'Alpha',
      owner_main_user_id: 'u_42',
      plan_id: 'starter',
      brand: { name: 'Alpha' },
      domain_mode: 'platform_subdomain'
    })
    expect(post).toHaveBeenNthCalledWith(1, '/agent-provisioning/agents', expect.objectContaining({ requested_slug: 'alpha' }), {
      headers: { 'Idempotency-Key': expect.any(String) }
    })

    await activateAgentProvisioningAgent('agt_123')
    expect(post).toHaveBeenNthCalledWith(2, '/agent-provisioning/agents/agt_123/activate', {
      confirm_agent_id: 'agt_123',
      readiness_confirmed: true
    }, { headers: { 'Idempotency-Key': expect.any(String) } })

    await revokeAgentProvisioningAgent('agt_123', 'agt_123')
    expect(post).toHaveBeenNthCalledWith(3, '/agent-provisioning/agents/agt_123/revoke', {
      confirm_agent_id: 'agt_123'
    }, { headers: { 'Idempotency-Key': expect.any(String) } })
  })

  it('retries a failed deployment through the idempotent control API', async () => {
    post.mockResolvedValue({ data: { agent_id: 'agt_123', status: 'pending' } })
    await retryAgentProvisioningAgent('agt_123')
    expect(post).toHaveBeenCalledWith('/agent-provisioning/agents/agt_123/retry', {}, {
      headers: { 'Idempotency-Key': expect.any(String) }
    })
  })

  it('dispatches resync and ID-only change hints from the authenticated SSE stream', async () => {
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('retry: 3000\n\nevent: resync\ndata: {}\n\nevent: agent\ndata: {"agent_id":"agt_123"}\n\n'))
        controller.close()
      }
    })
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, body })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('localStorage', { getItem: () => 'admin-token' })
    const onResync = vi.fn()
    const onAgent = vi.fn()
    const controller = new AbortController()

    await streamAgentProvisioningUpdates({ signal: controller.signal, onResync, onAgent })

    expect(buildApiUrl).toHaveBeenCalledWith('/agent-provisioning/agents/stream')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/agent-provisioning/agents/stream', expect.objectContaining({
      credentials: 'include',
      headers: expect.any(Headers),
      signal: controller.signal
    }))
    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(new Headers(request.headers).get('Authorization')).toBe('Bearer admin-token')
    expect(onResync).toHaveBeenCalledOnce()
    expect(onAgent).toHaveBeenCalledWith('agt_123')
    vi.unstubAllGlobals()
  })
})
