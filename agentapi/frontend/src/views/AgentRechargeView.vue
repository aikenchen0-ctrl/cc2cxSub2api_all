<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type RechargeOrder } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import { currencyLabel, providerLabel, statusLabel } from '@/agent/locale'

const orders = ref<RechargeOrder[]>([])
const enabled = ref(false)
const provider = ref('manual')
const currency = ref('CNY')
const minAmount = ref(1)
const maxAmount = ref(1000000)
const amount = ref('')
const amountInput = ref<HTMLInputElement | null>(null)
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const notice = ref('')

function money(cents: number, code = currency.value): string {
  return `${(Math.max(0, cents) / 100).toFixed(2)} ${currencyLabel(code)}`
}

function date(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

function statusClass(status: string): string {
  if (status === 'allocated') return 'text-emerald-600'
  if (status === 'paid_pending_allocation') return 'text-amber-600'
  if (status === 'failed' || status === 'closed' || status === 'expired') return 'text-red-600'
  return 'text-slate-500'
}

function idempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return `recharge-${crypto.randomUUID()}`
  return `recharge-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.recharge.list()
    enabled.value = result.enabled
    provider.value = result.provider || 'manual'
    currency.value = result.currency || 'CNY'
    minAmount.value = result.min_amount_cents || 1
    maxAmount.value = result.max_amount_cents || 1000000
    orders.value = result.items
  } catch (err) {
    error.value = errorMessage(err, '加载充值订单失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function createOrder(): Promise<void> {
  error.value = ''
  notice.value = ''
  if (!amount.value.trim()) {
    error.value = '请输入充值金额。'
    return
  }
  if (!Number.isFinite(Number(amount.value)) || Number(amount.value) < 0.01) {
    error.value = '请输入不低于 0.01 的有效充值金额。'
    return
  }
  if (amountInput.value?.validity.stepMismatch) {
    error.value = '充值金额最多保留两位小数。'
    return
  }
  submitting.value = true
  try {
    const result = await agentAPI.recharge.create(amount.value.trim(), idempotencyKey())
    orders.value = [result.order, ...orders.value.filter((item) => item.order_no !== result.order.order_no)]
    amount.value = ''
    notice.value = result.order.payment_url
      ? '订单已创建，请打开支付链接继续付款。'
      : '订单已创建，请使用已配置的支付渠道完成付款。'
  } catch (err) {
    error.value = errorMessage(err, '创建充值订单失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">充值</h1>
        <p class="mt-1 text-sm text-slate-500">充值会创建支付渠道订单，为你的 AgentAPI 子余额充值，不收取佣金。</p>
      </div>
      <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">刷新</button>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>

    <section v-if="!loading && !enabled" class="rounded-xl border border-amber-200 bg-amber-50 p-5 text-sm leading-6 text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
      当前 AgentAPI 实例尚未开放充值。请联系平台管理员配置支付渠道回调。浏览器无法自行开启支付或增加余额。
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="font-semibold text-slate-900 dark:text-white">创建充值订单</h2>
          <p class="mt-1 text-sm text-slate-500">支付渠道：{{ providerLabel(provider) }} · 币种：{{ currencyLabel(currency) }}</p>
        </div>
        <p class="text-xs text-slate-500">{{ money(minAmount) }} — {{ money(maxAmount) }}</p>
      </div>
      <form class="mt-4 flex flex-col gap-3 sm:flex-row" novalidate @submit.prevent="createOrder">
        <input ref="amountInput" v-model="amount" class="min-w-0 flex-1 rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" :disabled="!enabled || submitting" inputmode="decimal" min="0.01" placeholder="充值金额，例如 10.00" step="0.01" type="number">
        <button class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!enabled || submitting" type="submit">{{ submitting ? '正在创建…' : '创建订单' }}</button>
      </form>
      <p class="mt-3 text-xs leading-5 text-slate-500">订单仅在收到已签名的支付渠道回调，并成功同步代理站主账户的主站余额后才会分配。回调不会凭空增加主站余额。</p>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700"><h2 class="font-semibold text-slate-900 dark:text-white">我的订单</h2></div>
      <div v-if="loading" class="p-5 text-sm text-slate-500">正在加载…</div>
      <div v-else-if="orders.length === 0" class="p-5 text-sm text-slate-500">暂无充值订单。</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">订单号</th><th class="px-5 py-3">金额</th><th class="px-5 py-3">状态</th><th class="px-5 py-3">创建时间</th><th class="px-5 py-3"></th></tr></thead>
          <tbody>
            <tr v-for="order in orders" :key="order.order_no" class="border-t dark:border-slate-700">
              <td class="px-5 py-4 font-mono text-xs">{{ order.order_no }}</td>
              <td class="px-5 py-4">{{ money(order.amount_cents, order.currency) }}</td>
              <td class="px-5 py-4"><span :class="statusClass(order.status)">{{ statusLabel(order.status) }}</span><div v-if="order.status === 'paid_pending_allocation'" class="text-xs text-slate-500">等待主账户余额同步</div></td>
              <td class="px-5 py-4 text-slate-500">{{ date(order.created_at) }}</td>
              <td class="px-5 py-4 text-right"><a v-if="order.payment_url && order.status === 'pending'" class="text-blue-600 hover:underline" :href="order.payment_url" rel="noreferrer" target="_blank">前往支付</a></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </main>
</template>
