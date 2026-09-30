<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse, type AuditEventView } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentAuditPanel from '@/components/admin/audit/AgentAuditPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const events = ref<AuditEventView[]>([])
const loading = ref(true)
const contextError = ref('')
const auditError = ref('')
let loadSequence = 0

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  contextError.value = ''
  auditError.value = ''

  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    events.value = []
    contextError.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全查看操作日志。')
    loading.value = false
    return
  }

  try {
    const response = await agentAPI.getAuditEvents(500)
    if (sequence !== loadSequence) return
    events.value = response.items || []
  } catch (cause) {
    if (sequence !== loadSequence) return
    events.value = []
    auditError.value = errorMessage(cause, '本站操作日志加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">安全与合规</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">操作日志</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">沿用主站管理日志的筛选、结果标识和详情查看结构，仅记录当前代理站的管理面与安全操作。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading && !context" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站审计边界…</p>
      </div>
    </div>

    <div v-else-if="contextError" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载操作日志</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ contextError }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <section class="grid gap-4 md:grid-cols-3" aria-label="操作日志边界">
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">租户范围</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">仅当前 agent_id</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">其他代理站和主站全局管理员日志不可见。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">日志内容</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">管理操作与结果</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">记录操作者、对象、请求编号、结果和脱敏说明。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">敏感信息</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">凭据永不入前端</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不返回认证头、SuperKey、管理员 Key 或应用凭据。</p>
        </div>
      </section>

      <p v-if="auditError" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ auditError }}</p>
      <AgentAuditPanel :events="events" :loading="loading" :load-failed="Boolean(auditError)" @refresh="load" />

      <section class="card border-dashed p-5">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div>
            <h2 class="font-medium text-gray-900 dark:text-white">与主站审计的边界</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">主站全局请求日志、管理员认证方式、客户端 IP、请求正文和日志清理权限不会下放到代理站。本页只展示 AgentAPI 为当前代理站保存的白名单审计字段，也不允许站长删除或篡改记录。</p>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>
