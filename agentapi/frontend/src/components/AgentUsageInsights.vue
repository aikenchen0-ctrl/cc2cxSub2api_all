<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type UsageInsights } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentUsageStatsCards from '@/components/admin/usage/AgentUsageStatsCards.vue'
import AgentModelDistributionChart from '@/components/charts/AgentModelDistributionChart.vue'
import AgentTokenUsageTrend from '@/components/charts/AgentTokenUsageTrend.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'

const windowOptions = [
  { value: '1h', label: '最近 1 小时' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]

const window = ref('24h')
const metric = ref<'tokens' | 'actual_cost'>('tokens')
const data = ref<UsageInsights | null>(null)
const loading = ref(false)
const error = ref('')
let sequence = 0

async function load(): Promise<void> {
  const current = ++sequence
  loading.value = true
  error.value = ''
  data.value = null
  try {
    const result = await agentAPI.getUsageInsights(window.value)
    if (current === sequence) data.value = result
  } catch (err) {
    if (current === sequence) error.value = errorMessage(err, '统计暂不可用')
  } finally {
    if (current === sequence) loading.value = false
  }
}

function statusLabel(status: UsageInsights['status']): string {
  return status === 'partial' ? '部分可用' : status === 'measured' ? '实测' : '无记录'
}

onMounted(load)
</script>

<template>
  <section class="space-y-4" aria-label="用量分析">
    <div class="card flex flex-wrap items-center justify-between gap-3 p-4">
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">用量与费用分析</h2>
        <p class="mt-1 text-xs text-gray-500">数据源：本站请求留存的用量快照；只读统计，不重新计费。</p>
      </div>
      <div class="flex gap-3">
        <AgentSelect v-model="window" :options="windowOptions" class="w-40" aria-label="统计窗口" @change="load" />
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">刷新统计</button>
      </div>
    </div>

    <div v-if="loading" class="card flex min-h-32 items-center justify-center p-6" role="status">正在读取统计…</div>
    <p v-else-if="error" role="alert" class="card border-amber-200 p-4 text-sm text-amber-600 dark:border-amber-900">{{ error }}；不以零代替缺失数据。</p>

    <template v-else-if="data">
      <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500">
        <p>{{ data.scope === 'agent' ? '本站全部用户' : '当前用户' }} · {{ new Date(data.start).toLocaleString('zh-CN') }} 至 {{ new Date(data.end).toLocaleString('zh-CN') }}（不含结束时刻）</p>
        <AgentStatusBadge :status="data.status" :label="statusLabel(data.status)" />
      </div>

      <p v-if="data.status === 'partial'" class="card border-amber-200 bg-amber-50/70 p-4 text-sm text-amber-700 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-300">
        {{ data.unobserved }} 条缺少用量明细，{{ data.missing_actual }} 条未报告实际费用，{{ data.pending }} 条待确认。费用仅为已报告部分，不代表完整总额。
      </p>

      <AgentUsageStatsCards :stats="data" />

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <AgentModelDistributionChart v-model:metric="metric" :model-stats="data.models" />
        <AgentTokenUsageTrend :trend-data="data.trend" />
      </div>

      <div class="card p-4 text-sm text-gray-600 dark:text-gray-300">
        <p>模型路由一致性：{{ data.route_observed ? `${data.route_mismatch} 条不一致 / ${data.route_observed} 条已观测` : '未观测' }}</p>
        <p class="mt-1 text-xs text-gray-500">图表的空时间桶表示该桶内没有本站请求；“无数据”表示明细尚未同步，二者含义不同。</p>
      </div>
    </template>
  </section>
</template>
