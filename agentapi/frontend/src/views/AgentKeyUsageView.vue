<script setup lang="ts">
import { computed, ref } from 'vue'
import { siteLogo, siteName } from '@/agent/branding'
import { isDark, toggleTheme } from '@/agent/theme'
import AgentDateRangePicker from '@/components/common/AgentDateRangePicker.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import Icon from '@/components/icons/Icon.vue'

type RangeKey = 'today' | '7d' | '30d' | 'custom'
interface UsageStats {
  requests: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  actual_cost: number
}
interface DailyRow extends UsageStats { date: string; cache_write_tokens: number; cost: number }
interface ModelRow extends UsageStats { model: string }
interface KeyUsageResult {
  mode: string
  isValid: boolean
  planName: string
  balance: number
  remaining: number
  key?: { name?: string; prefix?: string }
  usage: { today: UsageStats; total: UsageStats; rpm: number; tpm: number }
  daily_usage: DailyRow[]
  model_stats: ModelRow[]
  status_detail?: string
}

const apiKey = ref('')
const keyVisible = ref(false)
const loading = ref(false)
const result = ref<KeyUsageResult | null>(null)
const error = ref('')
const currentRange = ref<RangeKey>('today')
const customStartDate = ref('')
const customEndDate = ref('')
const dailyDays = ref<7 | 30 | 90>(30)

const ranges: Array<{ key: RangeKey; label: string }> = [
  { key: 'today', label: '今天' }, { key: '7d', label: '近 7 天' },
  { key: '30d', label: '近 30 天' }, { key: 'custom', label: '自定义' },
]

function localDate(date: Date): string {
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 10)
}

function queryParams(): string {
  const params = new URLSearchParams({ days: String(dailyDays.value), timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' })
  const now = new Date()
  if (currentRange.value === 'custom') {
    if (customStartDate.value && customEndDate.value) {
      params.set('start_date', customStartDate.value)
      params.set('end_date', customEndDate.value)
    }
  } else {
    const back = currentRange.value === 'today' ? 0 : currentRange.value === '7d' ? 6 : 29
    params.set('start_date', localDate(new Date(now.getTime() - back * 86_400_000)))
    params.set('end_date', localDate(now))
  }
  return params.toString()
}

async function queryKey(): Promise<void> {
  if (loading.value) return
  const key = apiKey.value.trim()
  if (!key) {
    error.value = '请输入 API Key。'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const response = await fetch(`/v1/usage?${queryParams()}`, { headers: { Authorization: `Bearer ${key}` } })
    const payload = await response.json().catch(() => null) as (KeyUsageResult & { message?: string }) | null
    if (!response.ok || !payload) throw new Error(payload?.message || `查询失败（${response.status}）`)
    result.value = payload
  } catch (caught) {
    result.value = null
    error.value = caught instanceof Error ? caught.message : '查询失败，请稍后重试。'
  } finally {
    loading.value = false
  }
}

function setRange(range: RangeKey): void {
  currentRange.value = range
  if (range !== 'custom' && result.value) void queryKey()
}

function setDailyDays(days: 7 | 30 | 90): void {
  dailyDays.value = days
  if (result.value) void queryKey()
}

const statCells = computed(() => {
  if (!result.value) return []
  const today = result.value.usage.today
  const total = result.value.usage.total
  return [
    ['今日请求', formatNumber(today.requests)], ['今日输入 Token', formatNumber(today.input_tokens)],
    ['今日输出 Token', formatNumber(today.output_tokens)], ['今日费用', formatUSD(today.actual_cost)],
    ['区间请求', formatNumber(total.requests)], ['区间输入 Token', formatNumber(total.input_tokens)],
    ['区间输出 Token', formatNumber(total.output_tokens)], ['区间费用', formatUSD(total.actual_cost)],
  ]
})

function formatNumber(value?: number): string { return Number(value || 0).toLocaleString('zh-CN') }
function formatUSD(value?: number): string { return `$${Number(value || 0).toFixed(4)}` }
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/90 px-4 py-4 dark:border-dark-800 dark:bg-dark-950/90 sm:px-6">
      <nav class="mx-auto flex max-w-6xl items-center justify-between gap-4">
        <router-link to="/home" class="flex min-w-0 items-center gap-3 font-semibold">
          <img :src="siteLogo" alt="" class="h-10 w-10 shrink-0 rounded-xl object-contain" />
          <span class="truncate">{{ siteName }}</span>
        </router-link>
        <div class="flex items-center gap-2">
          <button class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800" type="button" aria-label="切换主题" @click="toggleTheme">
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
          <router-link to="/home" class="text-sm text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white">返回首页</router-link>
        </div>
      </nav>
    </header>

    <main class="mx-auto w-full max-w-5xl px-4 py-12 sm:px-6">
      <div class="text-center">
        <h1 class="text-3xl font-bold tracking-tight sm:text-4xl">Key 用量查询</h1>
        <p class="mx-auto mt-3 max-w-xl text-gray-500 dark:text-dark-400">输入本站创建的 API Key，查询该 Key 的用量与费用。</p>
      </div>

      <section class="mx-auto mt-10 max-w-2xl">
        <div class="flex flex-col gap-3 sm:flex-row">
          <div class="min-w-0 flex-1">
            <AgentInput v-model="apiKey" aria-label="API Key" :type="keyVisible ? 'text' : 'password'" autocomplete="off" placeholder="sk-..." class="h-12 rounded-xl" @enter="queryKey">
              <template #suffix>
                <button type="button" class="rounded p-1 text-gray-400" :aria-label="keyVisible ? '隐藏 Key' : '显示 Key'" @click="keyVisible = !keyVisible">
                  <Icon :name="keyVisible ? 'eyeOff' : 'eye'" size="sm" />
                </button>
              </template>
            </AgentInput>
          </div>
          <button type="button" :disabled="loading" class="h-12 rounded-xl bg-primary-600 px-7 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60" @click="queryKey">
            {{ loading ? '查询中…' : '查询' }}
          </button>
        </div>
        <p class="mt-3 text-center text-xs text-gray-400">Key 仅发送到本站，不会写入浏览器存储，也不会被转发或泄露。</p>
        <p v-if="error" role="alert" class="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      </section>

      <template v-if="result">
        <section class="mt-8 flex flex-wrap items-center justify-center gap-2">
          <button v-for="range in ranges" :key="range.key" type="button" class="rounded-lg border px-3 py-1.5 text-xs font-medium" :class="currentRange === range.key ? 'border-primary-600 bg-primary-600 text-white' : 'border-gray-200 bg-white text-gray-600 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300'" @click="setRange(range.key)">{{ range.label }}</button>
          <template v-if="currentRange === 'custom'">
            <AgentDateRangePicker
              v-model:start-date="customStartDate"
              v-model:end-date="customEndDate"
              aria-label="Key 用量自定义日期范围"
              @change="queryKey"
            />
          </template>
        </section>

        <section class="mt-6 grid gap-4 md:grid-cols-3">
          <article class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-800 dark:bg-dark-900">
            <p class="text-xs uppercase tracking-wider text-gray-400">Key 状态</p>
            <p class="mt-3 text-xl font-semibold text-emerald-600">正常</p>
            <p class="mt-1 text-sm text-gray-500">{{ result.key?.name || 'AgentAPI Key' }} · {{ result.key?.prefix }}…</p>
          </article>
          <article class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-800 dark:bg-dark-900">
            <p class="text-xs uppercase tracking-wider text-gray-400">账户余额</p>
            <p class="mt-3 text-3xl font-bold tabular-nums">{{ formatUSD(result.balance) }}</p>
            <p class="mt-1 text-sm text-gray-500">按本站统一计费规则结算</p>
          </article>
          <article class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-800 dark:bg-dark-900">
            <p class="text-xs uppercase tracking-wider text-gray-400">数据完整度</p>
            <p class="mt-3 text-xl font-semibold">{{ result.status_detail === 'partial' ? '部分同步' : result.status_detail === 'empty' ? '暂无用量' : '已同步' }}</p>
            <p class="mt-1 text-sm text-gray-500">展示本站已记录的权威用量快照</p>
          </article>
        </section>

        <section class="mt-6 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900">
          <div class="border-b border-gray-200 px-6 py-4 dark:border-dark-800"><h2 class="font-semibold">Token 与费用统计</h2></div>
          <div class="grid grid-cols-2 gap-px bg-gray-100 md:grid-cols-4 dark:bg-dark-800">
            <div v-for="cell in statCells" :key="cell[0]" class="bg-white px-5 py-4 dark:bg-dark-900">
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ cell[0] }}</p><p class="mt-1 font-semibold tabular-nums">{{ cell[1] }}</p>
            </div>
          </div>
        </section>

        <section class="mt-6 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-6 py-4 dark:border-dark-800">
            <h2 class="font-semibold">每日用量</h2>
            <div class="flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-700">
              <button v-for="days in ([7, 30, 90] as const)" :key="days" type="button" class="rounded-md px-3 py-1 text-xs" :class="dailyDays === days ? 'bg-primary-600 text-white' : 'text-gray-500'" @click="setDailyDays(days)">{{ days }} 天</button>
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full min-w-[720px] text-sm">
              <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-950"><tr><th class="px-4 py-3 text-left">日期</th><th class="px-4 py-3 text-right">请求</th><th class="px-4 py-3 text-right">输入</th><th class="px-4 py-3 text-right">输出</th><th class="px-4 py-3 text-right">缓存读取</th><th class="px-4 py-3 text-right">缓存写入</th><th class="px-4 py-3 text-right">费用</th></tr></thead>
              <tbody><tr v-for="row in result.daily_usage" :key="row.date" class="border-t border-gray-100 dark:border-dark-800"><td class="px-4 py-3 font-medium">{{ row.date }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.requests) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.input_tokens) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.output_tokens) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.cache_read_tokens) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.cache_write_tokens) }}</td><td class="px-4 py-3 text-right font-medium">{{ formatUSD(row.actual_cost) }}</td></tr></tbody>
            </table>
            <p v-if="result.daily_usage.length === 0" class="px-6 py-8 text-center text-sm text-gray-500">暂无每日用量</p>
          </div>
        </section>

        <section v-if="result.model_stats.length" class="mt-6 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900">
          <div class="border-b border-gray-200 px-6 py-4 dark:border-dark-800"><h2 class="font-semibold">模型统计</h2></div>
          <div class="overflow-x-auto"><table class="w-full min-w-[680px] text-sm"><thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-950"><tr><th class="px-4 py-3 text-left">模型</th><th class="px-4 py-3 text-right">请求</th><th class="px-4 py-3 text-right">输入 Token</th><th class="px-4 py-3 text-right">输出 Token</th><th class="px-4 py-3 text-right">总 Token</th><th class="px-4 py-3 text-right">费用</th></tr></thead><tbody><tr v-for="row in result.model_stats" :key="row.model" class="border-t border-gray-100 dark:border-dark-800"><td class="px-4 py-3 font-medium">{{ row.model }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.requests) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.input_tokens) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.output_tokens) }}</td><td class="px-4 py-3 text-right">{{ formatNumber(row.total_tokens) }}</td><td class="px-4 py-3 text-right font-medium">{{ formatUSD(row.actual_cost) }}</td></tr></tbody></table></div>
        </section>
      </template>
    </main>
  </div>
</template>
