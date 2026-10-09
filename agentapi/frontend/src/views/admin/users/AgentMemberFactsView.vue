<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { agentAPI, type AgentMemberFacts } from '@/agent/api'
import { errorMessage } from '@/agent/locale'

const route = useRoute()
const facts = ref<AgentMemberFacts | null>(null)
const loading = ref(true)
const error = ref('')

function moneyCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`
}

async function load(): Promise<void> {
  const mainUserID = String(route.query.main_user_id || '')
  loading.value = true
  error.value = ''
  facts.value = null
  if (!mainUserID) {
    error.value = '缺少本站用户 ID。'
    loading.value = false
    return
  }
  try {
    facts.value = await agentAPI.adminUsers.facts(mainUserID)
  } catch (cause) {
    error.value = errorMessage(cause, '该用户不属于当前站点，或主站明细暂不可用。')
  } finally {
    loading.value = false
  }
}

watch(() => route.query.main_user_id, () => void load())
onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-5xl space-y-4 p-6">
    <header>
      <p class="text-sm font-medium text-primary-600">本站管理</p>
      <h1 class="mt-1 text-2xl font-bold">成员余额与用量</h1>
      <p class="mt-2 text-sm text-gray-500">只读取已归属当前站点的用户。主站账号、分组和上游信息不会出现在这里。</p>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取主站记录…</section>
    <template v-else-if="facts">
      <section class="card p-4">
        <p class="text-xs text-gray-500">用户 {{ facts.main_user_id }}</p>
        <p class="mt-1 text-2xl font-bold">{{ facts.balance_available ? moneyCents(facts.balance_cents) : '暂不可用' }}</p>
      </section>
      <section class="card overflow-x-auto p-4">
        <h2 class="font-semibold">主站用量</h2>
        <p v-if="!facts.usage_available" class="mt-3 text-sm text-amber-700">用量暂未同步，不会显示成 0。</p>
        <table v-else class="mt-3 w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">模型</th><th>输入</th><th>输出</th><th>实际费用</th></tr></thead>
          <tbody>
            <tr v-for="item in facts.usage?.items || []" :key="item.request_id"><td class="py-2">{{ item.model }}</td><td>{{ item.input_tokens }}</td><td>{{ item.output_tokens }}</td><td>${{ item.actual_cost.toFixed(4) }}</td></tr>
            <tr v-if="!(facts.usage?.items || []).length"><td colspan="4" class="py-8 text-center text-gray-500">暂无主站用量</td></tr>
          </tbody>
        </table>
      </section>
    </template>
  </main>
</template>
