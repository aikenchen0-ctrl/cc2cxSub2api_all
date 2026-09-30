<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { agentAPI, type AgentUsageView, type UsageInsights } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentUsageStatsCards from '@/components/admin/usage/AgentUsageStatsCards.vue'
import AgentModelDistributionChart from '@/components/charts/AgentModelDistributionChart.vue'
import AgentTokenUsageTrend from '@/components/charts/AgentTokenUsageTrend.vue'
import AgentAutoRefreshButton from '@/components/common/AgentAutoRefreshButton.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'

type WindowOption = '1h' | '24h' | '7d' | '30d'

const windowOptions = [
  { value: '1h', label: '最近 1 小时' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]
const refreshIntervals = [15, 30, 60, 120] as const

const windowOption = ref<WindowOption>('24h')
const metric = ref<'tokens' | 'actual_cost'>('tokens')
const insights = ref<UsageInsights | null>(null)
const recent = ref<AgentUsageView[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const lastUpdated = ref<Date | null>(null)
const autoRefresh = ref(true)
const refreshSeconds = ref(30)
const countdown = ref(refreshSeconds.value)
let loadSequence = 0
let refreshTimer: ReturnType<typeof setInterval> | undefined

const failedRequests = computed(() => recent.value.filter(item => item.settlement_status === 'failed' || Boolean(item.error)).length)
const pendingRequests = computed(() => insights.value?.pending || 0)
const routeMismatchRate = computed(() => {
  const observed = insights.value?.route_observed || 0
  return observed ? `${(((insights.value?.route_mismatch || 0) / observed) * 100).toFixed(1)}%` : '未观测'
})

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  try {
    const nextInsights = await agentAPI.getUsageInsights(windowOption.value)
    const usage = await agentAPI.getUsage(1, 50, {
      start_time: nextInsights.start,
      end_time: nextInsights.end,
    })
    if (sequence !== loadSequence) return
    insights.value = nextInsights
    recent.value = usage.items
    total.value = usage.total
    lastUpdated.value = new Date()
  } catch (err) {
    if (sequence === loadSequence) error.value = errorMessage(err, '运营监控数据暂不可用，请稍后重试。')
  } finally {
    if (sequence === loadSequence) {
      loading.value = false
      countdown.value = refreshSeconds.value
    }
  }
}

function setAutoRefresh(value: boolean): void {
  autoRefresh.value = value
  configureAutoRefresh()
}

function setAutoRefreshInterval(value: number): void {
  refreshSeconds.value = value
  configureAutoRefresh()
}

function configureAutoRefresh(): void {
  if (refreshTimer) clearInterval(refreshTimer)
  refreshTimer = undefined
  countdown.value = refreshSeconds.value
  if (!autoRefresh.value) return
  refreshTimer = setInterval(() => {
    if (document.hidden || loading.value) return
    countdown.value -= 1
    if (countdown.value <= 0) void load()
  }, 1000)
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function formatDuration(value: number): string {
  if (!value) return '—'
  return value >= 1000 ? `${(value / 1000).toFixed(2)} s` : `${value} ms`
}

function formatTokens(item: AgentUsageView): string {
  const value = item.input_tokens + item.output_tokens + item.cache_creation_tokens + item.cache_read_tokens
  return value.toLocaleString()
}

function statusLabel(item: AgentUsageView): string {
  if (item.settlement_status === 'pending') return '待主站确认'
  if (item.settlement_status === 'confirmed') return '已确认'
  if (item.settlement_status === 'failed') return '失败'
  return item.settlement_status || '未知'
}

function statusTone(item: AgentUsageView): string {
  return item.error ? 'error' : item.settlement_status
}

onMounted(() => {
  void load()
  configureAutoRefresh()
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="card flex flex-wrap items-center justify-between gap-4 p-5">
      <div>
        <div class="flex items-center gap-2">
          <Icon name="chartBar" size="lg" class="text-primary-600 dark:text-primary-400" />
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">运营监控</h1>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">沿用主站运营面板的分析布局，仅统计当前代理站留存的请求与 Sub2API 用量事实。</p>
      </div>
      <div class="flex flex-wrap items-end gap-3">
        <label class="block text-xs font-medium text-gray-500">
          时间范围
          <AgentSelect v-model="windowOption" :options="windowOptions" class="mt-1 w-40" aria-label="运营监控时间范围" @change="load" />
        </label>
        <AgentAutoRefreshButton
          :enabled="autoRefresh"
          :interval-seconds="refreshSeconds"
          :countdown="countdown"
          :intervals="refreshIntervals"
          @update:enabled="setAutoRefresh"
          @update:interval="setAutoRefreshInterval"
        />
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新
        </button>
      </div>
    </header>

    <div class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-300">
      此页面不读取或修改主站全局运行参数、账号池、告警规则和系统日志。计费记录仍先进入 Sub2API；这里展示的是当前代理站范围内保存的只读快照。
    </div>

    <p v-if="error" role="alert" class="card border-red-200 p-4 text-sm text-red-600 dark:border-red-900">{{ error }}</p>
    <div v-if="loading && !insights" class="card flex min-h-40 items-center justify-center text-sm text-gray-500" role="status">正在读取本站运营数据…</div>

    <template v-else-if="insights">
      <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500">
        <p>本站全部用户 · {{ formatTime(insights.start) }} 至 {{ formatTime(insights.end) }}（结束时刻不计入）</p>
        <p>最近更新：{{ lastUpdated ? lastUpdated.toLocaleTimeString('zh-CN') : '—' }}</p>
      </div>

      <AgentUsageStatsCards :stats="insights" />

      <section class="grid gap-4 md:grid-cols-3" aria-label="运行状态">
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">待主站确认</p>
          <p class="mt-2 text-2xl font-bold text-amber-600">{{ pendingRequests.toLocaleString() }}</p>
          <p class="mt-1 text-xs text-gray-400">尚未完成主站事实同步的本站请求</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">最近列表异常</p>
          <p class="mt-2 text-2xl font-bold" :class="failedRequests ? 'text-red-600' : 'text-emerald-600'">{{ failedRequests.toLocaleString() }}</p>
          <p class="mt-1 text-xs text-gray-400">当前最多 50 条明细中的失败或错误记录</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">上游模型不一致率</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ routeMismatchRate }}</p>
          <p class="mt-1 text-xs text-gray-400">{{ insights.route_mismatch }} 条不一致 / {{ insights.route_observed }} 条已观测</p>
        </div>
      </section>

      <section class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <AgentTokenUsageTrend :trend-data="insights.trend" />
        <AgentModelDistributionChart v-model:metric="metric" :model-stats="insights.models" />
      </section>

      <section class="card overflow-hidden" aria-label="请求明细">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 p-5 dark:border-dark-700">
          <div>
            <h2 class="font-semibold text-gray-900 dark:text-white">请求明细</h2>
            <p class="mt-1 text-xs text-gray-500">按请求时间倒序，展示所选窗口内最近 50 条本站记录。</p>
          </div>
          <span class="badge badge-gray">共 {{ total.toLocaleString() }} 条</span>
        </div>
        <DataTable
          :columns='[{"key":"request_id","label":"请求编号"},{"key":"user","label":"用户"},{"key":"model","label":"模型"},{"key":"tokens","label":"令牌"},{"key":"latency","label":"耗时"},{"key":"status","label":"状态"},{"key":"created_at","label":"时间"}]'
          :data="recent"
          :loading="loading"
          row-key="request_id"
        >
          <template #cell-request_id="{ row: item }"><span class="font-mono text-xs">{{ item.request_id }}</span></template>
          <template #cell-user="{ row: item }"><span class="font-mono text-xs">{{ item.proxy_main_user_id || '—' }}</span></template>
          <template #cell-model="{ row: item }"><span class="font-mono text-xs">{{ item.model || '—' }}</span></template>
          <template #cell-tokens="{ row: item }">{{ formatTokens(item) }}</template>
          <template #cell-latency="{ row: item }">{{ formatDuration(item.duration_ms) }}</template>
          <template #cell-status="{ row: item }"><AgentStatusBadge :status="statusTone(item)" :label="statusLabel(item)" /></template>
          <template #cell-created_at="{ row: item }"><span class="whitespace-nowrap text-gray-500">{{ formatTime(item.created_at) }}</span></template>
          <template #empty><div class="py-10 text-center text-sm text-gray-500">所选时间范围内暂无本站请求。</div></template>
        </DataTable>
      </section>
    </template>
  </main>
</template>
