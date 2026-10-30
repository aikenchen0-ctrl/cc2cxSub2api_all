<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentBalanceHistoryItem } from '@/agent/api'
import { errorMessage } from '@/agent/locale'

const balance = ref<number | null>(null)
const frozen = ref(0)
const stationBalance = ref<number | null>(null)
const mainBalance = ref<number | null>(null)
const source = ref('')
const items = ref<AgentBalanceHistoryItem[]>([])
const totalRecharged = ref<number | null>(null)
const loading = ref(true)
const error = ref('')

const usesStationWallet = computed(() => source.value === 'station_wallet')

function moneyCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`
}
function money(value: number): string {
  return `$${value.toFixed(2)}`
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [current, history] = await Promise.all([
      agentAPI.balance.get(),
      agentAPI.balance.history(),
    ])
    balance.value = current.balance_cents
    frozen.value = current.frozen_balance_cents
    source.value = current.source || ''
    stationBalance.value = current.station_balance_cents ?? null
    mainBalance.value = current.main_balance_cents ?? null
    items.value = history.items || []
    totalRecharged.value = history.total_recharged ?? null
  } catch (cause) {
    balance.value = null
    error.value = errorMessage(cause, '余额暂不可用，不会显示成 0。')
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-4xl space-y-4">
    <header class="flex items-end justify-between gap-3">
      <div>
        <p class="text-sm font-medium text-primary-600">BALANCE</p>
        <h1 class="mt-1 text-2xl font-bold">余额</h1>
        <p class="mt-1.5 text-sm text-gray-500">
          站内余额用于本站模型调用扣费，充值成功后即时到账。主站余额仅作参考，这里不能修改任何余额。
        </p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">刷新</button>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">{{ error }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取余额…</section>
    <template v-else-if="balance != null">
      <section class="grid gap-4 sm:grid-cols-3">
        <div class="card p-4">
          <p class="text-xs text-gray-500">可用余额<span v-if="usesStationWallet">（站内）</span></p>
          <p class="mt-1 text-2xl font-bold" data-testid="station-balance">{{ moneyCents(balance) }}</p>
          <p v-if="frozen > 0" class="mt-1 text-xs text-amber-600">冻结 {{ moneyCents(frozen) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500">主站余额（参考）</p>
          <p class="mt-1 text-2xl font-bold text-gray-600 dark:text-gray-300" data-testid="main-balance">
            {{ mainBalance == null ? '—' : moneyCents(mainBalance) }}
          </p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500">累计充值</p>
          <p class="mt-1 text-2xl font-bold">{{ totalRecharged == null ? '待确认' : money(totalRecharged) }}</p>
        </div>
      </section>
      <section class="card overflow-x-auto p-4">
        <h2 class="font-semibold">余额变动</h2>
        <table class="mt-3 w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">类型</th><th>金额</th><th>余额</th><th>状态</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="(item, index) in items" :key="index">
              <td class="py-2">{{ item.type }}<span v-if="item.note" class="ml-1 text-xs text-gray-400">{{ item.note }}</span></td>
              <td>{{ money(item.value) }}</td>
              <td>{{ item.balance_after == null ? '—' : money(item.balance_after) }}</td>
              <td>{{ item.status }}</td>
              <td>{{ item.created_at || item.used_at || '—' }}</td>
            </tr>
            <tr v-if="!items.length"><td colspan="5" class="py-8 text-center text-gray-500">暂无余额变动记录</td></tr>
          </tbody>
        </table>
      </section>
    </template>
    <p v-if="stationBalance != null && !usesStationWallet" class="text-xs text-gray-400">
      站内余额：{{ moneyCents(stationBalance) }}
    </p>
  </main>
</template>
