<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { agentAPI, type AgentContextResponse, type AgentModelPolicy } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentChannelsPanel from '@/components/admin/channels/AgentChannelsPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  mode: 'channels' | 'pricing'
}>()

const context = ref<AgentContextResponse | null>(null)
const policy = ref<AgentModelPolicy | null>(null)
const enabled = ref<string[]>([])
const loading = ref(true)
const saving = ref(false)
const contextError = ref('')
const policyError = ref('')
const notice = ref('')
let loadSequence = 0

const isPricing = computed(() => props.mode === 'pricing')
const title = computed(() => isPricing.value ? '模型定价' : '渠道管理')

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  contextError.value = ''
  policyError.value = ''
  notice.value = ''

  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    policy.value = null
    enabled.value = []
    contextError.value = errorMessage(cause, `代理站信息加载失败，暂时不能安全查看${title.value}。`)
    loading.value = false
    return
  }

  try {
    const nextPolicy = await agentAPI.getAdminModelPolicy()
    if (sequence !== loadSequence) return
    policy.value = nextPolicy
    enabled.value = [...nextPolicy.enabled]
  } catch (cause) {
    if (sequence !== loadSequence) return
    policy.value = null
    enabled.value = []
    policyError.value = errorMessage(cause, '本站模型策略加载失败；渠道资料仍可只读查看，但模型开关暂不可保存。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function save(): Promise<void> {
  if (isPricing.value || !policy.value) return
  saving.value = true
  notice.value = ''
  policyError.value = ''
  try {
    const updated = await agentAPI.updateAdminModelPolicy(enabled.value)
    policy.value = updated
    enabled.value = [...updated.enabled]
    notice.value = `本站模型策略已保存，当前启用 ${updated.enabled.length} 个模型。`
  } catch (cause) {
    policyError.value = errorMessage(cause, '更新本站模型策略失败，主站未确认前不会应用本次修改。')
  } finally {
    saving.value = false
  }
}

watch(() => props.mode, () => void load())
onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">模型与渠道</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{{ title }}</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">
          {{ isPricing ? '按主站模型定价表查看当前代理站用户可用渠道的权威计费信息；本站不维护或覆盖主站价格。' : '沿用主站渠道目录的搜索、平台和状态结构，配置当前代理站允许向用户公开的模型。' }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
        <button v-if="context" type="button" class="btn btn-secondary" :disabled="loading || saving" @click="load">
          <Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新
        </button>
      </div>
    </header>

    <div v-if="loading && !context" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站{{ title }}边界…</p>
      </div>
    </div>

    <div v-else-if="contextError" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载{{ title }}</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ contextError }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <section class="grid gap-4 md:grid-cols-3" :aria-label="`${title}边界`">
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">资料来源</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">Sub2API 主站目录</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">渠道、平台、分组和价格均由主站提供。</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">本站能力</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">{{ isPricing ? '只读价格事实' : '模型允许列表' }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ isPricing ? '不提供本地价格覆盖或倍率编辑。' : '只控制当前代理站公开和可调用的模型。' }}</p>
        </div>
        <div class="card p-5">
          <p class="text-xs font-semibold uppercase tracking-wide text-gray-400">禁止操作</p>
          <p class="mt-2 font-medium text-gray-900 dark:text-white">主站全局配置</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不能创建渠道、管理账号池或读取渠道凭据。</p>
        </div>
      </section>

      <p v-if="policyError" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ policyError }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-emerald-50 p-4 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ notice }}</p>

      <AgentChannelsPanel
        v-model:enabled="enabled"
        :mode="mode"
        :policy="policy"
        :saving="saving"
        :load-failed="Boolean(policyError)"
        @save="save"
      />

      <section class="card border-dashed p-5">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div>
            <h2 class="font-medium text-gray-900 dark:text-white">主站与代理站职责</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">Sub2API 继续维护真实渠道、账号池、模型价格、分组倍率和最终计费。AgentAPI 只读取当前用户可见的白名单资料；渠道页保存本站模型范围时也必须获得主站运行时确认，确认失败会保持原策略。</p>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>
