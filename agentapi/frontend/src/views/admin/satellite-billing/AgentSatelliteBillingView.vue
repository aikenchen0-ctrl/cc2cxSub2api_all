<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentUsageView, type SettlementView, type UsageInsights } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'

type InsightWindow = '24h' | '7d' | '30d'

const windows: Array<{ value: InsightWindow; label: string }> = [
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]

const billingModes = [
  { key: 'model', title: '主站模型计费', description: '每次请求仍按 Sub2API 主站模型价格与实际用量计费。', icon: 'creditCard' as const, active: true },
  { key: 'custom', title: '自定义价格', description: '代理站不能覆盖主站价格或自行修改用户余额。', icon: 'calculator' as const, active: false },
  { key: 'request', title: '按请求计费', description: '当前代理站不建立第二套按次扣费账本。', icon: 'clock' as const, active: false },
  { key: 'points', title: '积分计费', description: '当前代理站不发行与主站余额并行的积分。', icon: 'sparkles' as const, active: false },
]

const selectedWindow = ref<InsightWindow>('30d')
const insights = ref<UsageInsights | null>(null)
const recentUsage = ref<AgentUsageView[]>([])
const pendingSettlements = ref<SettlementView[]>([])
const total = ref(0)
const loading = ref(true)
const reconciling = ref(false)
const loadError = ref('')
const actionMessage = ref('')
let loadSequence = 0

const reportedActual = computed(() => {
  const data = insights.value
  return data ? Math.max(0, data.measured - data.missing_actual) : 0
})

const actualCoverage = computed(() => {
  const measured = insights.value?.measured || 0
  return measured ? (reportedActual.value / measured) * 100 : 0
})

function formatNumber(value?: number): string {
  return Number(value || 0).toLocaleString('zh-CN')
}

function formatUSD(nanos?: number): string {
  const value = Number(nanos || 0) / 1e9
  return `$${value >= 1 ? value.toFixed(2) : value.toFixed(4)}`
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

function statusLabel(status: string): string {
  if (status === 'confirmed') return '主站已确认'
  if (status === 'pending' || status === 'prepared') return '等待主站确认'
  if (status === 'released') return '已释放'
  if (status === 'reversed') return '已冲正'
  if (status === 'failed') return '同步失败'
  return status || '未知'
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  loadError.value = ''
  actionMessage.value = ''
  try {
    const [nextInsights, pending] = await Promise.all([
      agentAPI.getUsageInsights(selectedWindow.value),
      agentAPI.getSettlements(),
    ])
    const usage = await agentAPI.getUsage(1, 25, {
      start_time: nextInsights.start,
      end_time: nextInsights.end,
    })
    if (sequence !== loadSequence) return
    insights.value = nextInsights
    pendingSettlements.value = pending.items
    recentUsage.value = usage.items
    total.value = usage.total
  } catch (cause) {
    if (sequence !== loadSequence) return
    insights.value = null
    pendingSettlements.value = []
    recentUsage.value = []
    total.value = 0
    loadError.value = errorMessage(cause, '代理站计费信息加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function reconcile(): Promise<void> {
  if (reconciling.value || !pendingSettlements.value.length) return
  reconciling.value = true
  actionMessage.value = ''
  try {
    const result = await agentAPI.reconcileSettlements()
    const message = result.total ? `已核对 ${result.total} 条待确认记录。` : '当前没有待核对记录。'
    await load()
    actionMessage.value = message
  } catch (cause) {
    actionMessage.value = errorMessage(cause, '核对主站用量记录失败，请稍后重试。')
  } finally {
    reconciling.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-5 p-6" aria-label="代理站计费">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">站点管理</p>
        <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">代理站计费</h1>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">沿用主站卫星计费页面的信息层级，但这里只展示当前代理站请求的主站计费事实。最终余额、价格和扣费记录仍以 Sub2API 为唯一权威。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <AgentSelect v-model="selectedWindow" :options="windows" class="w-40" aria-label="代理站计费时间范围" :disabled="loading" @change="load" />
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新
        </button>
      </div>
    </header>

    <div v-if="loadError" class="flex items-center justify-between gap-4 rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/10 dark:text-red-300" role="alert">
      <span>{{ loadError }}</span>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="load">重试</button>
    </div>
    <p v-if="actionMessage" class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/10 dark:text-blue-300" role="status">{{ actionMessage }}</p>

    <section class="card overflow-hidden" aria-labelledby="billing-mode-title">
      <div class="border-b border-gray-100 px-5 py-5 dark:border-dark-700 sm:px-6">
        <div class="flex flex-wrap items-center gap-2">
          <h2 id="billing-mode-title" class="text-lg font-semibold text-gray-900 dark:text-white">AgentAPI</h2>
          <code class="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">agentapi/</code>
          <span class="badge badge-success">主站权威计费</span>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">代理站管理员可以核对本站记录，但不能编辑 Sub2API 的全局卫星计费配置。</p>
      </div>
      <div class="space-y-4 p-5 sm:p-6">
        <div class="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">计费模式</h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">当前模式由主站执行，本站只读。</p>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4" role="list">
          <article v-for="mode in billingModes" :key="mode.key" :class="['rounded-xl border p-4', mode.active ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700 dark:bg-primary-900/10' : 'border-gray-200 bg-gray-50 opacity-65 dark:border-dark-700 dark:bg-dark-800/50']" role="listitem">
            <div class="flex items-start justify-between gap-3">
              <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300"><Icon :name="mode.icon" size="sm" /></span>
              <Icon v-if="mode.active" name="checkCircle" size="sm" class="text-primary-600 dark:text-primary-300" />
            </div>
            <p class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">{{ mode.title }}</p>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ mode.description }}</p>
          </article>
        </div>
      </div>
    </section>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-label="计费统计">
      <article class="card p-5"><p class="text-sm text-gray-500">本站请求</p><p class="mt-2 text-2xl font-bold tabular-nums">{{ formatNumber(insights?.requests) }}</p><p class="mt-1 text-xs text-gray-400">所选时间范围</p></article>
      <article class="card p-5"><p class="text-sm text-gray-500">主站已报告费用</p><p class="mt-2 text-2xl font-bold tabular-nums text-green-600">{{ reportedActual ? formatUSD(insights?.actual_cost_usd_nanos) : '无完整数据' }}</p><p class="mt-1 text-xs text-gray-400">不以本站估算替代实际扣费</p></article>
      <article class="card p-5"><p class="text-sm text-gray-500">待确认记录</p><p class="mt-2 text-2xl font-bold tabular-nums text-amber-600">{{ formatNumber(pendingSettlements.length) }}</p><p class="mt-1 text-xs text-gray-400">可重新向主站核对</p></article>
      <article class="card p-5"><p class="text-sm text-gray-500">实际费用覆盖率</p><p class="mt-2 text-2xl font-bold tabular-nums">{{ actualCoverage.toFixed(1) }}%</p><p class="mt-1 text-xs text-gray-400">已测量记录中主站已返回实际费用的比例</p></article>
    </section>

    <section class="card overflow-hidden" aria-labelledby="billing-records-title">
      <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div><h2 id="billing-records-title" class="font-semibold text-gray-900 dark:text-white">最近计费记录</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">仅包含当前代理站留存并与主站核对的请求，共 {{ total }} 条。</p></div>
        <button type="button" class="btn btn-secondary btn-sm w-fit" :disabled="reconciling || !pendingSettlements.length" @click="reconcile">
          <Icon name="sync" size="sm" :class="{ 'animate-spin': reconciling }" />{{ reconciling ? '正在核对…' : '核对待确认记录' }}
        </button>
      </div>
      <DataTable
        :columns='[{"key":"request_id","label":"请求编号"},{"key":"user","label":"归属用户"},{"key":"model","label":"模型"},{"key":"actual","label":"主站实际费用"},{"key":"status","label":"状态"},{"key":"created_at","label":"时间"}]'
        :data="recentUsage"
        :loading="loading"
        row-key="request_id"
      >
        <template #cell-request_id="{ row }"><span class="font-mono text-xs">{{ row.request_id }}</span></template>
        <template #cell-user="{ row }"><span class="font-mono text-xs">{{ row.proxy_main_user_id }}</span></template>
        <template #cell-model="{ row }"><span class="font-mono text-xs">{{ row.model || '—' }}</span></template>
        <template #cell-actual="{ row }"><span :class="row.actual_cost_reported ? 'font-medium text-green-600' : 'text-gray-400'">{{ row.actual_cost_reported ? formatUSD(row.actual_cost_usd_nanos) : '待主站确认' }}</span></template>
        <template #cell-status="{ row }"><AgentStatusBadge :status="row.settlement_status" :label="statusLabel(row.settlement_status)" /></template>
        <template #cell-created_at="{ row }"><span class="whitespace-nowrap text-sm text-gray-500">{{ formatTime(row.created_at) }}</span></template>
        <template #empty><div class="py-10 text-center text-sm text-gray-500">{{ loadError ? '计费记录加载失败。' : '所选时间范围内暂无本站计费记录。' }}</div></template>
      </DataTable>
    </section>

    <section class="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-800 dark:border-sky-900/40 dark:bg-sky-900/10 dark:text-sky-200">
      <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />
      <p>数据边界：页面不会读取其他代理站或主站全局卫星配置，也不会把 SuperKey、管理员 Key、应用凭据或主站用户令牌发送到浏览器。价格调整、余额变化和最终账单仍由 Sub2API 主站处理。</p>
    </section>
  </main>
</template>
