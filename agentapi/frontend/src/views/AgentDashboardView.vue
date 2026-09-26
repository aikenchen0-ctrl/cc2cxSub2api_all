<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { agentAPI, isAuthoritativeAgentUsageSource, type AgentContextResponse, type AgentUsageView } from '@/agent/api'
import { enumLabel, statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'

const context = ref<AgentContextResponse | null>(null)
const recentUsage = ref<AgentUsageView[]>([])
const loading = ref(true)
const usageLoading = ref(true)
const error = ref('')
const usageError = ref('')
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

function localSettlementLabel(item: AgentUsageView): string {
  if (item.settlement_status === 'confirmed') return `本地扣款 ${money(item.actual_cents)}`
  if (item.settlement_status === 'released') return `预留金额已退回 · ${money(item.reserved_cents)}`
  return `预留金额 · ${money(item.reserved_cents)}`
}

function sub2ApiUsageLabel(item: AgentUsageView): string {
  if (item.usage_source === 'owner_balance_delta_fallback') return '旧版主账户余额差额估算；暂无单次请求用量'
  if (!isAuthoritativeAgentUsageSource(item.usage_source)) return '暂无单次请求的 Sub2API 用量详情'
  if (item.actual_cost_reported) return `Sub2API 实际费用 ${usd(item.actual_cost_usd_nanos)} · 标准费用 ${usd(item.total_cost_usd_nanos)}`
  return `暂无 Sub2API 实际费用 · 标准费用 ${usd(item.total_cost_usd_nanos)}`
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

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">{{ context?.agent.site_name || 'AgentAPI' }}</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">总览</h1>
        <p class="mt-1 text-sm text-slate-500">AgentAPI 余额是本地子余额。模型请求由主站上的代理站主账户计费。</p>
      </div>
      <div class="flex gap-2">
        <RouterLink to="/recharge" class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white">充值</RouterLink>
        <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">刷新</button>
      </div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="loading" class="text-sm text-slate-500">正在加载…</p>

    <section v-if="context" class="grid gap-4 md:grid-cols-3">
      <div v-if="context.user" class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">可用余额</p>
        <p class="mt-2 text-2xl font-semibold">{{ money(context.user.balance_cents) }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">代理站</p>
        <p class="mt-2 text-lg font-semibold">{{ context.agent.name }}</p>
        <p class="text-sm text-slate-500">{{ context.agent.domain || '域名待配置' }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">计费方式</p>
        <p class="mt-2 text-lg font-semibold">{{ enumLabel(context.agent.billing_mode) }}</p>
        <p class="text-sm text-slate-500">不收取佣金，也不使用独立上游账户。</p>
      </div>
    </section>

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
              <span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-300">{{ statusLabel(item.settlement_status) }}</span>
            </div>
            <p class="mt-1 truncate font-mono text-xs text-slate-500" :title="item.request_id">{{ item.request_id }}</p>
            <p class="mt-1 text-xs text-slate-500">{{ date(item.created_at) }}<span v-if="item.inbound_endpoint"> · {{ item.inbound_endpoint }}</span></p>
            <p v-if="isAuthoritativeAgentUsageSource(item.usage_source)" class="mt-1 text-xs text-slate-600 dark:text-slate-300">
              输入 {{ item.input_tokens.toLocaleString('zh-CN') }} · 输出 {{ item.output_tokens.toLocaleString('zh-CN') }}
            </p>
            <p class="mt-1 text-xs text-slate-500">{{ sub2ApiUsageLabel(item) }}</p>
          </div>
          <p class="shrink-0 text-sm font-medium text-slate-700 dark:text-slate-200">{{ localSettlementLabel(item) }}</p>
        </article>
      </div>
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold">计费说明</h2>
      <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">
        你的账号是 Sub2API 主站中的正式账号。AgentAPI 仅维护账号关联和子余额；主站余额及用量账本为最终计费依据。如果暂时无法确认上游扣款，请求会保持待核对状态，不会被静默退款。
      </p>
    </section>
  </main>
</template>
