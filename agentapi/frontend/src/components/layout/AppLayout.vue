<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { setAgentOnboardingReplay } from '@/agent/onboarding'
import { useAgentSession } from '@/agent/session'
import { useAgentOnboardingTour } from '@/composables/useAgentOnboardingTour'
import AgentAppHeader from './AgentAppHeader.vue'
import AgentAppSidebar from './AgentAppSidebar.vue'

const route = useRoute()
const session = useAgentSession()
const sidebarCollapsed = ref(sessionStorage.getItem('agentapi_sidebar_collapsed') === 'true')
const mobileOpen = ref(false)
const onboarding = useAgentOnboardingTour(session.isAgentAdmin ? 'admin' : 'user')
let onboardingTimer: number | null = null

function setCollapsed(value: boolean): void {
  sidebarCollapsed.value = value
  sessionStorage.setItem('agentapi_sidebar_collapsed', String(value))
}

watch(() => route.fullPath, () => { mobileOpen.value = false })

onMounted(() => {
  setAgentOnboardingReplay(onboarding.replayTour)
  if (import.meta.env.MODE !== 'test') {
    onboardingTimer = window.setTimeout(() => { void onboarding.startTour() }, 350)
  }
})

onBeforeUnmount(() => {
  if (onboardingTimer !== null) window.clearTimeout(onboardingTimer)
  setAgentOnboardingReplay(null)
  onboarding.disposeTour()
})
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white" @keydown.esc="mobileOpen = false">
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>
    <AgentAppSidebar
      :collapsed="sidebarCollapsed"
      :mobile-open="mobileOpen"
      @update:collapsed="setCollapsed"
      @close-mobile="mobileOpen = false"
    />
    <div class="relative min-h-screen transition-all duration-300" :class="sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64'">
      <AgentAppHeader @open-mobile="mobileOpen = true" />
      <main class="agent-content p-4 md:p-6 lg:p-8"><slot /></main>
    </div>
  </div>
</template>

<style scoped>
.agent-content :deep(> main) { max-width: none; margin: 0; padding: 0; min-height: 0; }
</style>
