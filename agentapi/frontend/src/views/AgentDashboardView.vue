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
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">{{ context?.agent.site_name || 'AgentAPI' }}</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">总览</h1>
        <p class="mt-1 text-sm text-slate-500">余额、模型费用和请求记录会在本站控制台统一展示。</p>
      </div>
      <div class="flex gap-2">
        <RouterLink to="/purchase" class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white">充值</RouterLink>
        <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="refreshDashboard">刷新</button>
      </div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="loading" class="text-sm text-slate-500">正在加载…</p>

    <section v-if="context" class="grid grid-cols-1 gap-6 md:grid-cols-3">
      <StatCard v-if="context.user" title="可用余额" :value="context.balance_error ? '暂不可用' : money(context.user.balance_cents)" :icon="balanceIcon" />
      <StatCard title="代理站" :value="context.agent.site_name" :icon="siteIcon" icon-variant="success" />
      <StatCard title="计费方式" value="账户余额计费" :icon="billingIcon" icon-variant="warning" />
    </section>
    <p v-if="context?.balance_error" class="text-sm text-amber-600">余额读取失败，请刷新重试。</p>

    <AgentUsageInsights :key="insightsVersion" />

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div class="lg:col-span-2">
        <section v-if="context" class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b p-5 dark:border-slate-700">
            <div>
              <h2 class="font-semibold">最近用量</h2>
              <p class="mt-1 text-sm text-slate-500">AgentAPI 最近记录的 5 次请求。</p>
            </div>
            <RouterLink to="/usage" class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400">查看全部用量</RouterLink>
          </div>
          <p v-if="usageError" class="m-5 rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ usageError }}</p>
          <p v-else-if="usageLoading" class="p-5 text-sm text-slate-500">正在加载最近用量…</p>
          <p v-else-if="recentUsage.length === 0" class="p-5 text-sm text-slate-500">暂无模型请求记录，最近的请求会显示在这里。</p>
          <div v-else class="divide-y dark:divide-slate-700">
            <article v-for="item in recentUsage" :key="item.request_id" class="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-medium">{{ item.model || '未知模型' }}</span>
                  <AgentStatusBadge :status="item.settlement_status" :label="statusLabel(item.settlement_status)" />
                </div>
                <p class="mt-1 truncate font-mono text-xs text-slate-500" :title="item.request_id">{{ item.request_id }}</p>
                <p class="mt-1 text-xs text-slate-500">{{ date(item.created_at) }}<span v-if="item.inbound_endpoint"> · {{ item.inbound_endpoint }}</span></p>
                <p v-if="isAuthoritativeAgentUsageSource(item.usage_source)" class="mt-1 text-xs text-slate-600 dark:text-slate-300">
                  输入 {{ item.input_tokens.toLocaleString('zh-CN') }} · 输出 {{ item.output_tokens.toLocaleString('zh-CN') }}
                </p>
                <p class="mt-1 text-xs text-slate-500">{{ sub2ApiUsageLabel(item) }}</p>
              </div>
              <p class="shrink-0 text-sm font-medium text-slate-700 dark:text-slate-200">{{ settlementLabel(item) }}</p>
            </article>
          </div>
        </section>
      </div>
      <div class="lg:col-span-1">
        <UserDashboardQuickActions />
      </div>
    </div>
    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold">计费说明</h2>
      <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">
        本站会统一维护你的站点归属、API Key 和请求记录。余额、用量与扣费由系统核算；暂时无法确认的单次用量会保持待核对状态，不会重复收费。
      </p>
    </section>
  </main>
</template>
