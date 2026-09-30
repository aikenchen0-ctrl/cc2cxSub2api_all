<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentUsersPanel from '@/components/admin/user/AgentUsersPanel.vue'
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
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全管理用户。')
  } finally {
    loading.value = false
  }
}

onMounted(loadContext)
</script>

<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">用户与权限</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">用户管理</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">管理当前代理站记录的用户归属、本站访问状态和 AgentAPI Key；用户身份与实时余额仍以 Sub2API 主站为准。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading" class="rounded-xl border border-gray-200 bg-white p-12 text-center shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
      <p class="mt-3 text-sm text-gray-500">正在确认代理站管理边界…</p>
    </div>

    <div v-else-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-6 dark:border-red-900/50 dark:bg-red-950/30">
      <p class="font-medium text-red-800 dark:text-red-200">无法加载用户管理</p>
      <p class="mt-2 text-sm text-red-700 dark:text-red-300">{{ error }}</p>
      <button class="btn btn-secondary mt-4" type="button" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <AgentUsersPanel v-else-if="context" :owner-main-user-id="context.agent.owner_main_user_id || ''" />
  </div>
</template>
