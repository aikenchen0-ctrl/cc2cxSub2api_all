<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AgentNavigationProgress from '@/components/common/AgentNavigationProgress.vue'
import AgentToastHost from '@/components/common/AgentToastHost.vue'
import { useAgentSession } from './session'
const route = useRoute()
const session = useAgentSession()
const usesPanel = computed(() => session.isAuthenticated && route.meta.requiresAuth !== false)
const usesAuthLayout = computed(() => route.meta.authLayout === true)
</script>
<template>
  <AgentNavigationProgress />
  <AgentToastHost />
  <AppLayout v-if="usesPanel"><RouterView :key="route.path" /></AppLayout>
  <AuthLayout v-else-if="usesAuthLayout"><RouterView /></AuthLayout>
  <RouterView v-else />
</template>
