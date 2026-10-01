<script setup lang="ts">
import { computed, ref } from 'vue'
import type { SettlementView } from '@/agent/api'
import { errorMessage, statusLabel } from '@/agent/locale'
import DataTable from '@/components/common/DataTable.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'

const props = defineProps<{
  items: SettlementView[]
  loading?: boolean
  loadFailed?: boolean
  reconciling?: boolean
}>()

defineEmits<{ reconcile: [] }>()

const search = ref('')
const status = ref('')
const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '待同步' },
  { value: 'confirmed', label: '已同步' },
  { value: 'failed', label: '同步失败' },
]
const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return props.items.filter(item => {
    if (status.value && item.status !== status.value) return false
    if (!query) return true
    return [item.request_id, item.proxy_main_user_id, item.billing_main_user_id, item.usage_id, item.model]
      .some(value => String(value || '').toLowerCase().includes(query))
  })
})

function formatMoney(cents?: number): string {
  return `¥${(Math.max(0, cents || 0) / 100).toFixed(2)}`
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function settlementLabel(value: string): string {
  if (value === 'pending') return '待同步'
  if (value === 'confirmed') return '已同步'
  if (value === 'failed') return '同步失败'
  return statusLabel(value)
}

</script>

<template>
  <TablePageLayout>
    <template #actions>
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="sync" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">用量同步</h2>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">核对本站请求与已同步用量记录；此处仅用于查看和核对。</p>
        </div>
        <button type="button" class="btn btn-primary" :disabled="reconciling" @click="$emit('reconcile')">
          <Icon name="refresh" size="sm" :class="['mr-2', reconciling ? 'animate-spin' : '']" />
          {{ reconciling ? '正在核对…' : '核对待结算记录' }}
        </button>
      </div>
    </template>

    <template #filters>
      <div class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div class="flex flex-1 flex-wrap items-end gap-4">
            <div class="w-full sm:min-w-[260px] sm:flex-1">
              <label for="settlement-search" class="input-label">搜索同步记录</label>
              <div class="mt-2"><AgentSearchInput id="settlement-search" v-model="search" :debounce-ms="0" placeholder="请求编号、用户、用量编号或模型" /></div>
            </div>
            <div class="w-full sm:w-44">
              <label for="settlement-status" class="input-label">同步状态</label>
              <AgentSelect id="settlement-status" v-model="status" :options="statusOptions" class="mt-2" aria-label="筛选同步状态" />
            </div>
          </div>
          <button type="button" class="btn btn-secondary" :disabled="!search && !status" @click="search = ''; status = ''">重置</button>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable
        :columns='[{"key":"request_id","label":"请求编号"},{"key":"user","label":"归属用户"},{"key":"model","label":"模型"},{"key":"cost","label":"实际费用"},{"key":"status","label":"同步状态"},{"key":"updated_at","label":"更新时间"},{"key":"error","label":"说明"}]'
        :data="filteredItems"
        :loading="loading"
        row-key="request_id"
      >
        <template #cell-request_id="{ row: item }"><span class="font-mono text-xs text-gray-700 dark:text-dark-200">{{ item.request_id }}</span></template>
        <template #cell-user="{ row: item }">
          <div class="min-w-0 max-w-[220px]">
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.proxy_main_user_id || '—' }}</p>
            <p v-if="item.billing_main_user_id && item.billing_main_user_id !== item.proxy_main_user_id" class="truncate text-xs text-gray-400">计费：{{ item.billing_main_user_id }}</p>
          </div>
        </template>
        <template #cell-model="{ row: item }"><span class="font-mono text-xs">{{ item.model || '—' }}</span></template>
        <template #cell-cost="{ row: item }"><span :class="item.status === 'pending' ? 'text-gray-400' : 'font-medium text-gray-900 dark:text-white'">{{ item.status === 'pending' ? '待确认' : formatMoney(item.actual_cents) }}</span></template>
        <template #cell-status="{ row: item }"><AgentStatusBadge :status="item.status" :label="settlementLabel(item.status)" /></template>
        <template #cell-updated_at="{ row: item }"><span class="whitespace-nowrap text-gray-500 dark:text-dark-400">{{ formatTime(item.updated_at || item.created_at) }}</span></template>
        <template #cell-error="{ row: item }"><span class="max-w-xs whitespace-normal text-sm text-gray-500 dark:text-dark-400">{{ errorMessage({ message: item.error }, item.status === 'pending' ? '等待用量记录' : '—') }}</span></template>
        <template #empty>
          <div class="flex flex-col items-center py-8">
            <Icon :name="loadFailed ? 'exclamationTriangle' : 'sync'" size="xl" class="mb-3 text-gray-300 dark:text-dark-600" />
            <p class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ loadFailed ? '加载待结算记录失败，请刷新重试。' : search || status ? '没有符合筛选条件的同步记录。' : '暂无待结算记录。' }}</p>
          </div>
        </template>
      </DataTable>
    </template>

    <template #pagination>
      <p class="text-sm text-gray-500 dark:text-dark-400">显示 {{ filteredItems.length }} / {{ items.length }} 条本站同步记录</p>
    </template>
  </TablePageLayout>
</template>
