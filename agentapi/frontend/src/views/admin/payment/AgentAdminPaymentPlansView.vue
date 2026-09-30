<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentPaymentPlansPanel from '@/components/admin/payment/AgentPaymentPlansPanel.vue'
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
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全管理套餐展示。')
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
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">套餐管理</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">沿用主站套餐表格与编辑体验，仅管理当前代理站购买页的展示、排序和文案；套餐财务事实仍由 Sub2API 主站维护。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站套餐管理边界…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载套餐管理</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <section class="grid gap-4 md:grid-cols-3" aria-label="套餐管理边界">
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">主站维护</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">价格与订阅事实</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">币种、价格、分组、有效期和可售来源不可在本站篡改。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">本站维护</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">展示与销售入口</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">仅保存当前代理站的显示开关、排序和介绍文案。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">隔离保证</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">按 agent_id 保存</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不会修改主站或其他代理站的套餐配置。</p>
        </div>
      </section>
      <AgentPaymentPlansPanel />
    </template>
  </main>
</template>
