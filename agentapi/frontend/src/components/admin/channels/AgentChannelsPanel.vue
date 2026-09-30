<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  agentAPI,
  type AgentAvailableChannel,
  type AgentAvailableGroup,
  type AgentAvailableModel,
  type AgentModelPolicy,
} from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentPlatformBadge from '@/components/common/AgentPlatformBadge.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import Icon from '@/components/icons/Icon.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'

const props = defineProps<{
  mode: 'channels' | 'pricing'
  policy: AgentModelPolicy | null
  enabled: string[]
  saving?: boolean
  loadFailed?: boolean
}>()

const emit = defineEmits<{
  'update:enabled': [value: string[]]
  save: []
}>()

interface ChannelModelRow {
  key: string
  channel: string
  description: string
  platform: string
  groups: AgentAvailableGroup[]
  model: AgentAvailableModel
}

const channels = ref<AgentAvailableChannel[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const platform = ref('')
const status = ref<'' | 'enabled' | 'disabled'>('')

const title = computed(() => props.mode === 'pricing' ? '模型定价' : '渠道管理')
const rows = computed<ChannelModelRow[]>(() => channels.value.flatMap(channel =>
  channel.platforms.flatMap(section => section.supported_models.map((model, index) => ({
    key: `${channel.name}:${section.platform}:${model.name}:${index}`,
    channel: channel.name,
    description: channel.description,
    platform: section.platform,
    groups: section.groups,
    model,
  }))),
))
const platforms = computed(() => [...new Set(rows.value.map(row => row.platform).filter(Boolean))].sort())
const platformOptions = computed(() => [
  { value: '', label: '全部平台' },
  ...platforms.value.map(value => ({ value, label: value })),
])
const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'enabled', label: '本站已启用' },
  { value: 'disabled', label: '本站未启用' },
]
const enabledSet = computed(() => new Set(props.enabled))
const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase()
  return rows.value.filter((row) => {
    const enabled = enabledSet.value.has(row.model.name)
    if (platform.value && row.platform !== platform.value) return false
    if (status.value === 'enabled' && !enabled) return false
    if (status.value === 'disabled' && enabled) return false
    if (!query) return true
    return [row.channel, row.description, row.platform, row.model.name, ...row.groups.map(group => group.name)]
      .some(value => value.toLowerCase().includes(query))
  })
})

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.getAdminChannels()
    channels.value = result.channels || []
  } catch (err) {
    channels.value = []
    error.value = errorMessage(err, '加载主站渠道资料失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function toggleModel(model: string): void {
  const next = new Set(props.enabled)
  if (next.has(model)) next.delete(model)
  else next.add(model)
  emit('update:enabled', [...next])
}

function groupLabel(groups: AgentAvailableGroup[]): string {
  return groups.map(group => group.name).join('、') || '-'
}

function money(value: number | null | undefined, scale = 1): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `$${(value * scale).toLocaleString('en-US', { maximumFractionDigits: 6 })}`
}

function billingMode(model: AgentAvailableModel): string {
  const mode = model.pricing?.billing_mode
  if (mode === 'per_request') return '按次'
  if (mode === 'image') return '图片'
  if (mode === 'token') return 'Token'
  return mode || '-'
}

onMounted(() => { void load() })
</script>

<template>
  <section class="space-y-5" :aria-label="title">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <div class="flex items-center gap-2">
          <Icon :name="mode === 'pricing' ? 'dollar' : 'server'" size="lg" class="text-primary-600 dark:text-primary-400" />
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ mode === 'pricing' ? '查看主站对当前站长账户公开的模型计费信息。' : '查看主站渠道目录，并配置当前代理站允许公开的模型。' }}
        </p>
      </div>
      <button v-if="mode === 'channels'" class="btn btn-primary" type="button" :disabled="saving || !policy" data-testid="save-channel-policy" @click="emit('save')">
        <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="['mr-2', saving ? 'animate-spin' : '']" />
        {{ saving ? '正在保存…' : '保存本站模型策略' }}
      </button>
    </div>

    <div class="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
      <Icon name="shield" size="sm" class="mt-1 shrink-0" />
      <p>渠道、分组与价格来自 Sub2API 主站，只读展示；此页只能修改当前代理站的模型允许列表，不会修改主站全局渠道、账号或定价。</p>
    </div>

    <p v-if="loadFailed" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200" role="alert">
      本站模型策略加载失败。渠道资料仍可查看，但暂时不能可靠保存模型开关。
    </p>

    <TablePageLayout>
      <template #filters>
        <div class="grid gap-3 lg:grid-cols-[minmax(240px,1fr)_180px_160px_auto]">
          <AgentSearchInput v-model="search" placeholder="搜索渠道、分组或模型" aria-label="搜索管理渠道" />
          <AgentSelect v-model="platform" :options="platformOptions" aria-label="筛选渠道平台" />
          <AgentSelect v-model="status" :options="statusOptions" aria-label="筛选本站状态" />
          <button type="button" class="btn btn-secondary" :disabled="loading" aria-label="刷新管理渠道" @click="load">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新
          </button>
        </div>
      </template>

      <template #table>
        <div class="table-wrapper">
          <div v-if="loading" class="flex min-h-56 items-center justify-center" data-testid="admin-channels-loading">
            <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
          </div>
          <div v-else-if="error" class="px-6 py-12 text-center text-sm text-red-600" role="alert">
            <p>{{ error }}</p>
            <button type="button" class="mt-3 font-medium underline" @click="load">重新加载</button>
          </div>
          <div v-else-if="filteredRows.length === 0" class="px-6 py-12 text-center text-sm text-gray-500">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
            <p>没有符合条件的渠道模型。</p>
          </div>

          <table v-else-if="mode === 'channels'" class="w-full border-collapse text-sm" data-testid="admin-channels-table">
            <thead><tr><th>渠道</th><th>平台</th><th>可用分组</th><th>模型</th><th>本站状态</th><th class="text-right">操作</th></tr></thead>
            <tbody>
              <tr v-for="row in filteredRows" :key="row.key" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/50">
                <td><p class="font-medium text-gray-900 dark:text-white">{{ row.channel }}</p><p class="mt-1 max-w-xs text-xs text-gray-500">{{ row.description || '-' }}</p></td>
                <td><AgentPlatformBadge :platform="row.platform" /></td>
                <td class="max-w-xs text-xs text-gray-600 dark:text-dark-300">{{ groupLabel(row.groups) }}</td>
                <td class="font-mono text-xs text-gray-900 dark:text-white">{{ row.model.name }}</td>
                <td><span :class="enabledSet.has(row.model.name) ? 'badge badge-success' : 'badge badge-gray'">{{ enabledSet.has(row.model.name) ? '已启用' : '未启用' }}</span></td>
                <td class="text-right"><button type="button" class="btn btn-sm btn-secondary" :data-testid="`toggle-${row.model.name}`" @click="toggleModel(row.model.name)">{{ enabledSet.has(row.model.name) ? '停用本站模型' : '启用本站模型' }}</button></td>
              </tr>
            </tbody>
          </table>

          <table v-else class="w-full border-collapse text-sm" data-testid="admin-pricing-table">
            <thead><tr><th>模型</th><th>渠道 / 平台</th><th>计费模式</th><th>输入 / 1M</th><th>输出 / 1M</th><th>缓存写 / 1M</th><th>缓存读 / 1M</th><th>按次</th><th>本站状态</th></tr></thead>
            <tbody>
              <tr v-for="row in filteredRows" :key="row.key" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/50">
                <td class="font-mono text-xs font-medium text-gray-900 dark:text-white">{{ row.model.name }}</td>
                <td><p>{{ row.channel }}</p><AgentPlatformBadge class="mt-1" :platform="row.platform" /></td>
                <td>{{ billingMode(row.model) }}</td>
                <td>{{ money(row.model.pricing?.input_price, 1_000_000) }}</td>
                <td>{{ money(row.model.pricing?.output_price, 1_000_000) }}</td>
                <td>{{ money(row.model.pricing?.cache_write_price, 1_000_000) }}</td>
                <td>{{ money(row.model.pricing?.cache_read_price, 1_000_000) }}</td>
                <td>{{ money(row.model.pricing?.per_request_price) }}</td>
                <td><span :class="enabledSet.has(row.model.name) ? 'badge badge-success' : 'badge badge-gray'">{{ enabledSet.has(row.model.name) ? '已启用' : '未启用' }}</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </section>
</template>
