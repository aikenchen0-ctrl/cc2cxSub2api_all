<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAdminProvisioning, type AgentContextResponse } from '@/agent/api'
import { enumLabel, errorMessage, statusLabel } from '@/agent/locale'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const provisioning = ref<AgentAdminProvisioning | null>(null)
const loading = ref(true)
const loadError = ref('')
let loadSequence = 0

const lifecycleSteps = ['pending', 'provisioning', 'active', 'suspended', 'revoked'] as const
const currentStep = computed(() => {
  const status = provisioning.value?.runtime_status || provisioning.value?.configured_status || 'pending'
  const index = lifecycleSteps.indexOf(status as typeof lifecycleSteps[number])
  return index >= 0 ? index : 0
})

const statusTone = computed(() => {
  if (!provisioning.value?.control_available) return 'warning'
  if (provisioning.value.ready) return 'success'
  return provisioning.value.runtime_status || 'inactive'
})

function formatTime(value?: string): string {
  if (!value) return '未由主站控制面检查'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

function billingModeLabel(value: string): string {
  return enumLabel(value)
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  loadError.value = ''
  context.value = null
  provisioning.value = null
  try {
    const nextContext = await agentAPI.getContext()
    if (sequence !== loadSequence) return
    context.value = nextContext
    const nextProvisioning = await agentAPI.adminProvisioning.get()
    if (sequence !== loadSequence) return
    provisioning.value = nextProvisioning
  } catch (cause) {
    if (sequence !== loadSequence) return
    context.value = null
    provisioning.value = null
    loadError.value = errorMessage(cause, '代理站开通状态加载失败，请稍后重试。')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-5 p-6" aria-label="代理站开通">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">站点管理 · 当前实例</p>
        <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">代理站开通</h1>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">沿用主站代理站开通页的信息层级，但只展示当前代理站的生命周期事实。创建、激活、暂停和撤销仍由 Sub2API 主站控制面执行。</p>
      </div>
      <button type="button" class="btn btn-secondary w-fit" :disabled="loading" @click="load">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新
      </button>
    </header>

    <section v-if="loading" class="card flex min-h-52 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认当前代理站和主站生命周期状态…</p>
      </div>
    </section>

    <section v-else-if="loadError || !context || !provisioning" class="card border-red-200 p-6 dark:border-red-900/50" role="alert">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 class="font-semibold text-red-700 dark:text-red-300">无法加载代理站开通状态</h2>
          <p class="mt-1 text-sm text-red-600 dark:text-red-400">{{ loadError || '当前代理站上下文不可用。' }}</p>
        </div>
        <button type="button" class="btn btn-secondary w-fit" @click="load">重试</button>
      </div>
    </section>

    <template v-else>
      <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-label="代理站开通概览">
        <article class="card p-5">
          <p class="text-sm text-gray-500">运行状态</p>
          <div class="mt-2 flex items-center gap-2"><AgentStatusBadge :status="statusTone" :label="statusLabel(provisioning.runtime_status)" /><span class="text-xs text-gray-400">{{ provisioning.ready ? '可提供服务' : '尚未就绪' }}</span></div>
          <p class="mt-2 text-xs text-gray-400">配置状态：{{ statusLabel(provisioning.configured_status) }}</p>
        </article>
        <article class="card p-5">
          <p class="text-sm text-gray-500">控制面状态</p>
          <p class="mt-2 text-lg font-semibold" :class="provisioning.control_available ? 'text-green-600' : 'text-amber-600'">{{ provisioning.control_enabled ? (provisioning.control_available ? '主站状态有效' : '主站状态已过期') : '独立运行模式' }}</p>
          <p class="mt-2 text-xs text-gray-400">检查时间：{{ formatTime(provisioning.last_checked_at) }}</p>
        </article>
        <article class="card p-5">
          <p class="text-sm text-gray-500">当前域名</p>
          <p class="mt-2 break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ provisioning.domain || context.agent.domain || '未配置' }}</p>
          <p class="mt-2 text-xs text-gray-400">只显示当前 agent_id 的域名</p>
        </article>
        <article class="card p-5">
          <p class="text-sm text-gray-500">计费模式</p>
          <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">{{ billingModeLabel(provisioning.billing_mode) }}</p>
          <p class="mt-2 text-xs text-gray-400">最终余额与扣费仍由主站处理</p>
        </article>
      </section>

      <section class="card overflow-hidden" aria-labelledby="current-agent-title">
        <div class="border-b border-gray-100 px-5 py-5 dark:border-dark-700 sm:px-6">
          <div class="flex flex-wrap items-center gap-2">
            <h2 id="current-agent-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ provisioning.site_name || provisioning.display_name }}</h2>
            <span class="badge badge-gray">当前代理站</span>
            <AgentStatusBadge :status="statusTone" :label="statusLabel(provisioning.runtime_status)" />
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">浏览器只会收到这一个实例的白名单状态字段，不会枚举其他站长或主站全局配置。</p>
        </div>

        <dl class="grid gap-x-8 gap-y-5 p-5 text-sm sm:grid-cols-2 sm:p-6 lg:grid-cols-3">
          <div><dt class="text-gray-500">Agent ID</dt><dd class="mt-1 break-all font-mono font-medium">{{ provisioning.agent_id }}</dd></div>
          <div><dt class="text-gray-500">卫星标识</dt><dd class="mt-1 font-mono font-medium">{{ provisioning.satellite_slug }}</dd></div>
          <div><dt class="text-gray-500">站长主站用户</dt><dd class="mt-1 break-all font-mono font-medium">{{ provisioning.owner_main_user_id }}</dd></div>
          <div><dt class="text-gray-500">显示名称</dt><dd class="mt-1 font-medium">{{ provisioning.display_name || '—' }}</dd></div>
          <div><dt class="text-gray-500">站点标题</dt><dd class="mt-1 font-medium">{{ provisioning.site_name || '—' }}</dd></div>
          <div><dt class="text-gray-500">状态过期阈值</dt><dd class="mt-1 font-medium">{{ provisioning.stale_after_seconds }} 秒</dd></div>
          <div><dt class="text-gray-500">生命周期权威</dt><dd class="mt-1 font-medium">Sub2API 主站</dd></div>
          <div><dt class="text-gray-500">管理范围</dt><dd class="mt-1 font-medium">当前代理站只读</dd></div>
          <div><dt class="text-gray-500">最近检查</dt><dd class="mt-1 font-medium">{{ formatTime(provisioning.last_checked_at) }}</dd></div>
        </dl>
      </section>

      <section class="card p-5 sm:p-6" aria-labelledby="lifecycle-title">
        <div class="flex items-start gap-3">
          <Icon name="server" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div class="min-w-0 flex-1">
            <h2 id="lifecycle-title" class="font-semibold text-gray-900 dark:text-white">生命周期</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">状态由主站推进；本站页面不提供跨代理站控制按钮。</p>
          </div>
        </div>
        <ol class="mt-5 grid gap-3 sm:grid-cols-5" aria-label="代理站生命周期">
          <li v-for="(step, index) in lifecycleSteps" :key="step" :class="['rounded-xl border p-4', index === currentStep ? 'border-primary-400 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/10' : index < currentStep ? 'border-green-200 bg-green-50/50 dark:border-green-900/40 dark:bg-green-900/10' : 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50']">
            <div class="flex items-center gap-2">
              <span :class="['flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold', index <= currentStep ? 'bg-primary-600 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600']">{{ index + 1 }}</span>
              <span class="text-sm font-medium">{{ statusLabel(step) }}</span>
            </div>
          </li>
        </ol>
      </section>

      <section class="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-800 dark:border-sky-900/40 dark:bg-sky-900/10 dark:text-sky-200">
        <Icon name="shield" size="sm" class="mt-0.5 shrink-0" />
        <div>
          <p class="font-medium">数据与操作边界</p>
          <p class="mt-1">这里只能查看当前代理站状态，不能创建其他代理站，也不能激活、暂停、恢复或撤销主站实例。主站控制凭据、应用凭据、管理员 Key、SuperKey、数据库路径和部署信息不会进入浏览器。</p>
        </div>
      </section>
    </template>
  </main>
</template>
