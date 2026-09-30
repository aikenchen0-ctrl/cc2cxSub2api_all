import { agentClient } from './client'
import type { LoginResponse } from './api'

export interface AgentPasskeyConfig {
  enabled: boolean
  configured: boolean
  supported_origin: boolean
  rp_id?: string
}

export interface PasskeyCredentialSummary {
  id: number
  name: string
  created_at: string
  last_used_at?: string
  backup: boolean
}

interface CeremonyOptionsResponse {
  session_token: string
  options: { publicKey: Record<string, unknown> }
}

function requirePasskeySupport(): void {
  if (!window.PublicKeyCredential || !navigator.credentials) {
    throw new Error('当前浏览器不支持 Passkey。')
  }
}

export function base64URLToBuffer(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = atob(padded)
  return Uint8Array.from(binary, (character) => character.charCodeAt(0)).buffer
}

export function bufferToBase64URL(value: ArrayBuffer | null): string | null {
  if (value === null) return null
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

export function creationOptionsFromJSON(value: Record<string, unknown>): PublicKeyCredentialCreationOptions {
  const options = { ...value } as Record<string, unknown>
  options.challenge = base64URLToBuffer(String(options.challenge))
  const user = { ...(options.user as Record<string, unknown>) }
  user.id = base64URLToBuffer(String(user.id))
  options.user = user
  if (Array.isArray(options.excludeCredentials)) {
    options.excludeCredentials = options.excludeCredentials.map((descriptor) => ({
      ...(descriptor as Record<string, unknown>),
      id: base64URLToBuffer(String((descriptor as Record<string, unknown>).id)),
    }))
  }
  return options as unknown as PublicKeyCredentialCreationOptions
}

export function requestOptionsFromJSON(value: Record<string, unknown>): PublicKeyCredentialRequestOptions {
  const options = { ...value } as Record<string, unknown>
  options.challenge = base64URLToBuffer(String(options.challenge))
  if (Array.isArray(options.allowCredentials)) {
    options.allowCredentials = options.allowCredentials.map((descriptor) => ({
      ...(descriptor as Record<string, unknown>),
      id: base64URLToBuffer(String((descriptor as Record<string, unknown>).id)),
    }))
  }
  return options as unknown as PublicKeyCredentialRequestOptions
}

function serializeRegistrationCredential(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response as AuthenticatorAttestationResponse
  return {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      attestationObject: bufferToBase64URL(response.attestationObject),
      clientDataJSON: bufferToBase64URL(response.clientDataJSON),
      transports: typeof response.getTransports === 'function' ? response.getTransports() : [],
    },
  }
}

function serializeAssertionCredential(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response as AuthenticatorAssertionResponse
  return {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      authenticatorData: bufferToBase64URL(response.authenticatorData),
      clientDataJSON: bufferToBase64URL(response.clientDataJSON),
      signature: bufferToBase64URL(response.signature),
      userHandle: bufferToBase64URL(response.userHandle),
    },
  }
}

async function login(): Promise<LoginResponse> {
  requirePasskeySupport()
  const begin = (await agentClient.post<CeremonyOptionsResponse>('/auth/passkey/login/begin', {})).data
  const credential = await navigator.credentials.get({
    publicKey: requestOptionsFromJSON(begin.options.publicKey),
  })
  if (!(credential instanceof PublicKeyCredential)) throw new Error('Passkey 登录已取消。')
  return (await agentClient.post<LoginResponse>('/auth/passkey/login/finish', {
    session_token: begin.session_token,
    credential: serializeAssertionCredential(credential),
  })).data
}

async function register(name: string, password: string): Promise<PasskeyCredentialSummary> {
  requirePasskeySupport()
  const begin = (await agentClient.post<CeremonyOptionsResponse>('/agent/passkeys/register/begin', { password })).data
  const credential = await navigator.credentials.create({
    publicKey: creationOptionsFromJSON(begin.options.publicKey),
  })
  if (!(credential instanceof PublicKeyCredential)) throw new Error('Passkey 创建已取消。')
  return (await agentClient.post<PasskeyCredentialSummary>('/agent/passkeys/register/finish', {
    session_token: begin.session_token,
    name,
    credential: serializeRegistrationCredential(credential),
  })).data
}

export const agentPasskeyAPI = {
  isSupported: () => Boolean(window.PublicKeyCredential && navigator.credentials),
  config: async () => (await agentClient.get<AgentPasskeyConfig>('/auth/passkey/config')).data,
  login,
  register,
  list: async () => (await agentClient.get<PasskeyCredentialSummary[]>('/agent/passkeys')).data,
  rename: async (id: number, name: string) => { await agentClient.patch(`/agent/passkeys/${id}`, { name }) },
  remove: async (id: number, password: string) => { await agentClient.delete(`/agent/passkeys/${id}`, { data: { password } }) },
}
