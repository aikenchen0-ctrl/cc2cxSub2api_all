import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { agentAPI, type AgentUser, type LoginResponse } from './api'
import { agentPasskeyAPI } from './passkeys'
import { errorMessage } from './client'

function validUser(value: unknown): value is AgentUser {
  if (!value || typeof value !== 'object') return false
  const id = (value as Partial<AgentUser>).id
  return typeof id === 'string' ? id.trim().length > 0 : typeof id === 'number' && Number.isSafeInteger(id) && id > 0
}

export const useAgentSession = defineStore('agent-session', () => {
  const user = ref<AgentUser | null>(null)
  const initialized = ref(false)
  const busy = ref(false)
  let identityVersion = 0
  const isAuthenticated = computed(() => validUser(user.value))
  const isAgentAdmin = computed(() => user.value?.agent_admin === true)

  async function checkAuth(): Promise<void> {
    const version = identityVersion
    try {
      const current = await agentAPI.auth.me()
      if (version === identityVersion) user.value = validUser(current) ? current : null
    } catch {
      if (version === identityVersion) user.value = null
    } finally {
      initialized.value = true
    }
  }

  async function login(email: string, password: string): Promise<LoginResponse> {
    const version = ++identityVersion
    busy.value = true
    try {
      const response = await agentAPI.auth.login(email, password)
      if (version !== identityVersion) throw new Error('会话已变更，请重新登录。')
      user.value = !response.requires_2fa && validUser(response.user) ? response.user : null
      return response
    } finally {
      if (version === identityVersion) busy.value = false
    }
  }

  async function login2FA(tempToken: string, totpCode: string): Promise<AgentUser> {
    const version = ++identityVersion
    busy.value = true
    try {
      const response = await agentAPI.auth.login2FA(tempToken, totpCode)
      if (version !== identityVersion) throw new Error('会话已变更，请重新登录。')
      if (!validUser(response.user)) {
        user.value = null
        throw new Error('登录响应缺少有效用户信息，请重新登录。')
      }
      user.value = response.user
      return response.user
    } finally {
      if (version === identityVersion) busy.value = false
    }
  }

  async function loginWithPasskey(): Promise<AgentUser> {
    const version = ++identityVersion
    busy.value = true
    try {
      const response = await agentPasskeyAPI.login()
      if (version !== identityVersion) throw new Error('会话已变更，请重新登录。')
      if (!validUser(response.user)) {
        user.value = null
        throw new Error('登录响应缺少有效用户信息，请重新登录。')
      }
      user.value = response.user
      return response.user
    } finally {
      if (version === identityVersion) busy.value = false
    }
  }

  async function register(email: string, password: string, username?: string, affiliateCode?: string, verifyCode?: string): Promise<LoginResponse> {
    const version = ++identityVersion
    busy.value = true
    try {
      const response = verifyCode
        ? await agentAPI.auth.register(email, password, username, affiliateCode, verifyCode)
        : await agentAPI.auth.register(email, password, username, affiliateCode)
      if (version !== identityVersion) throw new Error('会话已变更，请重新登录。')
      user.value = !response.requires_2fa && validUser(response.user) ? response.user : null
      return response
    } finally {
      if (version === identityVersion) busy.value = false
    }
  }

  async function refresh(): Promise<AgentUser> {
    const version = identityVersion
    const response = await agentAPI.auth.refresh()
    if (version !== identityVersion) throw new Error('会话已变更，请重新登录。')
    if (!validUser(response.user)) {
      user.value = null
      throw new Error('续期响应缺少有效用户信息，请重新登录。')
    }
    user.value = response.user
    return response.user
  }

  async function logout(): Promise<void> {
    clear()
    await agentAPI.auth.logout()
  }

  function clear(): void {
    identityVersion++
    user.value = null
    busy.value = false
  }

  return { user, initialized, busy, isAuthenticated, isAgentAdmin, checkAuth, login, login2FA, loginWithPasskey, register, refresh, logout, clear, errorMessage }
})
