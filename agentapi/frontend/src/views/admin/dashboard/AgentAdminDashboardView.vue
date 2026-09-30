<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse, type UsageInsights } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const insights = ref<UsageInsights | null>(null)
const totalUsers = ref(0)
const activeUsers = ref(0)
const pendingSettlements = ref(0)
const loading = ref(true)
const error = ref('')
const partialFailures = ref<string[]>([])
const lastUpdated = ref<Date | null>(null)
let loadSequence = 0

const totalTokens = computed(() => {
  if (!insights.value) return 0
  return insights.value.input_tokens
    + insights.value.output_tokens
    + insights.value.cache_creation_tokens
    + insights.value.cache_read_tokens
})

const actualCostLabel = computed(() => {
  if (!insights.value || insights.value.status === 'empty' || insights.value.measured <= insights.value.missing_actual) return '无数据'
  return formatUSD(insights.value.actual_cost_usd_nanos)
})

const coverageLabel = computed(() => {
  if (!insights.value?.requests) return '无记录'
  return `${insights.value.measured.toLocaleString()} / ${insights.value.requests.toLocaleString()}`
})

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  partialFailures.value = []

  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    error.value = errorMessage(cause, '代理站上下文加载失败，暂时不能安全展示站长仪表盘。')
    loading.value = false
    return
  }

  const results = await Promise.allSettled([
    agentAPI.getUsers(1, 1),
    agentAPI.getUsers(1, 1, '', 'active'),
    agentAPI.getUsageInsights('24h'),
    agentAPI.getSettlements(),
  ])
  if (sequence !== loadSequence) return

  const [usersResult, activeUsersResult, insightsResult, settlementsResult] = results
  if (usersResult.status === 'fulfilled') totalUsers.value = usersResult.value.total
  else partialFailures.value.push('用户总量')
  if (activeUsersResult.status === 'fulfilled') activeUsers.value = activeUsersResult.value.total
  else partialFailures.value.push('活跃用户')
  if (insightsResult.status === 'fulfilled') insights.value = insightsResult.value
  else partialFailures.value.push('24 小时用量')
  if (settlementsResult.status === 'fulfilled') pendingSettlements.value = settlementsResult.value.total
  else partialFailures.value.push('待确认结算')

  lastUpdated.value = new Date()
  loading.value = false
}

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

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">本站管理</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">站长仪表盘</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">查看当前代理站的用户、请求和主站计费快照；所有统计均固定在当前代理站范围内。</p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
        <span v-if="lastUpdated" class="text-xs text-gray-400">更新于 {{ lastUpdated.toLocaleTimeString('zh-CN') }}</span>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新
        </button>
      </div>
    </header>

    <div class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-300">
      余额、实际费用和结算状态仍以 Sub2API 主站为权威。此页面不会读取主站全局用户、账号池、渠道凭据或管理员配置，也不能修改主站全局设置。
    </div>

    <div v-if="loading && !context" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在读取本站统计…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载站长仪表盘</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <p v-if="partialFailures.length" role="alert" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300">
        部分数据暂不可用：{{ partialFailures.join('、') }}。其余本站统计仍可使用。
      </p>

      <section class="grid grid-cols-2 gap-4 lg:grid-cols-4" aria-label="核心统计">
        <div class="card flex items-center gap-3 p-4">
          <div class="rounded-lg bg-emerald-100 p-2 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400"><Icon name="users" size="md" /></div>
          <div>
            <p class="text-xs font-medium text-gray-500">本站用户</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ totalUsers.toLocaleString() }}</p>
            <p class="text-xs text-emerald-600">{{ activeUsers.toLocaleString() }} 名已启用</p>
          </div>
        </div>
        <div class="card flex items-center gap-3 p-4">
          <div class="rounded-lg bg-blue-100 p-2 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400"><Icon name="chart" size="md" /></div>
          <div>
            <p class="text-xs font-medium text-gray-500">24 小时请求</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ (insights?.requests || 0).toLocaleString() }}</p>
            <p class="text-xs text-gray-400">本站精确时间窗口</p>
          </div>
        </div>
        <div class="card flex items-center gap-3 p-4">
          <div class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400"><Icon name="cube" size="md" /></div>
          <div>
            <p class="text-xs font-medium text-gray-500">已观测令牌</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(totalTokens) }}</p>
            <p class="text-xs text-gray-400">仅含本站主站快照</p>
          </div>
        </div>
        <div class="card flex items-center gap-3 p-4">
          <div class="rounded-lg bg-green-100 p-2 text-green-600 dark:bg-green-900/30 dark:text-green-400"><Icon name="dollar" size="md" /></div>
          <div>
            <p class="text-xs font-medium text-gray-500">已报告实际费用</p>
            <p class="text-xl font-bold text-green-600">{{ actualCostLabel }}</p>
            <p class="text-xs text-gray-400">主站报告值，不以零值补齐</p>
          </div>
        </div>
      </section>

      <section class="grid gap-4 md:grid-cols-2 xl:grid-cols-4" aria-label="运行状态">
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">代理站状态</p>
          <div class="mt-2 flex items-center gap-2"><AgentStatusBadge :status="context.agent.status" :label="context.agent.status === 'active' ? '运行中' : context.agent.status" /></div>
          <p class="mt-2 truncate text-xs text-gray-400">{{ context.agent.domain || '域名待配置' }}</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">计费方式</p>
          <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">用户主站直扣</p>
          <p class="mt-2 text-xs text-gray-400">代理站不维护共享余额账本</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">待主站确认</p>
          <p class="mt-2 text-2xl font-bold text-amber-600">{{ pendingSettlements.toLocaleString() }}</p>
          <p class="mt-1 text-xs text-gray-400">本站待核对结算记录</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-medium text-gray-500">主站明细覆盖</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ coverageLabel }}</p>
          <p class="mt-1 text-xs text-gray-400">{{ insights?.status === 'partial' ? '部分可用' : insights?.status === 'measured' ? '实测' : '暂无数据' }}</p>
        </div>
      </section>

      <section class="card p-5" aria-label="快捷操作">
        <div class="mb-4">
          <h2 class="font-semibold text-gray-900 dark:text-white">快捷操作</h2>
          <p class="mt-1 text-xs text-gray-500">沿用主站仪表盘入口，跳转到当前代理站可安全管理的功能。</p>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <RouterLink to="/admin/users" class="group flex items-center gap-3 rounded-lg bg-gray-50 p-4 transition-colors hover:bg-emerald-50 dark:bg-dark-800/50 dark:hover:bg-emerald-900/20">
            <span class="rounded-lg bg-emerald-100 p-2 text-emerald-600 dark:bg-emerald-900/30"><Icon name="users" size="md" /></span>
            <span class="min-w-0 flex-1"><span class="block text-sm font-medium">用户管理</span><span class="block text-xs text-gray-500">本站映射与 API Key</span></span><Icon name="chevronRight" size="sm" class="text-gray-400" />
          </RouterLink>
          <RouterLink to="/admin/usage" class="group flex items-center gap-3 rounded-lg bg-gray-50 p-4 transition-colors hover:bg-blue-50 dark:bg-dark-800/50 dark:hover:bg-blue-900/20">
            <span class="rounded-lg bg-blue-100 p-2 text-blue-600 dark:bg-blue-900/30"><Icon name="chartBar" size="md" /></span>
            <span class="min-w-0 flex-1"><span class="block text-sm font-medium">用量同步</span><span class="block text-xs text-gray-500">主站事实与结算核对</span></span><Icon name="chevronRight" size="sm" class="text-gray-400" />
          </RouterLink>
          <RouterLink to="/admin/channels" class="group flex items-center gap-3 rounded-lg bg-gray-50 p-4 transition-colors hover:bg-purple-50 dark:bg-dark-800/50 dark:hover:bg-purple-900/20">
            <span class="rounded-lg bg-purple-100 p-2 text-purple-600 dark:bg-purple-900/30"><Icon name="server" size="md" /></span>
            <span class="min-w-0 flex-1"><span class="block text-sm font-medium">渠道目录</span><span class="block text-xs text-gray-500">本站可用模型范围</span></span><Icon name="chevronRight" size="sm" class="text-gray-400" />
          </RouterLink>
          <RouterLink to="/admin/settings" class="group flex items-center gap-3 rounded-lg bg-gray-50 p-4 transition-colors hover:bg-amber-50 dark:bg-dark-800/50 dark:hover:bg-amber-900/20">
            <span class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30"><Icon name="cog" size="md" /></span>
            <span class="min-w-0 flex-1"><span class="block text-sm font-medium">本站设置</span><span class="block text-xs text-gray-500">品牌与租户配置</span></span><Icon name="chevronRight" size="sm" class="text-gray-400" />
          </RouterLink>
        </div>
      </section>
    </template>
  </main>
</template>
