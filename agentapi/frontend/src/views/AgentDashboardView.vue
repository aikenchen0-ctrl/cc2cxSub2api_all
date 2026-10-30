<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { agentAPI, isAuthoritativeAgentUsageSource, type AgentContextResponse, type AgentUsageView } from '@/agent/api'
import { statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'

import StatCard from '@/components/common/StatCard.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import AgentUsageInsights from '@/components/AgentUsageInsights.vue'
import Icon from '@/components/icons/Icon.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
const balanceIcon = () => h(Icon, { name: 'creditCard', size: 'lg' })
const siteIcon = () => h(Icon, { name: 'home', size: 'lg' })
const billingIcon = () => h(Icon, { name: 'shield', size: 'lg' })

const context = ref<AgentContextResponse | null>(null)
const recentUsage = ref<AgentUsageView[]>([])
const loading = ref(true)
const usageLoading = ref(true)
const error = ref('')
const usageError = ref('')
const insightsVersion = ref(0)
let loadSequence = 0

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

function usd(nanos: number): string {
  const [whole, fraction = ''] = (Math.max(0, nanos) / 1_000_000_000).toFixed(9).split('.')
  return `$${whole}.${fraction.replace(/0+$/, '').padEnd(2, '0')}`
}

function date(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

function settlementLabel(item: AgentUsageView): string {
  if (item.settlement_status === 'confirmed') return `实际扣费 ${money(item.actual_cents)}`
  if (item.settlement_status === 'released') return `请求前校验已释放 · ${money(item.reserved_cents)}`
  return `请求前余额校验 · ${money(item.reserved_cents)}`
}

function sub2ApiUsageLabel(item: AgentUsageView): string {
  if (item.usage_source === 'user_balance_delta_fallback') return '账户余额差额估算；暂无单次请求用量'
  if (item.usage_source === 'owner_balance_delta_fallback') return '旧版主账户余额差额估算；暂无单次请求用量'
  if (!isAuthoritativeAgentUsageSource(item.usage_source)) return '暂无单次请求的用量详情'
  if (item.actual_cost_reported) return `实际费用 ${usd(item.actual_cost_usd_nanos)} · 标准费用 ${usd(item.total_cost_usd_nanos)}`
  return `实际费用待确认 · 标准费用 ${usd(item.total_cost_usd_nanos)}`
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  usageLoading.value = true
  error.value = ''
  usageError.value = ''

  // The context already contains the current mapped user's balance, so do not
  // make a second wallet request for the same dashboard. Load recent usage
  // independently so a usage API outage cannot hide the site's core details.
  const contextRequest = agentAPI.getContext()
    .then((nextContext) => {
      if (sequence === loadSequence) context.value = nextContext
    })
    .catch((err) => {
      if (sequence === loadSequence) {
        error.value = errorMessage(err, '加载总览失败，请稍后刷新重试。')
      }
    })
    .finally(() => {
      if (sequence === loadSequence) loading.value = false
    })

  const usageRequest = agentAPI.getUsage(1, 5)
    .then((usage) => {
      if (sequence === loadSequence) recentUsage.value = usage.items
    })
    .catch((err) => {
      if (sequence === loadSequence) {
        recentUsage.value = []
        usageError.value = errorMessage(err, '加载最近用量失败，请稍后刷新重试。')
      }
    })
    .finally(() => {
      if (sequence === loadSequence) usageLoading.value = false
    })

  await Promise.all([contextRequest, usageRequest])
}

async function refreshDashboard(): Promise<void> {
  insightsVersion.value += 1
  await load()
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="relative overflow-hidden rounded-2xl border border-primary-200/70 bg-white/85 p-5 shadow-card backdrop-blur-xl dark:border-primary-500/15 dark:bg-dark-900/70 sm:p-6">
      <div class="pointer-events-none absolute -right-12 -top-16 h-48 w-48 rounded-full bg-primary-300/25 blur-3xl dark:bg-primary-500/10"></div>
      <div class="pointer-events-none absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-primary-500/60 to-transparent"></div>
      <div class="relative flex flex-wrap items-end justify-between gap-5">
        <div class="max-w-2xl">
          <p class="text-[11px] font-semibold uppercase tracking-[0.24em] text-primary-700 dark:text-primary-300">Site overview</p>
          <h1 class="mt-2 text-2xl font-bold tracking-tight text-accent-950 dark:text-white">欢迎回来</h1>
          <p class="mt-2 text-sm leading-6 text-accent-600 dark:text-dark-300">
            {{ context?.agent.site_name || 'AgentAPI' }} · 余额、模型费用与请求记录都集中在这里。
          </p>
        </div>
        <div class="flex gap-2">
          <RouterLink to="/purchase" class="btn btn-primary">充值</RouterLink>
          <button class="btn btn-secondary" type="button" @click="refreshDashboard">刷新</button>
        </div>
      </div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="loading" class="text-sm text-accent-500 dark:text-dark-400">正在加载…</p>

    <section v-if="context" class="grid grid-cols-1 gap-6 md:grid-cols-3">
      <StatCard v-if="context.user" title="可用余额" :value="context.balance_error ? '暂不可用' : money(context.user.balance_cents)" :icon="balanceIcon" />
      <StatCard title="代理站" :value="context.agent.site_name" :icon="siteIcon" icon-variant="primary" />
      <StatCard title="计费方式" value="账户余额计费" :icon="billingIcon" icon-variant="warning" />
    </section>
    <p v-if="context?.balance_error" class="text-sm text-amber-600">余额读取失败，请刷新重试。</p>

    <AgentUsageInsights :key="insightsVersion" />

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div class="lg:col-span-2">
        <section v-if="context" class="card overflow-hidden">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-accent-200/70 px-5 py-4 dark:border-dark-700">
            <div>
              <h2 class="font-semibold text-accent-950 dark:text-white">最近用量</h2>
              <p class="mt-1 text-sm text-accent-500 dark:text-dark-400">AgentAPI 最近记录的 5 次请求。</p>
            </div>
            <RouterLink to="/usage" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400">查看全部用量</RouterLink>
          </div>
          <p v-if="usageError" class="m-5 rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ usageError }}</p>
          <p v-else-if="usageLoading" class="p-5 text-sm text-accent-500 dark:text-dark-400">正在加载最近用量…</p>
          <div v-else-if="recentUsage.length === 0" class="flex min-h-44 flex-col items-center justify-center px-5 py-8 text-center">
            <span class="flex h-11 w-11 items-center justify-center rounded-full border border-primary-200 bg-primary-50 text-primary-600 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="clock" size="lg" /></span>
            <p class="mt-3 text-sm text-accent-500 dark:text-dark-400">暂无模型请求记录，最近的请求会显示在这里。</p>
          </div>
          <div v-else class="divide-y divide-accent-100 dark:divide-dark-800">
            <article v-for="item in recentUsage" :key="item.request_id" class="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-medium text-accent-950 dark:text-white">{{ item.model || '未知模型' }}</span>
                  <AgentStatusBadge :status="item.settlement_status" :label="statusLabel(item.settlement_status)" />
                </div>
                <p class="mt-1 truncate font-mono text-xs text-accent-400" :title="item.request_id">{{ item.request_id }}</p>
                <p class="mt-1 text-xs text-accent-500 dark:text-dark-400">{{ date(item.created_at) }}<span v-if="item.inbound_endpoint"> · {{ item.inbound_endpoint }}</span></p>
                <p v-if="isAuthoritativeAgentUsageSource(item.usage_source)" class="mt-1 text-xs text-accent-600 dark:text-accent-300">
                  输入 {{ item.input_tokens.toLocaleString('zh-CN') }} · 输出 {{ item.output_tokens.toLocaleString('zh-CN') }}
                </p>
                <p class="mt-1 text-xs text-accent-500 dark:text-dark-400">{{ sub2ApiUsageLabel(item) }}</p>
              </div>
              <p class="shrink-0 text-sm font-medium text-accent-700 dark:text-accent-200">{{ settlementLabel(item) }}</p>
            </article>
          </div>
        </section>
      </div>
      <div class="lg:col-span-1">
        <UserDashboardQuickActions />
      </div>
    </div>
    <section class="card flex gap-4 p-5">
      <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-100 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="shield" size="lg" /></span>
      <div>
        <h2 class="font-semibold text-accent-950 dark:text-white">计费说明</h2>
        <p class="mt-2 text-sm leading-6 text-accent-600 dark:text-accent-300">
          本站会统一维护你的站点归属、API Key 和请求记录。余额、用量与扣费由系统核算；暂时无法确认的单次用量会保持待核对状态，不会重复收费。
        </p>
      </div>
    </section>
  </main>
</template>
