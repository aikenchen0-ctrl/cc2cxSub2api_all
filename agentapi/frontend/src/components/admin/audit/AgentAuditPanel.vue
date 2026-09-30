<script setup lang="ts">
import { computed, ref } from 'vue'
import type { AuditEventView } from '@/agent/api'
import { operationLabel, statusLabel, targetLabel } from '@/agent/locale'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import Icon from '@/components/icons/Icon.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'

const props = defineProps<{
  events: AuditEventView[]
  loading?: boolean
  loadFailed?: boolean
}>()
const emit = defineEmits<{
  refresh: []
}>()

const query = ref('')
const actorType = ref('')
const result = ref('')
const detail = ref<AuditEventView | null>(null)
const filteredEvents = computed(() => {
  const search = query.value.trim().toLowerCase()
  return props.events.filter(event => {
    if (actorType.value && event.actor_type !== actorType.value) return false
    if (result.value && event.result !== result.value) return false
    if (!search) return true
    return [event.operation, event.actor_id, event.target_type, event.target_id, event.request_id, event.reason]
      .some(value => String(value || '').toLowerCase().includes(search))
  })
})
const actorTypes = computed(() => [...new Set(props.events.map(event => event.actor_type).filter(Boolean))].sort())
const resultValues = computed(() => [...new Set(props.events.map(event => event.result).filter(Boolean))].sort())
const actorTypeOptions = computed(() => [
  { value: '', label: '全部类型' },
  ...actorTypes.value.map(value => ({ value, label: statusLabel(value) })),
])
const resultOptions = computed(() => [
  { value: '', label: '全部结果' },
  ...resultValues.value.map(value => ({ value, label: statusLabel(value) })),
])

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function resultClass(value: string): string {
  if (value === 'success' || value === 'ok') return 'badge badge-success'
  if (value === 'failed' || value === 'error' || value === 'denied') return 'badge badge-danger'
  return 'badge badge-warning'
}

function resetFilters(): void {
  query.value = ''
  actorType.value = ''
  result.value = ''
}
</script>

<template>
  <TablePageLayout>
    <template #actions>
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="shield" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">操作日志</h2>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">查看当前代理站最近的安全与管理操作。日志不保存密钥明文、认证头或主站内部凭据。</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" aria-label="刷新操作日志" @click="emit('refresh')">
          <Icon name="refresh" size="sm" :class="['mr-2', loading ? 'animate-spin' : '']" />刷新
        </button>
      </div>
    </template>

    <template #filters>
      <div class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div class="flex flex-1 flex-wrap items-end gap-4">
            <div class="w-full sm:min-w-[260px] sm:flex-1">
              <label for="audit-search" class="input-label">搜索日志</label>
              <div class="mt-2"><AgentSearchInput id="audit-search" v-model="query" :debounce-ms="0" placeholder="操作、操作者、对象或请求编号" /></div>
            </div>
            <div class="w-full sm:w-44">
              <label for="audit-actor" class="input-label">操作者类型</label>
              <AgentSelect id="audit-actor" v-model="actorType" :options="actorTypeOptions" class="mt-2" aria-label="筛选操作者类型" />
            </div>
            <div class="w-full sm:w-40">
              <label for="audit-result" class="input-label">结果</label>
              <AgentSelect id="audit-result" v-model="result" :options="resultOptions" class="mt-2" aria-label="筛选操作结果" />
            </div>
          </div>
          <button type="button" class="btn btn-secondary" :disabled="!query && !actorType && !result" @click="resetFilters">重置</button>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable
        :columns='[{"key":"created_at","label":"时间"},{"key":"actor","label":"操作者"},{"key":"operation","label":"操作"},{"key":"target","label":"对象"},{"key":"result","label":"结果"},{"key":"request_id","label":"请求编号"},{"key":"actions","label":"详情"}]'
        :data="filteredEvents"
        :loading="loading"
        row-key="id"
      >
        <template #cell-created_at="{ row: event }"><span class="whitespace-nowrap text-gray-600 dark:text-dark-300">{{ formatTime(event.created_at) }}</span></template>
        <template #cell-actor="{ row: event }">
          <div class="min-w-0 max-w-[220px]"><p class="truncate font-medium text-gray-900 dark:text-white">{{ event.actor_id || '—' }}</p><p class="mt-0.5 text-xs text-gray-400">{{ statusLabel(event.actor_type) }}</p></div>
        </template>
        <template #cell-operation="{ row: event }"><span class="font-mono text-sm text-gray-800 dark:text-dark-200">{{ operationLabel(event.operation) }}</span></template>
        <template #cell-target="{ row: event }"><div><p>{{ targetLabel(event.target_type) }}</p><p v-if="event.target_id" class="font-mono text-xs text-gray-400">{{ event.target_id }}</p></div></template>
        <template #cell-result="{ row: event }"><span :class="resultClass(event.result)">{{ statusLabel(event.result) }}</span></template>
        <template #cell-request_id="{ row: event }"><span class="font-mono text-xs text-gray-500">{{ event.request_id }}</span></template>
        <template #cell-actions="{ row: event }"><button type="button" class="inline-flex items-center gap-1 font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="detail = event"><Icon name="eye" size="sm" />查看</button></template>
        <template #empty>
          <div class="flex flex-col items-center py-8">
            <Icon :name="loadFailed ? 'exclamationTriangle' : 'shield'" size="xl" class="mb-3 text-gray-300 dark:text-dark-600" />
            <p class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ loadFailed ? '加载审计记录失败，请刷新重试。' : query || actorType || result ? '没有符合筛选条件的日志。' : '暂无审计记录。' }}</p>
          </div>
        </template>
      </DataTable>
    </template>

    <template #pagination><p class="text-sm text-gray-500 dark:text-dark-400">显示 {{ filteredEvents.length }} / {{ events.length }} 条本站最近日志</p></template>
  </TablePageLayout>

  <BaseDialog :show="detail !== null" title="操作日志详情" width="wide" @close="detail = null">
    <div v-if="detail" class="space-y-5">
      <div class="flex flex-wrap items-center gap-3 rounded-xl border border-gray-200 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-900/60">
        <span :class="resultClass(detail.result)">{{ statusLabel(detail.result) }}</span>
        <p class="font-mono font-medium text-gray-900 dark:text-white">{{ operationLabel(detail.operation) }}</p>
        <span class="ml-auto text-sm text-gray-500">{{ formatTime(detail.created_at) }}</span>
      </div>
      <dl class="grid gap-4 sm:grid-cols-2">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><dt class="text-xs text-gray-500">操作者</dt><dd class="mt-1 break-all font-mono text-sm">{{ statusLabel(detail.actor_type) }} · {{ detail.actor_id }}</dd></div>
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><dt class="text-xs text-gray-500">操作对象</dt><dd class="mt-1 break-all font-mono text-sm">{{ targetLabel(detail.target_type) }}<span v-if="detail.target_id"> · {{ detail.target_id }}</span></dd></div>
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700 sm:col-span-2"><dt class="text-xs text-gray-500">请求编号</dt><dd class="mt-1 break-all font-mono text-sm">{{ detail.request_id || '—' }}</dd></div>
        <div v-if="detail.reason" class="rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/60 dark:bg-amber-950/20 sm:col-span-2"><dt class="text-xs text-amber-700 dark:text-amber-300">说明</dt><dd class="mt-1 whitespace-pre-wrap text-sm text-amber-900 dark:text-amber-100">{{ detail.reason }}</dd></div>
      </dl>
    </div>
  </BaseDialog>
</template>
