<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentContentPagesPanel from '@/components/admin/content/AgentContentPagesPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const loading = ref(true)
const error = ref('')
const panel = ref<InstanceType<typeof AgentContentPagesPanel> | null>(null)

async function loadContext(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    context.value = await agentAPI.getContext()
    await nextTick()
    await panel.value?.load()
  } catch (cause) {
    context.value = null
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全管理本站内容页面。')
  } finally {
    loading.value = false
  }
}

onMounted(() => void loadContext())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">站点内容</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">内容页面</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">管理当前代理站的公开协议和自定义 Markdown 页面，页面外观沿用主站，但内容与发布状态按代理站隔离。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading && !context" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站内容边界…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载内容页面</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <section class="grid gap-4 md:grid-cols-3">
        <div class="card p-5">
          <Icon name="globe" size="lg" class="text-primary-500" />
          <h2 class="mt-3 font-semibold text-gray-900 dark:text-white">公开协议</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">发布后通过本站 `/legal/:slug` 向访客公开。</p>
        </div>
        <div class="card p-5">
          <Icon name="document" size="lg" class="text-violet-500" />
          <h2 class="mt-3 font-semibold text-gray-900 dark:text-white">自定义页面</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">登录用户通过本站 `/custom/:slug` 查看。</p>
        </div>
        <div class="card p-5">
          <Icon name="shield" size="lg" class="text-emerald-500" />
          <h2 class="mt-3 font-semibold text-gray-900 dark:text-white">租户隔离</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">只修改当前代理站内容，不覆盖 Sub2API 主站页面。</p>
        </div>
      </section>

      <AgentContentPagesPanel ref="panel" :auto-load="false" />

      <section class="card border-dashed p-5">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div>
            <h2 class="font-medium text-gray-900 dark:text-white">发布边界</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">页面编号、正文、排序和状态只保存在 AgentAPI 当前 `agent_id` 下。代理站管理员不能读取或修改其他代理站内容、Sub2API 主站协议、主站首页正文或主站全局菜单配置。</p>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>
