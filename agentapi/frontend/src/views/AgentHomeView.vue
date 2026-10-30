<script setup lang="ts">
import { computed } from 'vue'
import { useAgentSession } from '@/agent/session'
import { compactHomeEnabled, homeContent, siteDocURL, siteName, siteLogo, siteSubtitle } from '@/agent/branding'
import { isDark, toggleTheme } from '@/agent/theme'
import Icon from '@/components/icons/Icon.vue'
import ReferenceLanding from '@/components/home/ReferenceLanding.vue'

const session = useAgentSession()
const isAuthenticated = computed(() => session.isAuthenticated)
const dashboardPath = computed(() => session.isAgentAdmin ? '/admin/dashboard' : '/dashboard')
const customHomeURL = computed(() => {
  try {
    const url = new URL(homeContent.value)
    return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password ? url.href : ''
  } catch { return '' }
})
const customHomeHTML = computed(() => homeContent.value.startsWith('<') ? homeContent.value : '')
const hasHomeContent = computed(() => Boolean(customHomeURL.value || customHomeHTML.value))
const currentYear = new Date().getFullYear()
</script>

<template>
  <div v-if="hasHomeContent" class="min-h-screen" data-testid="custom-home">
    <iframe v-if="customHomeURL" :src="customHomeURL" :title="`${siteName} 首页`" class="h-screen w-full border-0" sandbox="allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation" referrerpolicy="no-referrer" allowfullscreen />
    <iframe v-else :srcdoc="customHomeHTML" :title="`${siteName} 首页`" class="h-screen w-full border-0" sandbox="allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation" referrerpolicy="no-referrer" />
  </div>

  <div v-else-if="compactHomeEnabled" data-testid="compact-home" class="relative flex min-h-screen flex-col overflow-hidden bg-accent-50 text-accent-950 dark:bg-dark-950 dark:text-white">
    <div class="pointer-events-none absolute inset-0 bg-mesh-gradient dark:opacity-70"></div>
    <div class="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(179,138,43,0.045)_1px,transparent_1px),linear-gradient(90deg,rgba(179,138,43,0.045)_1px,transparent_1px)] bg-[size:64px_64px] [mask-image:linear-gradient(to_bottom,black,transparent_78%)] dark:bg-[linear-gradient(rgba(226,194,115,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(226,194,115,0.035)_1px,transparent_1px)]"></div>
    <header class="relative z-10 border-b border-primary-200/60 bg-white/75 backdrop-blur-xl dark:border-primary-500/10 dark:bg-dark-950/75">
      <nav class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-3.5 sm:px-6">
        <div class="flex min-w-0 items-center gap-3">
          <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-primary-200/80 bg-white shadow-sm shadow-primary-500/10 dark:border-primary-500/20 dark:bg-dark-900">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-8 w-8 rounded-lg object-contain">
          </span>
          <span class="truncate text-base font-semibold tracking-tight">{{ siteName }}</span>
        </div>
        <div class="flex flex-wrap items-center gap-1.5 sm:gap-2">
          <a v-if="siteDocURL" :href="siteDocURL" target="_blank" rel="noopener noreferrer" class="flex h-10 w-10 items-center justify-center rounded-xl text-accent-500 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-dark-400 dark:hover:bg-primary-500/10 dark:hover:text-primary-300" title="查看文档"><Icon name="book" size="md" /></a>
          <button type="button" class="flex h-10 w-10 items-center justify-center rounded-xl text-accent-500 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-dark-400 dark:hover:bg-primary-500/10 dark:hover:text-primary-300" :title="isDark ? '切换浅色模式' : '切换深色模式'" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" size="md" /></button>
          <router-link to="/downloads" class="flex h-10 items-center rounded-xl px-2.5 text-sm text-accent-500 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-dark-400 dark:hover:bg-primary-500/10 dark:hover:text-primary-300">客户端下载</router-link>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="inline-flex min-h-10 items-center justify-center rounded-xl border border-primary-300/80 bg-primary-50 px-4 py-2 text-sm font-semibold text-primary-800 transition-colors hover:border-primary-400 hover:bg-primary-100 dark:border-primary-500/25 dark:bg-primary-500/10 dark:text-primary-200 dark:hover:bg-primary-500/15">{{ isAuthenticated ? '进入控制台' : '登录' }}</router-link>
        </div>
      </nav>
    </header>
    <main class="relative z-10 flex flex-1 items-center justify-center px-4 py-16 text-center sm:px-6">
      <div class="w-full max-w-3xl">
        <span class="inline-flex items-center gap-2 rounded-full border border-primary-300/70 bg-white/70 px-3.5 py-1.5 text-xs font-semibold tracking-[0.18em] text-primary-700 shadow-sm dark:border-primary-500/25 dark:bg-primary-500/10 dark:text-primary-300">
          <span class="h-1.5 w-1.5 rounded-full bg-primary-500"></span>
          UNIFIED AI ACCESS
        </span>
        <div class="relative mx-auto mt-8 h-24 w-24">
          <div class="absolute inset-0 rounded-[1.75rem] bg-primary-400/20 blur-xl"></div>
          <div class="relative flex h-24 w-24 items-center justify-center rounded-[1.75rem] border border-primary-200 bg-white p-3 shadow-glass dark:border-primary-500/25 dark:bg-dark-900">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full rounded-2xl object-contain">
          </div>
        </div>
        <h1 class="mt-6 break-words text-3xl font-bold tracking-tight md:text-5xl">{{ siteName }}</h1>
        <p class="mx-auto mt-4 max-w-xl whitespace-pre-wrap break-words text-base leading-7 text-accent-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="btn btn-primary btn-lg min-w-36">{{ isAuthenticated ? '进入控制台' : '登录' }}</router-link>
          <router-link to="/downloads" class="btn btn-secondary btn-lg min-w-36">客户端下载</router-link>
        </div>
        <div class="mt-8 flex flex-wrap items-center justify-center gap-x-6 gap-y-2 text-xs text-accent-500 dark:text-dark-400">
          <span class="inline-flex items-center gap-1.5"><span class="h-1 w-1 rounded-full bg-primary-500"></span>统一 API 入口</span>
          <span class="inline-flex items-center gap-1.5"><span class="h-1 w-1 rounded-full bg-primary-500"></span>用量清晰可查</span>
          <span class="inline-flex items-center gap-1.5"><span class="h-1 w-1 rounded-full bg-primary-500"></span>桌面端轻松接入</span>
        </div>
      </div>
    </main>
    <footer class="relative z-10 border-t border-primary-200/60 px-4 py-5 text-center text-sm text-accent-500 dark:border-primary-500/10 dark:text-dark-400">&copy; {{ currentYear }} {{ siteName }}</footer>
  </div>

  <ReferenceLanding v-else />
</template>
