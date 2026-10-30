<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <button
      type="button"
      class="absolute right-4 top-4 z-20 flex h-10 w-10 items-center justify-center rounded-full border border-primary-200/80 bg-white/75 text-accent-600 shadow-sm backdrop-blur transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-primary-500/20 dark:bg-dark-900/75 dark:text-dark-300 dark:hover:bg-primary-500/10 dark:hover:text-primary-300"
      :title="isDark ? '切换浅色模式' : '切换深色模式'"
      aria-label="切换主题"
      @click="toggleTheme"
    >
      <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
    </button>

    <!-- Background -->
    <div
      class="absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
    ></div>

    <!-- Decorative Elements -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Gradient Orbs -->
      <div
        class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary-300/10 blur-3xl"
      ></div>

      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(201,159,55,0.05)_1px,transparent_1px),linear-gradient(90deg,rgba(201,159,55,0.05)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl shadow-lg shadow-primary-500/30"
          >
            <img :src="siteLogo" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="agent-auth-content card-glass rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// Sub2API AuthLayout presentation, with AgentAPI-local branding.
import { siteLogo, siteName, siteSubtitle } from '@/agent/branding'
import { isDark, toggleTheme } from '@/agent/theme'
import Icon from '@/components/icons/Icon.vue'
const settingsLoaded = true
const currentYear = new Date().getFullYear()
</script>

<style scoped>
.agent-auth-content :deep(> main) { padding: 0; margin: 0; min-height: 0; max-width: none; }
.agent-auth-content :deep(> main > section) { background: transparent; padding: 0; border: 0; box-shadow: none; }

.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
