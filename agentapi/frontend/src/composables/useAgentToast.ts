import { readonly, ref, type Ref } from 'vue'

export type AgentToastType = 'success' | 'error' | 'warning' | 'info'

export interface AgentToast {
  id: string
  type: AgentToastType
  message: string
  title?: string
  duration?: number
}

export interface AgentToastOptions {
  title?: string
  duration?: number
}

const toasts = ref<AgentToast[]>([])
const timers = new Map<string, ReturnType<typeof setTimeout>>()
let toastID = 0

function cleanText(value: string, limit: number): string {
  return value.trim().slice(0, limit)
}

function removeToast(id: string): void {
  const timer = timers.get(id)
  if (timer !== undefined) {
    clearTimeout(timer)
    timers.delete(id)
  }
  const index = toasts.value.findIndex((toast) => toast.id === id)
  if (index >= 0) toasts.value.splice(index, 1)
}

function clearAllToasts(): void {
  for (const timer of timers.values()) clearTimeout(timer)
  timers.clear()
  toasts.value = []
}

function showToast(type: AgentToastType, message: string, options: AgentToastOptions = {}): string {
  const id = `agent-toast-${++toastID}`
  const duration = options.duration === undefined
    ? undefined
    : Math.max(0, Math.min(options.duration, 60_000))
  const toast: AgentToast = {
    id,
    type,
    message: cleanText(message, 1_000) || '操作已完成。',
    title: options.title ? cleanText(options.title, 120) : undefined,
    duration,
  }
  toasts.value.push(toast)
  if (duration !== undefined && duration > 0) {
    timers.set(id, setTimeout(() => removeToast(id), duration))
  }
  return id
}

function showSuccess(message: string, options: AgentToastOptions = {}): string {
  return showToast('success', message, { duration: 3_000, ...options })
}

function showError(message: string, options: AgentToastOptions = {}): string {
  return showToast('error', message, { duration: 5_000, ...options })
}

function showWarning(message: string, options: AgentToastOptions = {}): string {
  return showToast('warning', message, { duration: 4_000, ...options })
}

function showInfo(message: string, options: AgentToastOptions = {}): string {
  return showToast('info', message, { duration: 3_000, ...options })
}

export function useAgentToast(): {
  toasts: Readonly<Ref<readonly AgentToast[]>>
  showToast: typeof showToast
  showSuccess: typeof showSuccess
  showError: typeof showError
  showWarning: typeof showWarning
  showInfo: typeof showInfo
  removeToast: typeof removeToast
  clearAllToasts: typeof clearAllToasts
} {
  return {
    toasts: readonly(toasts),
    showToast,
    showSuccess,
    showError,
    showWarning,
    showInfo,
    removeToast,
    clearAllToasts,
  }
}

export function resetAgentToastsForTests(): void {
  clearAllToasts()
  toastID = 0
}
