<template>
  <div ref="root" class="relative">
    <template v-if="isAdmin">
      <button type="button" class="flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs transition-colors" :class="hasUpdate ? 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400 dark:hover:bg-amber-900/50' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'" :title="hasUpdate ? t('version.updateAvailable') : t('version.upToDate')" @click="dropdownOpen = !dropdownOpen">
        <span class="font-medium">v{{ currentVersion || '--' }}</span>
        <span v-if="hasUpdate" class="h-2 w-2 rounded-full bg-amber-500 dark:bg-amber-400" />
      </button>
      <div v-if="dropdownOpen" class="absolute left-0 z-50 mt-2 w-64 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800">
        <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('version.currentVersion') }}</span>
          <button type="button" class="rounded-md px-2 py-1 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-dark-200" :disabled="loading" @click="refreshVersion">{{ t('version.refresh') }}</button>
        </div>
        <div v-if="loading" class="px-4 py-6 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('version.checking') }}</div>
        <div v-else class="px-4 py-5 text-center">
          <div class="text-2xl font-bold text-gray-900 dark:text-white">v{{ currentVersion || '--' }}</div>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ hasUpdate ? `${t('version.latestVersion')}: v${latestVersion}` : t('version.upToDate') }}</p>
          <a v-if="hasUpdate && releaseUrl" :href="releaseUrl" target="_blank" rel="noopener noreferrer" class="mt-3 inline-block text-xs text-amber-600 hover:underline dark:text-amber-400">{{ t('version.viewChangelog') }}</a>
        </div>
      </div>
    </template>
    <span v-else-if="version" class="text-xs text-gray-500 dark:text-dark-400">v{{ version }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
const { t } = useI18n()
const props = defineProps<{ version?: string }>()
const authStore = useAuthStore(); const appStore = useAppStore(); const root = ref<HTMLElement | null>(null)
const isAdmin = computed(() => authStore.isAdmin); const dropdownOpen = ref(false); const loading = computed(() => appStore.versionLoading); const currentVersion = computed(() => appStore.currentVersion || props.version || ''); const latestVersion = computed(() => appStore.latestVersion); const hasUpdate = computed(() => appStore.hasUpdate); const releaseUrl = computed(() => appStore.releaseInfo?.html_url)
function refreshVersion() { if (isAdmin.value) void appStore.fetchVersion(true) }
function closeOnOutside(event: MouseEvent) { if (root.value && !root.value.contains(event.target as Node)) dropdownOpen.value = false }
onMounted(() => { if (isAdmin.value) void appStore.fetchVersion(false); document.addEventListener('click', closeOnOutside) })
onBeforeUnmount(() => document.removeEventListener('click', closeOnOutside))
</script>
