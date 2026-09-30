<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">令牌用量趋势</h3>
    <div v-if="loading" class="flex h-48 items-center justify-center"><LoadingSpinner /></div>
    <div v-else-if="hasObservedData && chartData" class="h-48"><Line :data="chartData" :options="lineOptions" /></div>
    <div v-else class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无已观测令牌数据</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CategoryScale, Chart as ChartJS, Filler, Legend, LinearScale, LineElement, PointElement, Tooltip } from 'chart.js'
import { Line } from 'vue-chartjs'
import type { UsageInsights } from '@/agent/api'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

// Core chart structure copied from Sub2API TokenUsageTrend and adapted to the
// exact-window AgentAPI trend rows.
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)
const props = withDefaults(defineProps<{ trendData: UsageInsights['trend']; loading?: boolean }>(), { loading: false })
const dark = computed(() => document.documentElement.classList.contains('dark'))
const colors = computed(() => ({ text: dark.value ? '#e5e7eb' : '#374151', grid: dark.value ? '#374151' : '#e5e7eb', input: '#3b82f6', output: '#10b981', cacheCreation: '#f59e0b', cacheRead: '#06b6d4' }))
const hasObservedData = computed(() => props.trendData.some(row => row.input_tokens || row.output_tokens || row.cache_creation_tokens || row.cache_read_tokens))
const chartData = computed(() => ({ labels: props.trendData.map(row => new Date(row.key).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })), datasets: [
  { label: '输入', data: props.trendData.map(row => row.input_tokens), borderColor: colors.value.input, backgroundColor: `${colors.value.input}20`, fill: true, tension: 0.3 },
  { label: '输出', data: props.trendData.map(row => row.output_tokens), borderColor: colors.value.output, backgroundColor: `${colors.value.output}20`, fill: true, tension: 0.3 },
  { label: '缓存写入', data: props.trendData.map(row => row.cache_creation_tokens), borderColor: colors.value.cacheCreation, backgroundColor: `${colors.value.cacheCreation}20`, fill: true, tension: 0.3 },
  { label: '缓存读取', data: props.trendData.map(row => row.cache_read_tokens), borderColor: colors.value.cacheRead, backgroundColor: `${colors.value.cacheRead}20`, fill: true, tension: 0.3 },
] }))
const lineOptions = computed(() => ({ responsive: true, maintainAspectRatio: false, interaction: { intersect: false, mode: 'index' as const }, plugins: { legend: { position: 'top' as const, labels: { color: colors.value.text, usePointStyle: true, pointStyle: 'circle', padding: 15, font: { size: 11 } } }, tooltip: { callbacks: { label: (context: any) => `${context.dataset.label}: ${formatTokens(Number(context.raw) || 0)}` } } }, scales: { x: { grid: { color: colors.value.grid }, ticks: { color: colors.value.text, font: { size: 10 } } }, y: { beginAtZero: true, grid: { color: colors.value.grid }, ticks: { color: colors.value.text, font: { size: 10 }, callback: (value: string | number) => formatTokens(Number(value)) } } } }))
function formatTokens(value: number): string { if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`; if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`; if (value >= 1e3) return `${(value / 1e3).toFixed(2)}K`; return value.toLocaleString() }
</script>
