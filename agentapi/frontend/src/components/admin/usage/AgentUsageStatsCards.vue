<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div class="card flex items-center gap-3 p-4">
      <div class="rounded-lg bg-blue-100 p-2 text-blue-600 dark:bg-blue-900/30">
        <Icon name="document" size="md" />
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500">请求总数</p>
        <p class="text-xl font-bold">{{ stats.requests.toLocaleString() }}</p>
        <p class="text-xs text-gray-400">所选精确时间窗口</p>
      </div>
    </div>

    <div class="card flex items-center gap-3 p-4">
      <div class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30">
        <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg>
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500">已观测令牌</p>
        <p class="text-xl font-bold">{{ formatTokens(totalTokens) }}</p>
        <p class="flex flex-wrap items-center gap-x-1 text-xs text-gray-500">
          <span>输入 {{ formatTokens(stats.input_tokens) }}</span><span>/</span>
          <span>输出 {{ formatTokens(stats.output_tokens) }}</span><span>/</span>
          <span class="group relative inline-flex cursor-help items-center gap-0.5" tabindex="0">
            <span>缓存 {{ formatTokens(totalCacheTokens) }}</span>
            <svg class="h-3.5 w-3.5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            <span class="pointer-events-none absolute left-1/2 top-full z-30 mt-2 hidden w-56 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs text-gray-700 shadow-lg group-hover:block group-focus:block dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200">
              <span class="flex justify-between gap-3"><span>缓存写入</span><span>{{ formatTokens(stats.cache_creation_tokens) }}</span></span>
              <span class="mt-1 flex justify-between gap-3"><span>缓存读取</span><span>{{ formatTokens(stats.cache_read_tokens) }}</span></span>
            </span>
          </span>
        </p>
      </div>
    </div>

    <div class="card flex items-center gap-3 p-4">
      <div class="rounded-lg bg-green-100 p-2 text-green-600 dark:bg-green-900/30">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-gray-500">已报告实际费用</p>
        <p class="text-xl font-bold text-green-600">{{ reportedActual > 0 ? formatUSD(stats.actual_cost_usd_nanos) : '无数据' }}</p>
        <p class="text-xs text-gray-400">
          标准费用 {{ stats.measured ? formatUSD(stats.standard_cost_usd_nanos) : '无数据' }}
          <span v-if="stats.missing_actual"> · {{ stats.missing_actual }} 条待主站报告</span>
        </p>
      </div>
    </div>

    <div class="card flex items-center gap-3 p-4">
      <div class="rounded-lg bg-purple-100 p-2 text-purple-600 dark:bg-purple-900/30">
        <Icon name="chart" size="md" />
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500">主站明细覆盖</p>
        <p class="text-xl font-bold">{{ stats.requests ? `${stats.measured} / ${stats.requests}` : '无记录' }}</p>
        <p class="text-xs text-gray-400">{{ coverageLabel }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { UsageInsights } from '@/agent/api'
import Icon from '@/components/icons/Icon.vue'

// Layout copied from Sub2API UsageStatsCards. Fields are adapted to the
// AgentAPI exact-window snapshot contract; unavailable facts stay unavailable.
const props = defineProps<{ stats: UsageInsights }>()
const totalCacheTokens = computed(() => props.stats.cache_creation_tokens + props.stats.cache_read_tokens)
const totalTokens = computed(() => props.stats.input_tokens + props.stats.output_tokens + totalCacheTokens.value)
const reportedActual = computed(() => Math.max(0, props.stats.measured - props.stats.missing_actual))
const coverageLabel = computed(() => props.stats.status === 'partial' ? '部分可用' : props.stats.status === 'measured' ? '实测' : '暂无数据')

function formatTokens(value: number): string {
  if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`
  if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`
  if (value >= 1e3) return `${(value / 1e3).toFixed(2)}K`
  return value.toLocaleString()
}

function formatUSD(nanos: number): string {
  const dollars = nanos / 1e9
  return `$${dollars >= 1 ? dollars.toFixed(4) : dollars.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')}`
}
</script>
