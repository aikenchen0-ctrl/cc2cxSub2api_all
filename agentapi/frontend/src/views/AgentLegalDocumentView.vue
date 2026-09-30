<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { agentAPI, type AgentContentPage } from '@/agent/api'
import { siteLogo, siteName } from '@/agent/branding'

const route = useRoute()
const page = ref<AgentContentPage | null>(null)
const loading = ref(true)
const failed = ref(false)
const rendered = computed(() => page.value ? DOMPurify.sanitize(marked.parse(page.value.content, { breaks: true, gfm: true }) as string) : '')

async function load(): Promise<void> {
  loading.value = true
  failed.value = false
  page.value = null
  try { page.value = await agentAPI.contentPages.get('legal', String(route.params.slug || '')) }
  catch { failed.value = true }
  finally { loading.value = false }
}
watch(() => route.params.slug, load)
onMounted(load)
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/95 dark:border-dark-800 dark:bg-dark-900/95">
      <div class="mx-auto flex max-w-5xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
        <router-link to="/home" class="flex min-w-0 items-center gap-3"><span class="flex h-10 w-10 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200"><img :src="siteLogo" alt="" class="h-full w-full object-contain"></span><span class="truncate font-semibold">{{ siteName }}</span></router-link>
        <router-link to="/login" class="btn btn-primary">登录</router-link>
      </div>
    </header>
    <main class="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:py-10">
      <div v-if="loading" class="flex min-h-80 items-center justify-center"><span class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></span></div>
      <section v-else-if="failed || !page" class="card p-6"><div class="flex gap-3"><Icon name="document" /><div><h1 class="font-semibold">页面不存在</h1><p class="mt-2 text-sm text-gray-500">该协议尚未发布或已被移除。</p></div></div></section>
      <article v-else>
        <div class="mb-8 border-b border-gray-200 pb-6 dark:border-dark-700"><p class="text-sm font-medium text-primary-700">站点协议</p><h1 class="mt-2 text-3xl font-bold">{{ page.title }}</h1><p class="mt-3 text-sm text-gray-500">更新于 {{ new Date(page.updated_at).toLocaleString() }}</p></div>
        <div class="markdown-content" v-html="rendered"></div>
      </article>
    </main>
  </div>
</template>

<style scoped>
.markdown-content { line-height: 1.75; overflow-wrap: anywhere; }
.markdown-content :deep(h1) { @apply mb-4 mt-8 border-b border-gray-200 pb-3 text-3xl font-bold dark:border-dark-700; }
.markdown-content :deep(h2) { @apply mb-3 mt-7 text-2xl font-bold; }
.markdown-content :deep(h3) { @apply mb-2 mt-6 text-xl font-semibold; }
.markdown-content :deep(p) { @apply mb-4 text-gray-700 dark:text-dark-200; }
.markdown-content :deep(a) { @apply text-primary-600 underline; }
.markdown-content :deep(ul) { @apply mb-4 list-disc pl-6; }
.markdown-content :deep(ol) { @apply mb-4 list-decimal pl-6; }
.markdown-content :deep(blockquote) { @apply my-5 border-l-4 border-gray-300 pl-4 text-gray-600; }
.markdown-content :deep(code) { @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-800; }
.markdown-content :deep(pre) { @apply my-5 overflow-x-auto rounded-lg bg-gray-950 p-4 text-gray-100; }
.markdown-content :deep(table) { @apply my-5 w-full border-collapse; }
.markdown-content :deep(th), .markdown-content :deep(td) { @apply border border-gray-300 px-3 py-2 dark:border-dark-600; }
</style>
