<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { agentAPI, type AgentContextResponse } from './api'
import { useAgentSession } from './session'
import { errorMessage } from './client'

const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const context = ref<AgentContextResponse | null>(null)
const error = ref('')
const dark = ref(false)

const signedIn = computed(() => session.isAuthenticated)
const admin = computed(() => session.isAgentAdmin)
const siteName = computed(() => context.value?.agent.site_name || 'AgentAPI')
const logo = computed(() => context.value?.agent.site_logo || '/logo.svg')

const links = computed(() => {
  const items = [
    { to: '/dashboard', label: 'Dashboard' },
    { to: '/recharge', label: 'Recharge' },
    { to: '/console', label: 'Model console' },
    { to: '/keys', label: 'API keys' },
    { to: '/usage', label: 'Usage' },
    { to: '/profile', label: 'Profile' },
  ]
  if (admin.value) items.splice(1, 0, { to: '/agent-admin', label: 'Agent console' })
  return items
})

async function loadContext(): Promise<void> {
  try { context.value = await agentAPI.getContext() } catch (err) { error.value = errorMessage(err, '') }
}

async function signOut(): Promise<void> {
  await session.logout()
  await router.replace('/home')
}

function toggleTheme(): void {
  dark.value = !dark.value
  document.documentElement.classList.toggle('dark', dark.value)
  sessionStorage.setItem('agentapi_theme', dark.value ? 'dark' : 'light')
}

function hideBrokenImage(event: Event): void {
  const target = event.target
  if (target instanceof HTMLImageElement) target.style.display = 'none'
}

onMounted(async () => {
  dark.value = sessionStorage.getItem('agentapi_theme') === 'dark'
  document.documentElement.classList.toggle('dark', dark.value)
  await session.checkAuth()
  await loadContext()
})
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <header class="sticky top-0 z-20 border-b border-gray-200/80 bg-white/95 backdrop-blur dark:border-dark-700 dark:bg-dark-900/95">
      <div class="mx-auto flex max-w-7xl items-center gap-5 px-4 py-3 sm:px-6">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3 font-semibold">
          <img :src="logo" alt="" class="h-9 w-9 rounded-lg object-cover" @error="hideBrokenImage">
          <span class="truncate">{{ siteName }}</span>
        </RouterLink>
        <nav v-if="signedIn" class="hidden min-w-0 flex-1 items-center gap-1 md:flex">
          <RouterLink v-for="link in links" :key="link.to" :to="link.to" class="rounded-lg px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white" :class="route.path === link.to ? 'bg-gray-100 font-medium text-gray-900 dark:bg-dark-800 dark:text-white' : ''">{{ link.label }}</RouterLink>
        </nav>
        <div class="ml-auto flex items-center gap-2">
          <button class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600" type="button" aria-label="Toggle theme" @click="toggleTheme">{{ dark ? '☀️' : '🌙' }}</button>
          <RouterLink v-if="!signedIn && route.path !== '/login'" to="/login" class="hidden rounded-lg px-3 py-2 text-sm md:inline">Sign in</RouterLink>
          <RouterLink v-if="!signedIn && route.path !== '/register'" to="/register" class="rounded-lg bg-primary-600 px-3 py-2 text-sm font-medium text-white">Create account</RouterLink>
          <button v-if="signedIn" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600" type="button" @click="signOut">Sign out</button>
        </div>
      </div>
      <div v-if="signedIn" class="mx-auto flex max-w-7xl gap-1 overflow-x-auto px-4 pb-2 md:hidden sm:px-6">
        <RouterLink v-for="link in links" :key="`mobile-${link.to}`" :to="link.to" class="whitespace-nowrap rounded-lg px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300">{{ link.label }}</RouterLink>
      </div>
    </header>
    <p v-if="error" class="mx-auto max-w-7xl px-4 pt-3 text-sm text-amber-700 dark:text-amber-300 sm:px-6">{{ error }}</p>
    <RouterView />
  </div>
</template>
