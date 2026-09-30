<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI } from '@/agent/api'
import { siteLogo, siteName } from '@/agent/branding'
import { errorMessage } from '@/agent/client'
import { groupCatalogModels, modelCapability, type AgentModelCapability } from '@/agent/modelCatalog'
import { useAgentSession } from '@/agent/session'
import { isDark, toggleTheme } from '@/agent/theme'
import Icon from '@/components/icons/Icon.vue'
import AgentModelPlazaFilterBar from '@/components/modelPlaza/AgentModelPlazaFilterBar.vue'
import AgentModelGroupSection from '@/components/modelPlaza/AgentModelGroupSection.vue'

const models = ref<string[]>([])
const session = useAgentSession()
const loading = ref(true)
const error = ref('')
const selectedProvider = ref('all')
const selectedCapability = ref<AgentModelCapability | 'all'>('all')
const search = ref('')

const groups = computed(() => groupCatalogModels(models.value))
const providers = computed(() => groups.value.map((group) => group.provider))
const filteredGroups = computed(() => {
  const query = search.value.trim().toLowerCase()
  return groups.value
    .filter((group) => selectedProvider.value === 'all' || group.provider === selectedProvider.value)
    .map((group) => ({
      ...group,
      models: group.models.filter((model) =>
        (selectedCapability.value === 'all' || model.capability === selectedCapability.value)
        && (!query || model.name.toLowerCase().includes(query)),
      ),
    }))
    .filter((group) => group.models.length > 0)
})
const capabilityCounts = computed(() => ({
  text: models.value.filter((model) => modelCapability(model) === 'text').length,
  image: models.value.filter((model) => modelCapability(model) === 'image').length,
  video: models.value.filter((model) => modelCapability(model) === 'video').length,
}))
const dashboardPath = computed(() => session.isAgentAdmin ? '/admin/dashboard' : '/dashboard')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    models.value = await agentAPI.model.publicList()
  } catch (err) {
    models.value = []
    error.value = errorMessage(err, '加载可用模型失败，请稍后刷新重试。')
  } finally {
    loading.value = false
  }
}

onMounted(() => { void load() })
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="sticky top-0 z-30 border-b border-gray-200/70 bg-white/90 backdrop-blur dark:border-dark-700 dark:bg-dark-900/90">
      <div class="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3" :aria-label="siteName">
          <span class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700"><img :src="siteLogo" alt="" class="h-full w-full object-contain"></span>
          <span class="truncate text-base font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
        </RouterLink>
        <nav class="flex items-center gap-1 sm:gap-2" aria-label="公开导航">
          <RouterLink to="/home" class="hidden rounded-lg px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white sm:inline-flex">首页</RouterLink>
          <RouterLink to="/downloads" class="hidden rounded-lg px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white md:inline-flex">客户端下载</RouterLink>
          <button type="button" class="btn-ghost btn-icon" :aria-label="isDark ? '切换浅色模式' : '切换深色模式'" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" size="sm" /></button>
          <RouterLink v-if="session.isAuthenticated" :to="dashboardPath" class="btn btn-primary px-3 py-2 text-sm">进入控制台</RouterLink>
          <RouterLink v-else to="/login" class="btn btn-primary px-3 py-2 text-sm">登录</RouterLink>
        </nav>
      </div>
    </header>

    <main class="mx-auto max-w-7xl space-y-5 px-4 py-8 sm:px-6 lg:px-8 lg:py-10">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-300">MODEL PLAZA</p>
        <h1 class="mt-1 text-2xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-3xl">模型广场</h1>
        <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">浏览当前代理站已开放的模型，并直接进入工作台使用。</p>
      </div>
      <button type="button" class="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200" :disabled="loading" @click="load">
        <Icon name="refresh" size="sm" />刷新目录
      </button>
    </header>

    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="模型目录统计">
      <div v-for="item in [{ label: '可用模型', value: models.length }, { label: '文本模型', value: capabilityCounts.text }, { label: '图片模型', value: capabilityCounts.image }, { label: '视频模型', value: capabilityCounts.video }]" :key="item.label" class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800/60">
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
      </div>
    </section>

    <div class="flex items-start gap-2 rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm leading-6 text-blue-800 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-200">
      <Icon name="infoCircle" size="sm" class="mt-1 shrink-0" />
      <p>目录来自当前代理站的租户模型策略。代理站不维护独立定价，模型价格、余额变化和最终用量均以 Sub2API 主站实际记录为准。</p>
    </div>

    <AgentModelPlazaFilterBar
      :providers="providers"
      :provider="selectedProvider"
      :capability="selectedCapability"
      :search="search"
      @update:provider="selectedProvider = $event"
      @update:capability="selectedCapability = $event"
      @update:search="search = $event"
    />

    <div v-if="loading" class="flex min-h-60 items-center justify-center"><span class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600"></span></div>
    <div v-else-if="error" role="alert" class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
      <p>{{ error }}</p>
      <button type="button" class="mt-3 font-medium underline" @click="load">重新加载</button>
    </div>
    <div v-else-if="filteredGroups.length" class="space-y-5">
      <AgentModelGroupSection v-for="group in filteredGroups" :key="group.provider" :group="group" />
    </div>
    <div v-else class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
      {{ models.length ? '没有符合当前筛选条件的模型。' : '当前代理站尚未启用任何模型。' }}
    </div>
    </main>

    <footer class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-2 px-4 pb-8 pt-2 text-xs text-gray-400 sm:px-6 lg:px-8">
      <span>{{ siteName }} · 模型目录</span>
      <span>登录后可创建 API Key 并使用模型工作台</span>
    </footer>
  </div>
</template>
