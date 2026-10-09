<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentBalanceHistoryItem } from '@/agent/api'
import { errorMessage } from '@/agent/locale'

const balance = ref<number | null>(null)
const frozen = ref(0)
const items = ref<AgentBalanceHistoryItem[]>([])
const totalRecharged = ref<number | null>(null)
const loading = ref(true)
const error = ref('')

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
    items.value = history.items || []
    totalRecharged.value = history.total_recharged
  } catch (cause) {
    balance.value = null
    error.value = errorMessage(cause, '主站余额暂不可用，不会显示成 0。')
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
        <p class="mt-1.5 text-sm text-gray-500">读取当前登录用户在主站的余额和变动记录。这里不能修改余额。</p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">刷新</button>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">{{ error }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取主站余额…</section>
    <template v-else-if="balance != null">
      <section class="grid gap-4 sm:grid-cols-3">
        <div class="card p-4"><p class="text-xs text-gray-500">可用余额</p><p class="mt-1 text-2xl font-bold">{{ moneyCents(balance) }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">冻结</p><p class="mt-1 text-2xl font-bold">{{ moneyCents(frozen) }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">累计入账</p><p class="mt-1 text-2xl font-bold">{{ totalRecharged == null ? '待确认' : money(totalRecharged) }}</p></div>
      </section>
      <section class="card overflow-x-auto p-4">
        <h2 class="font-semibold">余额变动</h2>
        <table class="mt-3 w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">类型</th><th>金额</th><th>状态</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="(item, index) in items" :key="index"><td class="py-2">{{ item.type }}</td><td>{{ money(item.value) }}</td><td>{{ item.status }}</td><td>{{ item.created_at || item.used_at || '—' }}</td></tr>
            <tr v-if="!items.length"><td colspan="4" class="py-8 text-center text-gray-500">暂无余额变动记录</td></tr>
          </tbody>
        </table>
      </section>
    </template>
  </main>
</template>
