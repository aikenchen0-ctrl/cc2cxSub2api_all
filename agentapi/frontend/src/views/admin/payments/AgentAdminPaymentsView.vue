<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentPaymentStats } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import Icon from '@/components/icons/Icon.vue'

const stats = ref<AgentPaymentStats | null>(null)
const loading = ref(true)
const error = ref('')

function money(value: number, currency = 'USD'): string {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(value) }
  catch { return `${value.toFixed(2)} ${currency}` }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    stats.value = await agentAPI.paymentStats()
  } catch (cause) {
    stats.value = null
    error.value = errorMessage(cause, '支付统计加载失败。未取得主站订单时不会显示为零。')
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-5xl space-y-6 p-6 pb-12">
    <header class="flex items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600">本站管理</p>
        <h1 class="mt-1 text-2xl font-bold">支付统计</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500">汇总当前站点成员在主站的订单抽样。不能修改余额，也不能代其他用户取消或退款。</p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取主站订单…</section>
    <template v-else-if="stats">
      <p v-if="stats.partial || stats.lookup_failures" role="status" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
        当前是抽样结果：已查看 {{ stats.members_sampled }} / {{ stats.members_total }} 名成员，{{ stats.lookup_failures }} 次查询失败。未抽样到的金额不会补成 0。
      </p>
      <section class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div class="card p-4"><p class="text-xs text-gray-500">已完成订单</p><p class="mt-1 text-2xl font-bold">{{ stats.completed_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">待支付</p><p class="mt-1 text-2xl font-bold">{{ stats.pending_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">退款申请</p><p class="mt-1 text-2xl font-bold">{{ stats.refund_requested_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">抽样实付</p><p class="mt-1 text-2xl font-bold">{{ stats.amount_available && stats.completed_pay_amount != null ? money(stats.completed_pay_amount, stats.currency || 'USD') : '待确认' }}</p></div>
      </section>
    </template>
  </main>
</template>
