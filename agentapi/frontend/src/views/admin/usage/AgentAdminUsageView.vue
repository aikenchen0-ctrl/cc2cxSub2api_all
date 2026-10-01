<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse, type SettlementView } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentSettlementsPanel from '@/components/admin/usage/AgentSettlementsPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const settlements = ref<SettlementView[]>([])
const loading = ref(true)
const contextError = ref('')
const settlementsError = ref('')
const notice = ref('')
const reconciling = ref(false)
let loadSequence = 0

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  contextError.value = ''
  settlementsError.value = ''
  notice.value = ''

  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    settlements.value = []
    contextError.value = errorMessage(cause, '站点信息加载失败，暂时不能安全查看本站用量记录。')
    loading.value = false
    return
  }

  try {
    const response = await agentAPI.getSettlements()
    if (sequence !== loadSequence) return
    settlements.value = response.items
  } catch (cause) {
    if (sequence !== loadSequence) return
    settlements.value = []
    settlementsError.value = errorMessage(cause, '本站用量同步记录加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function reconcile(): Promise<void> {
  reconciling.value = true
  notice.value = ''
  settlementsError.value = ''
  try {
    const response = await agentAPI.reconcileSettlements()
    settlements.value = response.items
    notice.value = response.total > 0
      ? `已核对 ${response.total} 条待确认记录。`
      : '当前没有需要核对的待确认记录。'
  } catch (cause) {
    settlementsError.value = errorMessage(cause, '核对本站用量同步记录失败，请稍后重试。')
  } finally {
    reconciling.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">账本核对</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">用量同步</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">查看本站请求、用量和结算记录的同步状态。</p>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
        <button v-if="context" type="button" class="btn btn-secondary" :disabled="loading || reconciling" @click="load">
          <Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新
        </button>
      </div>
    </header>

    <div v-if="loading && !context" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认站点用量边界…</p>
      </div>
    </div>

    <div v-else-if="contextError" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载用量同步</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ contextError }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <div class="grid gap-4 md:grid-cols-3">
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">同步记录</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ settlements.length }}</p>
          <p class="mt-1 text-xs text-gray-400">仅本站请求</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">待确认</p>
          <p class="mt-2 text-2xl font-semibold text-amber-600 dark:text-amber-400">{{ settlements.filter(item => item.status === 'pending').length }}</p>
          <p class="mt-1 text-xs text-gray-400">不会以本站估算替代最终费用</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">计费边界</p>
          <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">用户余额计费</p>
          <p class="mt-1 text-xs text-gray-400">只读展示，不修改计费记录</p>
        </div>
      </div>

      <p v-if="settlementsError" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ settlementsError }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/40 dark:text-blue-300">{{ notice }}</p>

      <AgentSettlementsPanel
        :items="settlements"
        :loading="loading"
        :load-failed="Boolean(settlementsError)"
        :reconciling="reconciling"
        @reconcile="reconcile"
      />

      <section class="card border-dashed p-5">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div>
            <h2 class="font-medium text-gray-900 dark:text-white">数据与权限边界</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">本页只核对本站已记录的请求与用量。站点管理员不能查看其他站点数据，也不能调整平台级费用、余额或计费配置。</p>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>
