<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentTOTPStatus } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentTotpSetupModal from './AgentTotpSetupModal.vue'
import AgentTotpDisableDialog from './AgentTotpDisableDialog.vue'

const loading = ref(true)
const status = ref<AgentTOTPStatus | null>(null)
const error = ref('')
const notice = ref('')
const showSetup = ref(false)
const showDisable = ref(false)

async function loadStatus(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    status.value = await agentAPI.profile.totp.getStatus()
  } catch (err) {
    error.value = errorMessage(err, '加载两步验证状态失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function setupComplete(): Promise<void> {
  showSetup.value = false
  notice.value = '两步验证已启用。'
  await loadStatus()
}

async function disableComplete(): Promise<void> {
  showDisable.value = false
  notice.value = '两步验证已停用。'
  await loadStatus()
}

function formatDate(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleString('zh-CN', {
    year: 'numeric', month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

onMounted(loadStatus)
</script>

<template>
  <section data-testid="profile-totp-card" class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">两步验证</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">使用身份验证器动态验证码保护 Sub2API 主站账号。</p>
    </div>
    <div class="px-6 py-6">
      <p v-if="error" role="alert" class="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>
      <div v-if="loading" class="flex items-center justify-center py-8" role="status">
        <span class="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-primary-500"></span>
        <span class="sr-only">正在加载两步验证状态</span>
      </div>
      <div v-else-if="!status" class="flex items-center justify-between gap-4">
        <p class="text-sm text-gray-500 dark:text-gray-400">暂时无法读取两步验证状态。</p>
        <button type="button" class="btn btn-secondary" @click="loadStatus">重新加载</button>
      </div>
      <div v-else-if="!status.feature_enabled" class="flex items-center gap-4 py-2">
        <div class="rounded-full bg-gray-100 p-3 text-gray-400 dark:bg-dark-700" aria-hidden="true">
          <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 00-9 0v3.75m-.75 0h10.5A2.25 2.25 0 0119.5 12.75v6A2.25 2.25 0 0117.25 21H6.75a2.25 2.25 0 01-2.25-2.25v-6a2.25 2.25 0 012.25-2.25z" /></svg>
        </div>
        <div>
          <p class="font-medium text-gray-700 dark:text-gray-300">主站暂未开放两步验证</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">功能开放后可在这里直接配置。</p>
        </div>
      </div>
      <div v-else-if="status.enabled" class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-4">
          <div class="rounded-full bg-emerald-100 p-3 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400" aria-hidden="true">
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" /></svg>
          </div>
          <div>
            <p class="font-medium text-gray-900 dark:text-white">两步验证已启用</p>
            <p v-if="status.enabled_at" class="text-sm text-gray-500 dark:text-gray-400">启用时间：{{ formatDate(status.enabled_at) }}</p>
          </div>
        </div>
        <button type="button" class="btn btn-danger" data-testid="totp-disable" @click="showDisable = true">停用</button>
      </div>
      <div v-else class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-4">
          <div class="rounded-full bg-gray-100 p-3 text-gray-400 dark:bg-dark-700" aria-hidden="true">
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" /></svg>
          </div>
          <div>
            <p class="font-medium text-gray-700 dark:text-gray-300">尚未启用两步验证</p>
            <p class="text-sm text-gray-500 dark:text-gray-400">推荐使用 Google Authenticator、Microsoft Authenticator 等兼容应用。</p>
          </div>
        </div>
        <button type="button" class="btn btn-primary" data-testid="totp-enable" @click="showSetup = true">启用</button>
      </div>
    </div>
    <AgentTotpSetupModal v-if="showSetup" @close="showSetup = false" @success="setupComplete" />
    <AgentTotpDisableDialog v-if="showDisable" @close="showDisable = false" @success="disableComplete" />
  </section>
</template>
