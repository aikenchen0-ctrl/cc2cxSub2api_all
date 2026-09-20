<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 px-4 py-4 dark:border-dark-800 sm:px-6">
      <nav class="mx-auto flex max-w-5xl items-center justify-between gap-4">
        <router-link to="/home" class="flex items-center gap-3 font-semibold">
          <img :src="siteLogo || '/logo.jpg'" alt="Logo" class="h-9 w-9 rounded-lg object-contain" />
          <span>{{ siteName }}</span>
        </router-link>
        <router-link to="/home" class="text-sm text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white">
          {{ t('downloads.backHome') }}
        </router-link>
      </nav>
    </header>

    <main class="mx-auto max-w-5xl px-4 py-12 sm:px-6 sm:py-16">
      <div class="max-w-2xl">
        <p class="text-sm font-medium uppercase tracking-[0.18em] text-primary-600 dark:text-primary-400">CCSwitch</p>
        <h1 class="mt-3 text-3xl font-bold tracking-tight sm:text-4xl">{{ t('downloads.title') }}</h1>
        <p class="mt-4 text-base leading-7 text-gray-600 dark:text-dark-300">{{ t('downloads.description') }}</p>
      </div>

      <section class="mt-10 rounded-2xl border border-primary-200 bg-primary-50/70 p-5 dark:border-primary-900/60 dark:bg-primary-950/30">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p class="text-sm font-medium text-primary-700 dark:text-primary-300">{{ t('downloads.recommended') }}</p>
            <h2 class="mt-1 text-xl font-semibold">{{ recommendation.title }}</h2>
            <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">{{ recommendation.description }}</p>
          </div>
          <a :href="recommendation.url" class="inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700" target="_blank" rel="noopener noreferrer">
            {{ t('downloads.downloadRecommended') }}
          </a>
        </div>
      </section>

      <section class="mt-8 grid gap-4 md:grid-cols-2" aria-label="CCSwitch downloads">
        <a v-for="item in downloads" :key="item.key" :href="item.url" target="_blank" rel="noopener noreferrer" class="group rounded-xl border border-gray-200 bg-white p-5 transition hover:border-primary-400 hover:shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="font-semibold group-hover:text-primary-600 dark:group-hover:text-primary-400">{{ item.title }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ item.description }}</p>
            </div>
            <Icon name="download" size="md" class="shrink-0 text-gray-400" />
          </div>
          <span class="mt-4 inline-block text-xs text-gray-400">v3.20.0</span>
        </a>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { recommendDownload, downloadOptions } from '@/utils/ccswitch-downloads'

const { t } = useI18n()
const appStore = useAppStore()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '超参池X')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const downloads = computed(() => downloadOptions.map((item) => ({ ...item, title: t(item.titleKey), description: t(item.descriptionKey) })))
const recommendation = computed(() => ({ ...recommendDownload(), title: t(recommendDownload().titleKey), description: t(recommendDownload().descriptionKey) }))
</script>
