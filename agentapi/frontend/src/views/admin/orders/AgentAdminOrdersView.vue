<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentAdminOrdersPanel from '@/components/admin/orders/AgentAdminOrdersPanel.vue'
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
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全管理订单。')
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
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">订单管理</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">查看当前代理站映射用户在 Sub2API 主站创建的订单；支付、到账和退款事实仍由主站维护。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站订单边界…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载订单管理</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <AgentAdminOrdersPanel v-else-if="context" />
  </main>
</template>
