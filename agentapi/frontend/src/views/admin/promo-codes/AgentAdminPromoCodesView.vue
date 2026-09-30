<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAdminPromoCodes, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const promoCodes = ref<AgentAdminPromoCodes | null>(null)
const loading = ref(true)
const loadError = ref('')
const search = ref('')
const status = ref('')
let loadSequence = 0
const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '有效' },
  { value: 'disabled', label: '已禁用' },
]

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return (promoCodes.value?.items || []).filter((item) => {
    if (status.value && item.status !== status.value) return false
    return !query || item.code.toLowerCase().includes(query)
  })
})

function formatDate(value?: string | null): string {
  if (!value) return '永不过期'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  loadError.value = ''
  context.value = null
  promoCodes.value = null
  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
    const nextPromoCodes = await agentAPI.adminPromoCodes.get()
    if (sequence !== loadSequence) return
    promoCodes.value = nextPromoCodes
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    promoCodes.value = null
    loadError.value = errorMessage(cause, '优惠码管理加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-5 p-6 pb-12" aria-label="优惠码管理">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">用户与奖励</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">优惠码管理</h1>
        <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">沿用主站注册优惠码管理页的搜索、状态、奖励、使用次数和有效期结构。优惠码奖励会进入 Sub2API 主站余额，因此发行前必须先建立代理站出资与主站原子入账协议。</p>
      </div>
      <button v-if="context" type="button" class="btn btn-secondary w-fit" :disabled="loading" @click="load"><Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新</button>
    </header>

    <section v-if="loading" class="card flex min-h-52 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500 dark:text-gray-400"><Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" /><p class="mt-3">正在确认当前代理站优惠码资金边界…</p></div>
    </section>

    <section v-else-if="loadError || !context || !promoCodes" class="card border-red-200 p-6 dark:border-red-900/50" role="alert">
      <p class="font-semibold text-red-700 dark:text-red-300">无法加载优惠码管理</p>
      <p class="mt-1 text-sm text-red-600 dark:text-red-400">{{ loadError || '当前代理站上下文不可用。' }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load">重试</button>
    </section>

    <template v-else>
      <section class="grid gap-4 md:grid-cols-3" aria-label="优惠码管理边界">
        <article class="card p-5"><p class="text-xs font-semibold uppercase tracking-wide text-gray-400">余额权威</p><p class="mt-2 font-medium text-gray-900 dark:text-white">Sub2API 主站</p><p class="mt-1 text-xs text-gray-500">奖励不能在 AgentAPI 本地伪记。</p></article>
        <article class="card p-5"><p class="text-xs font-semibold uppercase tracking-wide text-gray-400">资金模式</p><p class="mt-2 font-medium text-amber-600">尚未配置</p><p class="mt-1 text-xs text-gray-500">等待确认站长出资与核销规则。</p></article>
        <article class="card p-5"><p class="text-xs font-semibold uppercase tracking-wide text-gray-400">当前能力</p><p class="mt-2 font-medium text-gray-900 dark:text-white">安全只读边界</p><p class="mt-1 text-xs text-gray-500">不会调用主站全局优惠码接口。</p></article>
      </section>

      <section class="card overflow-hidden">
        <div class="flex flex-col gap-3 border-b border-gray-100 p-4 dark:border-dark-700 sm:flex-row sm:items-center">
          <AgentSearchInput v-model="search" class="min-w-0 flex-1" placeholder="搜索优惠码…" aria-label="搜索优惠码" />
          <AgentSelect v-model="status" :options="statusOptions" class="sm:w-40" aria-label="优惠码状态" />
          <button type="button" class="btn btn-primary" disabled title="需要先配置主站资金协议"><Icon name="plus" size="sm" class="mr-2" />创建优惠码</button>
        </div>

        <div v-if="filteredItems.length" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-100 text-left text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800/70"><tr><th class="px-5 py-3">优惠码</th><th class="px-5 py-3">赠送余额</th><th class="px-5 py-3">使用次数</th><th class="px-5 py-3">状态</th><th class="px-5 py-3">过期时间</th><th class="px-5 py-3">创建时间</th><th class="px-5 py-3">操作</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="item in filteredItems" :key="item.code"><td class="px-5 py-4 font-mono font-medium">{{ item.code }}</td><td class="px-5 py-4">${{ item.bonus_amount.toFixed(2) }}</td><td class="px-5 py-4">{{ item.used_count }} / {{ item.max_uses === 0 ? '∞' : item.max_uses }}</td><td class="px-5 py-4"><AgentStatusBadge :status="item.status" :label="item.status === 'active' ? '有效' : '已禁用'" /></td><td class="px-5 py-4">{{ formatDate(item.expires_at) }}</td><td class="px-5 py-4">{{ formatDate(item.created_at) }}</td><td class="px-5 py-4 text-gray-400">资金协议启用后开放</td></tr></tbody>
          </table>
        </div>
        <div v-else class="p-10 text-center">
          <Icon name="badge" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
          <h2 class="mt-4 font-semibold text-gray-900 dark:text-white">优惠码资金模式尚未配置</h2>
          <p class="mx-auto mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">当前不会创建无资金来源的优惠码，也不会使用本地余额冒充主站入账。确认出资规则后，可在保持本页面结构的前提下接通创建、编辑、删除、使用记录和注册核销。</p>
        </div>
      </section>

      <section class="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-800 dark:border-sky-900/40 dark:bg-sky-900/10 dark:text-sky-200">
        <Icon name="shield" size="sm" class="mt-0.5 shrink-0" />
        <div><p class="font-medium">资金与数据边界</p><p class="mt-1">代理站站长不能调用主站全局优惠码管理接口，也不能使用管理员 Key 或 SuperKey直接增加用户余额。浏览器不会收到主站凭据、站长余额或内部账本字段。</p></div>
      </section>
    </template>
  </main>
</template>
