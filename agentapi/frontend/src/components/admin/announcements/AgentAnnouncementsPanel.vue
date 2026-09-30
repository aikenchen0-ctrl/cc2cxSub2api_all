<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { agentAPI, type AgentAnnouncement, type AgentAnnouncementInput, type AgentAnnouncementStatus } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentPagination from '@/components/AgentPagination.vue'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import AgentTextArea from '@/components/common/AgentTextArea.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'

const allItems = ref<AgentAnnouncement[]>([])
const loading = ref(false)
const saving = ref(false)
const notice = ref('')
const error = ref('')
const search = ref('')
const status = ref<'' | AgentAnnouncementStatus>('')
const page = ref(1)
const pageSize = ref(20)
const sortKey = ref<'created_at' | 'title' | 'status' | 'notify_mode'>('created_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const showEditDialog = ref(false)
const showDeleteDialog = ref(false)
const editing = ref<AgentAnnouncement | null>(null)
const deleting = ref<AgentAnnouncement | null>(null)
const previewing = ref<AgentAnnouncement | null>(null)
const form = reactive<AgentAnnouncementInput>({ title: '', content: '', status: 'draft', notify_mode: 'silent', starts_at: '', ends_at: '' })
const announcementStatusOptions = [
  { value: 'draft', label: '草稿' },
  { value: 'active', label: '已发布' },
  { value: 'archived', label: '已归档' },
]
const announcementFilterOptions = [{ value: '', label: '全部状态' }, ...announcementStatusOptions]
const notifyModeOptions = [
  { value: 'silent', label: '仅铃铛显示' },
  { value: 'popup', label: '首次进入弹窗' },
]

const columns: Column[] = [
  { key: 'title', label: '标题', sortable: true },
  { key: 'status', label: '状态', sortable: true },
  { key: 'notify_mode', label: '提醒方式', sortable: true },
  { key: 'time_range', label: '生效时间' },
  { key: 'created_at', label: '创建时间', sortable: true },
  { key: 'actions', label: '操作' },
]

const filteredItems = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return allItems.value.filter(item => {
    if (status.value && item.status !== status.value) return false
    return !query || `${item.title}\n${item.content}`.toLocaleLowerCase().includes(query)
  }).sort((left, right) => {
    const compared = String(left[sortKey.value] ?? '').localeCompare(String(right[sortKey.value] ?? ''), 'zh-CN', { numeric: true, sensitivity: 'base' })
    return sortOrder.value === 'asc' ? compared : -compared
  })
})

const pageItems = computed(() => filteredItems.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

function resetForm(): void {
  editing.value = null
  Object.assign(form, { title: '', content: '', status: 'draft', notify_mode: 'silent', starts_at: '', ends_at: '' })
}

function openCreate(): void {
  resetForm()
  showEditDialog.value = true
}

function openEdit(item: AgentAnnouncement): void {
  editing.value = item
  Object.assign(form, { title: item.title, content: item.content, status: item.status, notify_mode: item.notify_mode, starts_at: toLocalInput(item.starts_at), ends_at: toLocalInput(item.ends_at) })
  showEditDialog.value = true
}

function closeEdit(): void {
  showEditDialog.value = false
  resetForm()
}

function toLocalInput(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}

function toRFC3339(value?: string): string { return value ? new Date(value).toISOString() : '' }

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function statusLabel(value: AgentAnnouncementStatus): string {
  return ({ draft: '草稿', active: '已发布', archived: '已归档' } as const)[value]
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    allItems.value = (await agentAPI.adminAnnouncements.list()).items
    page.value = Math.min(page.value, Math.max(1, Math.ceil(filteredItems.value.length / pageSize.value)))
  } catch (cause) {
    allItems.value = []
    error.value = errorMessage(cause, '公告列表加载失败，请稍后重试。')
  } finally { loading.value = false }
}

function applyFilters(): void { page.value = 1 }

function handleSort(key: string, order: 'asc' | 'desc'): void {
  if (key === 'created_at' || key === 'title' || key === 'status' || key === 'notify_mode') {
    sortKey.value = key
    sortOrder.value = order
    page.value = 1
  }
}

async function save(): Promise<void> {
  if (!form.title.trim() || !form.content.trim()) { error.value = '标题和正文不能为空。'; return }
  saving.value = true
  error.value = ''
  notice.value = ''
  const payload: AgentAnnouncementInput = { title: form.title.trim(), content: form.content.trim(), status: form.status, notify_mode: form.notify_mode, starts_at: toRFC3339(form.starts_at), ends_at: toRFC3339(form.ends_at) }
  try {
    if (editing.value) await agentAPI.adminAnnouncements.update(editing.value.id, payload)
    else await agentAPI.adminAnnouncements.create(payload)
    notice.value = editing.value ? '公告已更新。' : '公告已创建。'
    closeEdit()
    await load()
  } catch (cause) { error.value = errorMessage(cause, '公告保存失败，请稍后重试。') }
  finally { saving.value = false }
}

function requestDelete(item: AgentAnnouncement): void { deleting.value = item; showDeleteDialog.value = true }

async function confirmDelete(): Promise<void> {
  const item = deleting.value
  showDeleteDialog.value = false
  deleting.value = null
  if (!item) return
  try {
    await agentAPI.adminAnnouncements.delete(item.id)
    notice.value = '公告已删除。'
    await load()
  } catch (cause) { error.value = errorMessage(cause, '公告删除失败，请稍后重试。') }
}

function changePageSize(value: number): void { pageSize.value = value; page.value = 1 }

onMounted(load)
</script>

<template>
  <section class="space-y-5">
    <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <div class="flex flex-wrap items-center gap-3">
        <AgentSearchInput v-model="search" class="min-w-56 flex-1 sm:max-w-72" placeholder="搜索公告标题或正文" aria-label="搜索公告" :debounce-ms="0" @search="applyFilters" />
        <AgentSelect v-model="status" :options="announcementFilterOptions" class="w-36" aria-label="公告状态" @change="applyFilters" />
        <div class="ml-auto flex items-center gap-2">
          <button class="btn btn-secondary" :disabled="loading" title="刷新" @click="load"><Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" /><span class="sr-only">刷新</span></button>
          <button class="btn btn-primary" data-test="create-announcement" @click="openCreate"><Icon name="plus" size="md" />创建公告</button>
        </div>
      </div>
    </div>

    <p class="rounded-lg border border-blue-100 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900/40 dark:bg-blue-950/30 dark:text-blue-200">公告仅属于当前代理站并按本站用户隔离，不会修改 Sub2API 主站公告。</p>
    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-lg bg-green-50 p-4 text-sm text-green-700 dark:bg-green-950/30 dark:text-green-300">{{ notice }}</p>

    <div class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <DataTable :columns="columns" :data="pageItems" :loading="loading" :server-side-sort="true" default-sort-key="created_at" default-sort-order="desc" row-key="id" @sort="handleSort">
        <template #cell-title="{ row }"><div class="max-w-72 min-w-0"><p class="truncate font-medium text-gray-900 dark:text-white">{{ row.title }}</p><p class="mt-1 line-clamp-2 whitespace-normal text-xs text-gray-500 dark:text-dark-400">{{ row.content }}</p></div></template>
        <template #cell-status="{ row }"><AgentStatusBadge :status="row.status" :label="statusLabel(row.status)" /></template>
        <template #cell-notify_mode="{ row }"><span class="badge" :class="row.notify_mode === 'popup' ? 'badge-warning' : 'badge-gray'">{{ row.notify_mode === 'popup' ? '首次进入弹窗' : '仅铃铛显示' }}</span></template>
        <template #cell-time_range="{ row }"><div class="space-y-1 text-xs text-gray-500 dark:text-dark-400"><p>开始：{{ row.starts_at ? formatTime(row.starts_at) : '立即' }}</p><p>结束：{{ row.ends_at ? formatTime(row.ends_at) : '永久' }}</p></div></template>
        <template #cell-created_at="{ row }"><span class="text-sm text-gray-500">{{ formatTime(row.created_at) }}</span></template>
        <template #cell-actions="{ row }"><div class="flex items-center justify-end gap-1"><button class="rounded-lg p-2 text-gray-500 hover:bg-blue-50 hover:text-blue-600" title="预览" @click="previewing = row"><Icon name="eye" size="sm" /><span class="sr-only">预览</span></button><button class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800" title="编辑" @click="openEdit(row)"><Icon name="edit" size="sm" /><span class="sr-only">编辑</span></button><button class="rounded-lg p-2 text-gray-500 hover:bg-red-50 hover:text-red-600" title="删除" @click="requestDelete(row)"><Icon name="trash" size="sm" /><span class="sr-only">删除</span></button></div></template>
        <template #empty><EmptyState title="暂无公告" description="创建第一条当前代理站公告。" action-text="创建公告" @action="openCreate" /></template>
      </DataTable>
      <div v-if="filteredItems.length" class="border-t border-gray-200 p-4 dark:border-dark-700"><AgentPagination :total="filteredItems.length" :page="page" :page-size="pageSize" item-label="条公告" @update:page="page = $event" @update:page-size="changePageSize" /></div>
    </div>

    <BaseDialog :show="showEditDialog" :title="editing ? '编辑公告' : '创建公告'" width="wide" @close="closeEdit">
      <form id="agent-announcement-form" class="space-y-4" @submit.prevent="save">
        <label class="block"><span class="input-label">标题</span><AgentInput v-model="form.title" maxlength="160" required placeholder="公告标题" /></label>
        <label class="block"><span class="input-label">正文</span><AgentTextArea v-model="form.content" class="min-h-44" maxlength="20000" required placeholder="公告正文（按原样安全显示）" /></label>
        <div class="grid gap-4 md:grid-cols-2"><label><span class="input-label">状态</span><AgentSelect v-model="form.status" :options="announcementStatusOptions" aria-label="公告编辑状态" /></label><label><span class="input-label">提醒方式</span><AgentSelect v-model="form.notify_mode" :options="notifyModeOptions" aria-label="公告提醒方式" /></label><label><span class="input-label">开始时间（可选）</span><AgentInput v-model="form.starts_at" type="datetime-local" /></label><label><span class="input-label">结束时间（可选）</span><AgentInput v-model="form.ends_at" type="datetime-local" /></label></div>
      </form>
      <template #footer><div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="closeEdit">取消</button><button type="submit" form="agent-announcement-form" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button></div></template>
    </BaseDialog>

    <BaseDialog :show="Boolean(previewing)" title="公告预览" width="normal" @close="previewing = null"><article v-if="previewing" class="space-y-4"><div class="flex items-center gap-2"><AgentStatusBadge :status="previewing.status" :label="statusLabel(previewing.status)" /><span v-if="previewing.notify_mode === 'popup'" class="badge badge-warning">弹窗</span></div><h2 class="text-xl font-semibold">{{ previewing.title }}</h2><p class="whitespace-pre-wrap break-words text-sm leading-7">{{ previewing.content }}</p></article></BaseDialog>

    <AgentConfirmDialog :open="showDeleteDialog" title="删除公告" :message="deleting ? `确定删除公告“${deleting.title}”吗？此操作不可撤销。` : '确定删除此公告吗？'" confirm-label="删除" destructive @confirm="confirmDelete" @cancel="showDeleteDialog = false; deleting = null" />
  </section>
</template>
