import { readonly, ref } from 'vue'

export type AgentOnboardingRole = 'admin' | 'user'

const STORAGE_PREFIX = 'agentapi_onboarding_v1'

type ReplayHandler = () => void

let replayHandler: ReplayHandler | null = null
const replayAvailable = ref(false)

export const agentOnboardingReplayAvailable = readonly(replayAvailable)

export function agentOnboardingStorageKey(role: AgentOnboardingRole): string {
  return `${STORAGE_PREFIX}_${role}_complete`
}

export function isAgentOnboardingComplete(role: AgentOnboardingRole): boolean {
  try {
    return localStorage.getItem(agentOnboardingStorageKey(role)) === 'true'
  } catch {
    return false
  }
}

export function markAgentOnboardingComplete(role: AgentOnboardingRole): void {
  try {
    localStorage.setItem(agentOnboardingStorageKey(role), 'true')
  } catch {
    // The guide remains usable when storage is disabled.
  }
}

export function clearAgentOnboardingComplete(role: AgentOnboardingRole): void {
  try {
    localStorage.removeItem(agentOnboardingStorageKey(role))
  } catch {
    // A replay still works for the current page when storage is disabled.
  }
}

export function setAgentOnboardingReplay(handler: ReplayHandler | null): void {
  replayHandler = handler
  replayAvailable.value = handler !== null
}

export function replayAgentOnboarding(): boolean {
  if (!replayHandler) return false
  replayHandler()
  return true
}
