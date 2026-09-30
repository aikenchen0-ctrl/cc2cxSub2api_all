<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentPaymentDashboardPanel from '@/components/admin/payment/AgentPaymentDashboardPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const loading = ref(true)
const error = ref('')

async function loadContext(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    context.value = await agentAPI.getContext()
  } catch (cause) {
    context.value = null
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全查看支付统计。')
  } finally {
    loading.value = false
  }
}

onMounted(() => void loadContext())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">支付与订单</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">支付统计</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">按主站管理台的收入趋势、支付方式和用户排行结构，汇总当前代理站映射用户在 Sub2API 主站产生的权威订单事实。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站支付统计边界…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载支付统计</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <section class="grid gap-4 md:grid-cols-3" aria-label="支付统计边界">
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">统计范围</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">当前代理站映射用户</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不会读取其他代理站或主站全局订单。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">权威来源</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">Sub2API 主站订单</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">金额、状态、币种和支付方式均以主站为准。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">管理边界</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">只读经营统计</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不提供全局退款、支付密钥或供应商配置。</p>
        </div>
      </section>
      <AgentPaymentDashboardPanel />
    </template>
  </main>
</template>
