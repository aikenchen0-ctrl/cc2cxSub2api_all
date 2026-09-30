<script setup lang="ts">
import { computed, ref } from 'vue'
import type { AgentModelPolicy } from '@/agent/api'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  policy: AgentModelPolicy | null
  enabled: string[]
  saving?: boolean
  loadFailed?: boolean
}>()

const emit = defineEmits<{
  'update:enabled': [value: string[]]
  save: []
}>()

const search = ref('')
const selected = computed({
  get: () => props.enabled,
  set: value => emit('update:enabled', value),
})
const filteredModels = computed(() => {
  const query = search.value.trim().toLowerCase()
  return (props.policy?.catalog || []).filter(model => !query || model.toLowerCase().includes(query))
})
const selectedCount = computed(() => props.enabled.length)
const allFilteredSelected = computed(() => filteredModels.value.length > 0 && filteredModels.value.every(model => props.enabled.includes(model)))

function toggleVisible(): void {
  const next = new Set(props.enabled)
  if (allFilteredSelected.value) filteredModels.value.forEach(model => next.delete(model))
  else filteredModels.value.forEach(model => next.add(model))
  selected.value = [...next]
}

function clearAll(): void {
  selected.value = []
}
</script>

<template>
  <section class="space-y-6" aria-label="代理站模型权限">
    <div class="card overflow-hidden">
      <div class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="cube" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">模型权限</h2>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">控制当前代理站公开的模型允许列表。停用的模型会从本站 <code>/v1/models</code> 隐藏，并在请求主站前被拒绝。</p>
        </div>
        <button class="btn btn-primary" type="button" :disabled="saving || !policy" @click="emit('save')">
          <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="['mr-2', saving ? 'animate-spin' : '']" />
          {{ saving ? '正在保存…' : '保存模型权限' }}
        </button>
      </div>

      <div v-if="policy" class="space-y-5 p-6">
        <div class="grid gap-4 sm:grid-cols-3">
          <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700/50">
            <p class="text-xs text-gray-500 dark:text-dark-400">公开模型目录</p>
            <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ policy.catalog.length }}</p>
          </div>
          <div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/60 dark:bg-emerald-950/20">
            <p class="text-xs text-emerald-700 dark:text-emerald-300">本站已启用</p>
            <p class="mt-1 text-2xl font-semibold text-emerald-800 dark:text-emerald-200">{{ selectedCount }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-dark-400">策略来源</p>
            <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ policy.customized ? '本站自定义' : '继承公开目录' }}</p>
          </div>
        </div>

        <div class="flex flex-wrap items-end justify-between gap-4 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700/40">
          <div class="w-full sm:w-80">
            <label for="model-policy-search" class="input-label">搜索模型</label>
            <div class="mt-2"><AgentSearchInput id="model-policy-search" v-model="search" :debounce-ms="0" placeholder="按模型名称筛选" /></div>
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary" :disabled="filteredModels.length === 0" @click="toggleVisible">
              {{ allFilteredSelected ? '取消当前筛选' : '选择当前筛选' }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="selectedCount === 0" @click="clearAll">全部清除</button>
          </div>
        </div>

        <div v-if="filteredModels.length" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <label v-for="model in filteredModels" :key="model" class="group flex cursor-pointer items-center gap-3 rounded-xl border border-gray-200 bg-white p-4 transition-colors hover:border-primary-300 hover:bg-primary-50/40 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:bg-primary-950/10">
            <input v-model="selected" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" type="checkbox" :value="model">
            <div class="min-w-0 flex-1">
              <p class="truncate font-mono text-sm font-medium text-gray-900 dark:text-white" :title="model">{{ model }}</p>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ enabled.includes(model) ? '允许本站调用' : '不在本站允许列表' }}</p>
            </div>
            <Icon :name="enabled.includes(model) ? 'checkCircle' : 'xCircle'" size="sm" :class="enabled.includes(model) ? 'text-emerald-500' : 'text-gray-300 dark:text-dark-500'" />
          </label>
        </div>
        <div v-else class="rounded-xl border border-dashed border-gray-300 py-12 text-center dark:border-dark-600">
          <Icon name="search" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
          <p class="mt-3 text-sm font-medium text-gray-600 dark:text-dark-300">没有匹配的公开模型</p>
        </div>

        <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-200">
          此页面只控制本站允许列表，不会修改 Sub2API 的全局模型目录、渠道、账号或定价配置。
        </div>
      </div>
      <div v-else class="flex flex-col items-center py-14 text-center">
        <Icon :name="loadFailed ? 'exclamationTriangle' : 'refresh'" size="xl" :class="loadFailed ? 'text-amber-500' : 'animate-spin text-primary-500'" />
        <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ loadFailed ? '加载模型目录失败，请刷新重试。' : '正在加载模型目录…' }}</p>
      </div>
    </div>
  </section>
</template>
