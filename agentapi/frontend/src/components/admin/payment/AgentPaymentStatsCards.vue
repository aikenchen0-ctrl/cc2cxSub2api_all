<script setup lang="ts">
import type { AgentAdminPaymentDashboard, AgentCurrencyAmounts } from '@/agent/api'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ stats: AgentAdminPaymentDashboard }>()

function sortedAmounts(amounts: AgentCurrencyAmounts) {
  return Object.entries(amounts || {}).sort(([left], [right]) => left.localeCompare(right))
}

function money(currency: string, amount: number) {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(amount) }
  catch { return `${currency} ${Number(amount || 0).toFixed(2)}` }
}
</script>

<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30"><Icon name="dollar" size="md" class="text-emerald-600 dark:text-emerald-400" /></div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">今日收入</p>
          <p v-for="[currency, amount] in sortedAmounts(stats.today_amount)" :key="currency" class="truncate text-xl font-bold text-gray-900 dark:text-white">{{ money(currency, amount) }}</p>
          <p v-if="!Object.keys(stats.today_amount || {}).length" class="text-xl font-bold text-gray-900 dark:text-white">—</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ stats.today_count }} 笔已支付订单</p>
        </div>
      </div>
    </div>

    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30"><Icon name="creditCard" size="md" class="text-blue-600 dark:text-blue-400" /></div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">区间收入</p>
          <p v-for="[currency, amount] in sortedAmounts(stats.total_amount)" :key="currency" class="truncate text-xl font-bold text-gray-900 dark:text-white">{{ money(currency, amount) }}</p>
          <p v-if="!Object.keys(stats.total_amount || {}).length" class="text-xl font-bold text-gray-900 dark:text-white">—</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ stats.total_count }} 笔已支付订单</p>
        </div>
      </div>
    </div>

    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30"><Icon name="chart" size="md" class="text-purple-600 dark:text-purple-400" /></div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">待支付订单</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.pending_orders }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">当前本站用户</p>
        </div>
      </div>
    </div>

    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30"><Icon name="chartBar" size="md" class="text-amber-600 dark:text-amber-400" /></div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">平均支付金额</p>
          <p v-for="[currency, amount] in sortedAmounts(stats.avg_amount)" :key="currency" class="truncate text-xl font-bold text-gray-900 dark:text-white">{{ money(currency, amount) }}</p>
          <p v-if="!Object.keys(stats.avg_amount || {}).length" class="text-xl font-bold text-gray-900 dark:text-white">—</p>
        </div>
      </div>
    </div>
  </div>
</template>
