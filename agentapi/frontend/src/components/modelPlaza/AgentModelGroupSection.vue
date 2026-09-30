<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import type { AgentCatalogGroup, AgentModelCapability } from '@/agent/modelCatalog'

defineProps<{ group: AgentCatalogGroup }>()

const labels: Record<AgentModelCapability, string> = { text: '文本', image: '图片', video: '视频' }
const icons: Record<AgentModelCapability, 'chat' | 'sparkles' | 'play'> = { text: 'chat', image: 'sparkles', video: 'play' }

function providerClass(provider: string): string {
  if (provider === 'OpenAI') return 'border-emerald-400/60'
  if (provider === 'Anthropic') return 'border-orange-400/60'
  if (provider === 'Google') return 'border-blue-400/60'
  if (provider === 'xAI') return 'border-slate-500/60'
  if (provider === 'ByteDance' || provider === 'Kling') return 'border-violet-400/60'
  return 'border-primary-400/50'
}
</script>

<template>
  <section class="overflow-hidden rounded-2xl border bg-white shadow-card dark:bg-dark-800/50" :class="providerClass(group.provider)">
    <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700/60">
      <div>
        <div class="flex items-center gap-2">
          <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="cpu" size="sm" /></span>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ group.provider }}</h2>
          <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-dark-300">{{ group.models.length }} 个模型</span>
        </div>
        <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">这里只展示当前代理站公开开放的模型。</p>
      </div>
      <span class="inline-flex items-center gap-1 rounded-lg bg-amber-50 px-2.5 py-1 text-xs text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"><Icon name="infoCircle" size="xs" />费用以主站实际账单为准</span>
    </header>

    <div class="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
      <article v-for="model in group.models" :key="model.name" class="group flex min-h-32 flex-col justify-between rounded-xl border border-gray-100 bg-gray-50/70 p-4 transition hover:border-primary-300 hover:bg-white hover:shadow-sm dark:border-dark-700 dark:bg-dark-900/40 dark:hover:border-primary-500/50 dark:hover:bg-dark-900">
        <div>
          <div class="flex items-start justify-between gap-3">
            <p class="break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ model.name }}</p>
            <span class="inline-flex shrink-0 items-center gap-1 rounded-md bg-white px-2 py-1 text-xs text-gray-600 ring-1 ring-inset ring-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700"><Icon :name="icons[model.capability]" size="xs" />{{ labels[model.capability] }}</span>
          </div>
          <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-dark-400">该模型请求由代理站鉴权后转发至主站，余额、扣费与用量事实由主站确认。</p>
        </div>
        <RouterLink :to="{ path: '/console', query: { model: model.name, mode: model.capability } }" class="mt-4 inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300">
          在工作台使用 <Icon name="arrowRight" size="xs" />
        </RouterLink>
      </article>
    </div>
  </section>
</template>
