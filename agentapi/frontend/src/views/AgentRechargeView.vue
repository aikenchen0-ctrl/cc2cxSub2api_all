<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI } from '@/agent/api'
import { errorMessage } from '@/agent/client'

const rechargeURL = ref('')
const loading = ref(true)
const error = ref('')

function safeRechargeURL(value?: string): string {
  if (!value) return ''
  try {
    const parsed = new URL(value, window.location.origin)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : ''
  } catch {
    return ''
  }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const settings = await agentAPI.getPublicSettings()
    rechargeURL.value = safeRechargeURL(settings.recharge_url)
    if (!rechargeURL.value) error.value = '主站充值地址暂不可用，请联系代理站管理员。'
  } catch (err) {
    error.value = errorMessage(err, '加载主站充值入口失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-4xl space-y-6 p-6">
    <header>
      <p class="text-sm text-slate-500">AgentAPI</p>
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">充值 Sub2API 余额</h1>
      <p class="mt-1 text-sm text-slate-500">你的模型调用由自己的 Sub2API 主站账户直接计费，AgentAPI 不维护或分配本地子余额。</p>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>

    <section class="rounded-xl border bg-white p-6 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold text-slate-900 dark:text-white">统一主站充值</h2>
      <p class="mt-2 text-sm leading-6 text-slate-500">充值、余额变动和支付订单均由 Sub2API 主站处理。充值完成后返回本代理站并刷新页面，即可看到最新真实余额。</p>
      <div class="mt-5 flex flex-wrap gap-3">
        <a
          v-if="rechargeURL"
          class="rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-blue-700"
          :href="rechargeURL"
          rel="noopener noreferrer"
          target="_blank"
        >前往 Sub2API 主站充值</a>
        <button class="rounded-lg border px-5 py-2.5 text-sm disabled:opacity-50" type="button" :disabled="loading" @click="load">{{ loading ? '正在加载…' : '重新加载入口' }}</button>
      </div>
      <p class="mt-4 text-xs leading-5 text-slate-500">AgentAPI 不接收任何主站管理凭据或应用凭据，也不会在浏览器中生成主站计费身份。</p>
    </section>
  </main>
</template>
