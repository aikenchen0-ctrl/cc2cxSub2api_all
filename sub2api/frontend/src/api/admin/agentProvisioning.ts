import { apiClient, buildApiUrl } from '../client'
import { ADMIN_UI_REQUEST_HEADER } from '../adminUIRequest'

export type AgentProvisioningStatus =
  | 'pending'
  | 'provisioning'
  | 'active'
  | 'suspended'
  | 'failed'
  | 'revoked'

export interface AgentProvisioningAgent {
  agent_id: string
  slug: string
  domain: string
  display_name: string
  owner_main_user_id: number
  plan_id: string
  brand_name: string
  logo_url?: string | null
  status: AgentProvisioningStatus
  current_step: string
  recent_error?: string
  can_retry: boolean
  domain_status: string
  request_id: string
  created_at: string
  updated_at: string
}

export interface AgentProvisioningListResponse {
  items: AgentProvisioningAgent[]
  total: number
  page: number
  page_size: number
}

export interface AgentProvisioningListParams {
  page: number
  page_size: number
  q?: string
  status?: AgentProvisioningStatus | ''
}

export interface CreateAgentProvisioningRequest {
  requested_slug: string
  display_name: string
  owner_main_user_id: string
  plan_id: string
  brand: {
    name: string
    logo_url?: string
  }
  domain_mode: 'platform_subdomain'
}

function idempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export async function listAgentProvisioningAgents(
  params: AgentProvisioningListParams,
  signal?: AbortSignal
): Promise<AgentProvisioningListResponse> {
  const { data } = await apiClient.get<AgentProvisioningListResponse>('/agent-provisioning/agents', {
    params,
    signal
  })
  return data
}

export async function createAgentProvisioningAgent(
  request: CreateAgentProvisioningRequest
): Promise<AgentProvisioningAgent> {
  const { data } = await apiClient.post<AgentProvisioningAgent>(
    '/agent-provisioning/agents',
    request,
    { headers: { 'Idempotency-Key': idempotencyKey() } }
  )
  return data
}

async function transitionAgent(
  agentId: string,
  operation: 'activate' | 'suspend' | 'resume' | 'retry' | 'revoke',
  confirmAgentId?: string
): Promise<AgentProvisioningAgent> {
  const { data } = await apiClient.post<AgentProvisioningAgent>(
    `/agent-provisioning/agents/${encodeURIComponent(agentId)}/${operation}`,
    operation === 'activate'
      ? { confirm_agent_id: confirmAgentId, readiness_confirmed: true }
      : operation === 'revoke' ? { confirm_agent_id: confirmAgentId } : {},
    { headers: { 'Idempotency-Key': idempotencyKey() } }
  )
  return data
}

export const activateAgentProvisioningAgent = (agentId: string) => transitionAgent(agentId, 'activate', agentId)
export const suspendAgentProvisioningAgent = (agentId: string) => transitionAgent(agentId, 'suspend')
export const resumeAgentProvisioningAgent = (agentId: string) => transitionAgent(agentId, 'resume')
export const retryAgentProvisioningAgent = (agentId: string) => transitionAgent(agentId, 'retry')
export const revokeAgentProvisioningAgent = (agentId: string, confirmAgentId: string) =>
  transitionAgent(agentId, 'revoke', confirmAgentId)

export async function streamAgentProvisioningUpdates(options: {
  signal: AbortSignal
  onResync: () => void
  onAgent: (agentId: string) => void
}): Promise<void> {
  const headers = new Headers({
    Accept: 'text/event-stream',
    [ADMIN_UI_REQUEST_HEADER]: '1'
  })
  const token = typeof localStorage !== 'undefined' ? localStorage.getItem('auth_token') : null
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const response = await fetch(buildApiUrl('/agent-provisioning/agents/stream'), {
    method: 'GET',
    headers,
    credentials: 'include',
    signal: options.signal
  })
  if (!response.ok || !response.body) {
    throw new Error(`Agent provisioning stream failed (${response.status})`)
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let eventName = 'message'
  let dataLines: string[] = []

  const dispatch = () => {
    if (dataLines.length === 0) {
      eventName = 'message'
      return
    }
    const data = dataLines.join('\n')
    if (eventName === 'resync') {
      options.onResync()
    } else if (eventName === 'agent') {
      try {
        const update = JSON.parse(data) as { agent_id?: unknown }
        if (typeof update.agent_id === 'string' && update.agent_id.trim() !== '') {
          options.onAgent(update.agent_id)
        }
      } catch {
        // Ignore malformed events and let the next resync restore the view.
      }
    }
    eventName = 'message'
    dataLines = []
  }

  try {
    while (!options.signal.aborted) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let boundary = buffer.indexOf('\n')
      while (boundary >= 0) {
        const line = buffer.slice(0, boundary).replace(/\r$/, '')
        buffer = buffer.slice(boundary + 1)
        if (line === '') {
          dispatch()
        } else if (line.startsWith('event:')) {
          eventName = line.slice(6).trim()
        } else if (line.startsWith('data:')) {
          dataLines.push(line.slice(5).replace(/^ /, ''))
        }
        boundary = buffer.indexOf('\n')
      }
    }
  } finally {
    reader.releaseLock()
  }
}

const agentProvisioningAPI = {
  list: listAgentProvisioningAgents,
  create: createAgentProvisioningAgent,
  activate: activateAgentProvisioningAgent,
  suspend: suspendAgentProvisioningAgent,
  resume: resumeAgentProvisioningAgent,
  retry: retryAgentProvisioningAgent,
  revoke: revokeAgentProvisioningAgent,
  stream: streamAgentProvisioningUpdates
}

export default agentProvisioningAPI
