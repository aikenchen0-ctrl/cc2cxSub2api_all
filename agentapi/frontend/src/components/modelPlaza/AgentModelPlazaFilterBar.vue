<script setup lang="ts">
import type { AgentModelCapability } from '@/agent/modelCatalog'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'

defineProps<{
  providers: string[]
  provider: string
  capability: AgentModelCapability | 'all'
  search: string
}>()

defineEmits<{
  'update:provider': [value: string]
  'update:capability': [value: AgentModelCapability | 'all']
  'update:search': [value: string]
}>()

const capabilities: Array<{ value: AgentModelCapability | 'all'; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'text', label: '文本' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' },
]
</script>

<template>
  <section class="space-y-3 rounded-2xl border border-gray-100 bg-white px-5 py-4 shadow-card dark:border-dark-700/50 dark:bg-dark-800/50">
    <div class="flex items-start gap-3">
      <span class="w-12 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">平台</span>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="item in ['all', ...providers]"
          :key="item"
          type="button"
          class="rounded-lg px-3 py-1.5 text-sm font-medium transition"
          :class="provider === item ? 'bg-gradient-to-r from-primary-500 to-primary-600 text-white shadow-sm shadow-primary-500/30' : 'bg-gray-50 text-gray-600 ring-1 ring-inset ring-gray-200 hover:bg-gray-100 dark:bg-dark-900/60 dark:text-dark-300 dark:ring-dark-700'"
          @click="$emit('update:provider', item)"
        >
          {{ item === 'all' ? '全部' : item }}
        </button>
      </div>
    </div>

    <div class="flex items-start gap-3">
      <span class="w-12 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">能力</span>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="item in capabilities"
          :key="item.value"
          type="button"
          class="rounded-lg px-3 py-1.5 text-sm font-medium transition"
          :class="capability === item.value ? 'bg-gradient-to-r from-primary-500 to-primary-600 text-white shadow-sm shadow-primary-500/30' : 'bg-gray-50 text-gray-600 ring-1 ring-inset ring-gray-200 hover:bg-gray-100 dark:bg-dark-900/60 dark:text-dark-300 dark:ring-dark-700'"
          @click="$emit('update:capability', item.value)"
        >
          {{ item.label }}
        </button>
      </div>
    </div>

    <div class="flex flex-wrap items-start gap-3">
      <span class="w-12 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">模型</span>
      <div class="w-full sm:w-80">
        <AgentSearchInput
          :model-value="search"
          :debounce-ms="0"
          clearable
          aria-label="搜索模型"
          clear-aria-label="清空模型搜索"
          placeholder="按模型名称搜索"
          @update:model-value="$emit('update:search', $event)"
        />
      </div>
    </div>
  </section>
</template>
