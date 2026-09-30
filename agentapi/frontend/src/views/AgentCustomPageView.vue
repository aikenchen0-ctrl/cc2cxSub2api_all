<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { agentAPI, type AgentContentPage } from '@/agent/api'

const route = useRoute()
const page = ref<AgentContentPage | null>(null)
const loading = ref(true)
const failed = ref(false)
const rendered = computed(() => page.value ? DOMPurify.sanitize(marked.parse(page.value.content, { breaks: true, gfm: true }) as string) : '')
async function load(): Promise<void> {
  loading.value = true; failed.value = false; page.value = null
  try { page.value = await agentAPI.contentPages.get('custom', String(route.params.slug || '')) }
  catch { failed.value = true }
  finally { loading.value = false }
}
watch(() => route.params.slug, load)
onMounted(load)
</script>

<template>
  <main class="card min-h-[calc(100vh-8rem)] overflow-hidden">
    <div v-if="loading" class="flex min-h-96 items-center justify-center"><span class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></span></div>
    <div v-else-if="failed || !page" class="flex min-h-96 items-center justify-center p-10 text-center"><div><Icon name="link" size="lg" class="mx-auto text-gray-400" /><h2 class="mt-4 text-lg font-semibold">页面不存在</h2><p class="mt-2 text-sm text-gray-500">该页面尚未发布或已被移除。</p></div></div>
    <article v-else class="mx-auto max-w-4xl p-6 md:p-10"><h1 class="text-3xl font-bold">{{ page.title }}</h1><div class="markdown-page-content mt-8" v-html="rendered"></div></article>
  </main>
</template>

<style scoped>
.markdown-page-content { line-height: 1.7; overflow-wrap: anywhere; }
.markdown-page-content :deep(h1) { @apply mb-4 mt-8 border-b border-gray-200 pb-2 text-3xl font-bold dark:border-dark-600; }
.markdown-page-content :deep(h2) { @apply mb-3 mt-6 text-2xl font-bold; }
.markdown-page-content :deep(h3) { @apply mb-2 mt-5 text-xl font-semibold; }
.markdown-page-content :deep(p) { @apply mb-4; }
.markdown-page-content :deep(ul) { @apply mb-4 list-disc pl-6; }
.markdown-page-content :deep(ol) { @apply mb-4 list-decimal pl-6; }
.markdown-page-content :deep(a) { @apply text-primary-500 underline; }
.markdown-page-content :deep(blockquote) { @apply my-4 border-l-4 border-gray-300 pl-4 text-gray-600; }
.markdown-page-content :deep(code) { @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-700; }
.markdown-page-content :deep(pre) { @apply my-4 overflow-x-auto rounded-lg bg-gray-900 p-4 text-gray-100; }
.markdown-page-content :deep(table) { @apply my-4 w-full border-collapse; }
.markdown-page-content :deep(th), .markdown-page-content :deep(td) { @apply border border-gray-300 px-3 py-2 dark:border-dark-500; }
</style>
