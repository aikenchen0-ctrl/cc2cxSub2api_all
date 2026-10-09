<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentRedeemHistoryItem } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import { notifyAgentAccountFactsChanged } from '@/agent/accountFacts'

const code = ref('')
const items = ref<AgentRedeemHistoryItem[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = (await agentAPI.redeem.history()).items || []
  } catch (cause) {
    items.value = []
    error.value = errorMessage(cause, '兑换记录暂不可用，不会显示成没有兑换过。')
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  if (saving.value || code.value.trim().length < 4) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await agentAPI.redeem.submit(code.value.trim())
    notice.value = `兑换成功，类型 ${result.type}，面值 ${result.value}。`
    code.value = ''
    notifyAgentAccountFactsChanged({ refreshSubscriptions: false })
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '兑换失败。此入口只能兑换到当前账户，不能发行或查看兑换码。')
  } finally {
    saving.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-3xl space-y-4">
    <header>
      <p class="text-sm font-medium text-primary-600 dark:text-primary-300">REDEEM</p>
      <h1 class="mt-1 text-2xl font-bold">兑换</h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">兑换码只作用于当前登录用户。这里不能创建、导出或管理兑换码。</p>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ notice }}</p>
    <section class="card flex flex-col gap-3 p-4 sm:flex-row">
      <input v-model="code" class="input" maxlength="64" placeholder="输入兑换码" aria-label="兑换码">
      <button class="btn btn-primary" type="button" :disabled="saving || code.trim().length < 4" @click="submit">兑换到当前账户</button>
    </section>
    <section class="card p-4">
      <h2 class="font-semibold">我的兑换记录</h2>
      <p v-if="loading" class="mt-4 text-sm text-gray-500">正在读取记录…</p>
      <ul v-else class="mt-3 divide-y divide-gray-100 text-sm dark:divide-dark-700">
        <li v-for="(item, index) in items" :key="index" class="flex justify-between py-3"><span>{{ item.type }} · {{ item.value }}</span><span class="text-gray-500">{{ item.status }}</span></li>
        <li v-if="!items.length" class="py-8 text-center text-gray-500">暂无兑换记录</li>
      </ul>
    </section>
  </main>
</template>
