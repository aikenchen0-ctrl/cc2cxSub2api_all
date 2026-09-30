<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { agentAPI, type AgentAdminPaymentDashboard, type AgentCurrencyAmounts } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentDailyRevenueChart from './AgentDailyRevenueChart.vue'
import AgentPaymentStatsCards from './AgentPaymentStatsCards.vue'
import Icon from '@/components/icons/Icon.vue'

const daysOptions = [7, 30, 90] as const
const days = ref<7 | 30 | 90>(30)
const stats = ref<AgentAdminPaymentDashboard | null>(null)
const loading = ref(false)
const error = ref('')
let loadSequence = 0

const topUserGroups = computed(() => Object.entries(stats.value?.top_users || {}).sort(([left], [right]) => left.localeCompare(right)))

function sortedAmounts(amounts: AgentCurrencyAmounts) {
  return Object.entries(amounts || {}).sort(([left], [right]) => left.localeCompare(right))
}

function money(currency: string, amount: number) {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(amount) }
  catch { return `${currency} ${Number(amount || 0).toFixed(2)}` }
}

function methodName(method: string) {
  const names: Record<string, string> = { alipay: '支付宝', alipay_direct: '支付宝直连', wxpay: '微信支付', wxpay_direct: '微信直连', stripe: 'Stripe', airwallex: 'Airwallex', unknown: '其他' }
  return names[method] || method
}

function methodClass(method: string) {
  if (method.includes('wxpay')) return 'bg-emerald-500'
  if (method.includes('alipay')) return 'bg-blue-500'
  if (method === 'stripe') return 'bg-purple-500'
  if (method === 'airwallex') return 'bg-cyan-500'
  return 'bg-gray-400'
}

function rankClass(index: number) {
  if (index === 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  if (index === 1) return 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  if (index === 2) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

async function load() {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.adminOrders.dashboard(days.value)
    if (sequence === loadSequence) stats.value = result
  } catch (cause) {
    if (sequence === loadSequence) error.value = errorMessage(cause, '支付统计加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(days, () => { void load() })
onMounted(() => { void load() })
</script>

<template>
  <section class="space-y-6" aria-label="本站支付统计">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="max-w-2xl text-sm text-gray-500 dark:text-gray-400">统计仅聚合当前代理站已登记用户在 Sub2API 主站产生的支付订单，不包含其他代理站或主站全局数据。</p>
      <div class="flex items-center gap-2">
        <div class="flex rounded-lg border border-gray-200 dark:border-dark-600" aria-label="统计周期">
          <button v-for="option in daysOptions" :key="option" type="button" class="px-3 py-1.5 text-xs font-medium transition-colors first:rounded-l-lg last:rounded-r-lg" :class="days === option ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'" @click="days = option">{{ option }} 天</button>
        </div>
        <button class="btn btn-secondary" type="button" :disabled="loading" aria-label="刷新支付统计" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" /></button>
      </div>
    </div>

    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>
    <div v-if="loading && !stats" class="card flex min-h-52 items-center justify-center text-sm text-gray-500">正在加载支付统计…</div>
    <template v-else-if="stats">
      <AgentPaymentStatsCards :stats="stats" />
      <AgentDailyRevenueChart :data="stats.daily_series || []" />

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div class="card p-4">
          <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">支付方式分布</h3>
          <div v-if="!stats.payment_methods?.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无支付数据</div>
          <div v-else class="space-y-3">
            <div v-for="method in stats.payment_methods" :key="method.type" class="flex items-center justify-between gap-4">
              <div class="flex items-center gap-2"><span class="inline-block h-3 w-3 rounded-full" :class="methodClass(method.type)" /><span class="text-sm text-gray-700 dark:text-gray-300">{{ methodName(method.type) }}</span></div>
              <div class="text-right"><span v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="block text-sm font-medium text-gray-900 dark:text-white">{{ money(currency, amount) }}</span><span class="text-xs text-gray-500 dark:text-gray-400">{{ method.count }} 笔</span></div>
            </div>
          </div>
        </div>

        <div class="card p-4">
          <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">本站支付用户排行</h3>
          <div v-if="!topUserGroups.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无支付数据</div>
          <div v-else class="space-y-4">
            <div v-for="[currency, users] in topUserGroups" :key="currency" class="space-y-2">
              <p class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ currency }}</p>
              <div v-for="(user, index) in users" :key="user.main_user_id" class="flex items-center justify-between gap-3 rounded-lg px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700">
                <div class="flex min-w-0 items-center gap-3"><span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold" :class="rankClass(index)">{{ index + 1 }}</span><div class="min-w-0"><p class="truncate text-sm text-gray-700 dark:text-gray-300">{{ user.name || user.email || user.main_user_id }}</p><p class="truncate font-mono text-[11px] text-gray-400">ID {{ user.main_user_id }}</p></div></div>
                <span class="shrink-0 text-sm font-medium text-gray-900 dark:text-white">{{ money(currency, user.amount) }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>
