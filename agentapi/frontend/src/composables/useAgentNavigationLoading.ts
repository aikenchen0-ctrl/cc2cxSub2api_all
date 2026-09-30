import { computed, readonly, ref } from 'vue'

export function createAgentNavigationLoading() {
  const navigating = ref(false)
  const visible = ref(false)
  let showTimer: ReturnType<typeof setTimeout> | null = null

  const ANTI_FLICKER_DELAY = 100

  function clearShowTimer(): void {
    if (showTimer !== null) {
      clearTimeout(showTimer)
      showTimer = null
    }
  }

  function startNavigation(): void {
    clearShowTimer()
    navigating.value = true
    showTimer = setTimeout(() => {
      showTimer = null
      if (navigating.value) visible.value = true
    }, ANTI_FLICKER_DELAY)
  }

  function endNavigation(): void {
    clearShowTimer()
    navigating.value = false
    visible.value = false
  }

  function resetState(): void {
    endNavigation()
  }

  return {
    isLoading: computed(() => visible.value),
    isNavigating: readonly(navigating),
    startNavigation,
    endNavigation,
    resetState,
    ANTI_FLICKER_DELAY,
  }
}

let navigationLoadingInstance: ReturnType<typeof createAgentNavigationLoading> | null = null

export function useAgentNavigationLoadingState() {
  if (!navigationLoadingInstance) navigationLoadingInstance = createAgentNavigationLoading()
  return navigationLoadingInstance
}

export function resetAgentNavigationLoadingForTests(): void {
  navigationLoadingInstance?.resetState()
  navigationLoadingInstance = null
}
