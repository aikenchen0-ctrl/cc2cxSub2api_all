import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { agentAPI, type AgentUser, type LoginResponse } from './api'
import { errorMessage } from './client'

export const useAgentSession = defineStore('agent-session', () => {
  const user = ref<AgentUser | null>(null)
  const initialized = ref(false)
  const busy = ref(false)
  const isAuthenticated = computed(() => user.value !== null)
  const isAgentAdmin = computed(() => user.value?.agent_admin === true)

  async function checkAuth(): Promise<void> {
    try {
      user.value = await agentAPI.auth.me()
    } catch {
      user.value = null
    } finally {
      initialized.value = true
    }
  }

  async function login(email: string, password: string): Promise<LoginResponse> {
    busy.value = true
    try {
      const response = await agentAPI.auth.login(email, password)
      if (!response.requires_2fa && response.user) user.value = response.user
      return response
    } finally {
      busy.value = false
    }
  }

  async function login2FA(tempToken: string, totpCode: string): Promise<AgentUser> {
    busy.value = true
    try {
      const response = await agentAPI.auth.login2FA(tempToken, totpCode)
      user.value = response.user
      return response.user
    } finally {
      busy.value = false
    }
  }

  async function register(email: string, password: string, username?: string): Promise<AgentUser> {
    busy.value = true
    try {
      const response = await agentAPI.auth.register(email, password, username)
      user.value = response.user
      return response.user
    } finally {
      busy.value = false
    }
  }

  async function refresh(): Promise<AgentUser> {
    const response = await agentAPI.auth.refresh()
    user.value = response.user
    return response.user
  }

  async function logout(): Promise<void> {
    try { await agentAPI.auth.logout() } finally { user.value = null }
  }

  function clear(): void { user.value = null }

  return { user, initialized, busy, isAuthenticated, isAgentAdmin, checkAuth, login, login2FA, register, refresh, logout, clear, errorMessage }
})

