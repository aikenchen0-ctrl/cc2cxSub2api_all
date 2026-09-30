<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">模型分布</h3>
      <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800">
        <button type="button" class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors" :class="metric === 'tokens' ? activeClass : inactiveClass" @click="emit('update:metric', 'tokens')">令牌</button>
        <button type="button" class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors" :class="metric === 'actual_cost' ? activeClass : inactiveClass" @click="emit('update:metric', 'actual_cost')">实际费用</button>
      </div>
    </div>
    <div v-if="loading" class="flex h-48 items-center justify-center"><LoadingSpinner /></div>
    <div v-else-if="displayRows.length" class="flex flex-col items-center gap-4 sm:flex-row sm:gap-6">
      <div class="flex h-48 w-48 shrink-0 items-center justify-center">
        <Doughnut v-if="chartData" :data="chartData" :options="doughnutOptions" />
        <span v-else class="text-center text-sm text-gray-500">此指标暂无可用数值</span>
      </div>
      <div class="max-h-48 w-full min-w-0 flex-1 overflow-auto">
        <table class="w-full text-xs">
          <thead><tr class="text-gray-500 dark:text-gray-400"><th class="pb-2 text-left">模型</th><th class="pb-2 text-right">请求 / 明细</th><th class="pb-2 text-right">令牌</th><th class="pb-2 text-right">实际</th><th class="pb-2 text-right">标准</th></tr></thead>
          <tbody>
            <tr v-for="row in displayRows" :key="row.key" class="border-t border-gray-100 dark:border-dark-700">
              <td class="max-w-[120px] truncate py-1.5 font-medium text-gray-900 dark:text-white" :title="row.key">{{ row.key }}</td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ row.requests }} / {{ row.measured }}</td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatTokens(rowTokens(row)) }}</td>
              <td class="py-1.5 text-right text-green-600 dark:text-green-400">{{ hasActual(row) ? formatUSD(row.actual_cost_usd_nanos) : '无数据' }}</td>
              <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">{{ row.measured ? formatUSD(row.standard_cost_usd_nanos) : '无数据' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div v-else class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无数据</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArcElement, Chart as ChartJS, Legend, Tooltip } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import type { UsageInsights, UsageMetrics } from '@/agent/api'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

ChartJS.register(ArcElement, Tooltip, Legend)
type Metric = 'tokens' | 'actual_cost'
type Row = UsageMetrics & { key: string }
const props = withDefaults(defineProps<{ modelStats: UsageInsights['models']; metric?: Metric; loading?: boolean }>(), { metric: 'tokens', loading: false })
const emit = defineEmits<{ 'update:metric': [value: Metric] }>()
const activeClass = 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
const inactiveClass = 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'

const displayRows = computed(() => [...props.modelStats].sort((a, b) => metricValue(b) - metricValue(a)))
const chartData = computed(() => {
  const rows = displayRows.value.filter(row => metricValue(row) > 0)
  if (!rows.length) return null
  return { labels: rows.map(row => row.key), datasets: [{ data: rows.map(metricValue), backgroundColor: ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#ec4899','#14b8a6','#f97316','#6366f1','#84cc16','#06b6d4','#a855f7'].slice(0, rows.length), borderWidth: 0 }] }
})
const doughnutOptions = computed(() => ({ responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false }, tooltip: { callbacks: { label: (context: any) => { const value = Number(context.raw) || 0; const total = context.dataset.data.reduce((sum: number, item: number) => sum + item, 0); const formatted = props.metric === 'actual_cost' ? formatUSD(value) : formatTokens(value); return `${context.label}: ${formatted} (${total ? ((value / total) * 100).toFixed(1) : '0.0'}%)` } } } } }))

function rowTokens(row: Row): number { return row.input_tokens + row.output_tokens + row.cache_read_tokens + row.cache_creation_tokens }
function hasActual(row: Row): boolean { return row.measured > row.missing_actual }
function metricValue(row: Row): number { return props.metric === 'actual_cost' ? (hasActual(row) ? row.actual_cost_usd_nanos : 0) : rowTokens(row) }
function formatTokens(value: number): string { if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`; if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`; if (value >= 1e3) return `${(value / 1e3).toFixed(2)}K`; return value.toLocaleString() }
function formatUSD(nanos: number): string { const value = nanos / 1e9; return `$${value >= 1 ? value.toFixed(2) : value.toFixed(4)}` }
</script>
