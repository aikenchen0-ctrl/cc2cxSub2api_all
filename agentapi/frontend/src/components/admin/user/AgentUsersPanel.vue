<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { agentAPI, type AgentUserView } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import Icon from '@/components/icons/Icon.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import AgentPagination from '@/components/AgentPagination.vue'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import AgentUserKeys from '@/components/AgentUserKeys.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import type { Column } from '@/components/common/types'

const props = withDefaults(defineProps<{ ownerMainUserId?: string }>(), { ownerMainUserId: '' })

type UserStatusFilter = '' | 'active' | 'disabled'
type StatusAction = { users: AgentUserView[]; status: 'active' | 'disabled' }

const users = ref<AgentUserView[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const searchInput = ref('')
const search = ref('')
const status = ref<UserStatusFilter>('')
const loading = ref(false)
const error = ref('')
const notice = ref('')
const selectedIds = ref<Array<string | number>>([])
const statusAction = ref<StatusAction | null>(null)
const updating = ref(false)
const keyUser = ref<AgentUserView | null>(null)
const showColumns = ref(false)
const visibleColumns = ref(new Set(['status', 'balance', 'created_at', 'updated_at']))
let sequence = 0

const columnChoices = [
  { key: 'status', label: '本站状态' },
  { key: 'balance', label: '主站余额' },
  { key: 'created_at', label: '关联时间' },
  { key: 'updated_at', label: '更新时间' },
]
const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '已启用' },
  { value: 'disabled', label: '已停用' },
]

const columns = computed<Column[]>(() => [
  { key: 'user', label: '用户', sortable: true },
  ...(visibleColumns.value.has('status') ? [{ key: 'status', label: '本站状态', sortable: true }] : []),
  ...(visibleColumns.value.has('balance') ? [{ key: 'balance', label: 'Sub2API 实时余额', sortable: true }] : []),
  ...(visibleColumns.value.has('created_at') ? [{ key: 'created_at', label: '关联时间', sortable: true }] : []),
  ...(visibleColumns.value.has('updated_at') ? [{ key: 'updated_at', label: '更新时间', sortable: true }] : []),
  { key: 'actions', label: '操作' },
])

const selectedUsers = computed(() => {
  const ids = new Set(selectedIds.value.map(String))
  return users.value.filter(user => ids.has(user.main_user_id) && !isOwner(user))
})

const confirmationMessage = computed(() => {
  const action = statusAction.value
  if (!action) return ''
  if (action.users.length === 1) {
    const user = action.users[0]
    const account = user.email || user.main_user_id
    return action.status === 'disabled'
      ? `确定停用 ${account} 在本代理站的访问吗？现有 Session 和客户端 Key 将立即停止使用。`
      : `确定重新启用 ${account} 在本代理站的访问吗？启用后 Session 和客户端 Key 可恢复使用。`
  }
  return action.status === 'disabled'
    ? `确定停用选中的 ${action.users.length} 名用户在本代理站的访问吗？此操作不会修改主站账号状态。`
    : `确定启用选中的 ${action.users.length} 名用户在本代理站的访问吗？`
})

function isOwner(user: AgentUserView): boolean {
  return user.main_user_id === props.ownerMainUserId
}

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

function dateTime(value: string): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN')
}

async function load(): Promise<void> {
  const current = ++sequence
  loading.value = true
  error.value = ''
  try {
    const result = status.value
      ? await agentAPI.getUsers(page.value, pageSize.value, search.value, status.value)
      : await agentAPI.getUsers(page.value, pageSize.value, search.value)
    if (current !== sequence) return
    const lastPage = Math.max(1, Math.ceil(result.total / pageSize.value))
    if (page.value > lastPage) {
      page.value = lastPage
      await load()
      return
    }
    users.value = result.items
    total.value = result.total
    const visible = new Set(result.items.map(user => user.main_user_id))
    selectedIds.value = selectedIds.value.filter(id => visible.has(String(id)))
  } catch (err) {
    if (current === sequence) error.value = errorMessage(err, '加载关联用户失败，请刷新重试。')
  } finally {
    if (current === sequence) loading.value = false
  }
}

function applySearch(value: string): void {
  const next = value.trim()
  if (next === search.value) return
  search.value = next
  page.value = 1
  void load()
}

function applyStatusFilter(): void {
  page.value = 1
  selectedIds.value = []
  void load()
}

function toggleColumn(key: string): void {
  const next = new Set(visibleColumns.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  visibleColumns.value = next
}

function requestStatus(user: AgentUserView): void {
  if (isOwner(user) || updating.value) return
  statusAction.value = { users: [user], status: user.status === 'active' ? 'disabled' : 'active' }
}

function requestBulkStatus(nextStatus: 'active' | 'disabled'): void {
  const targets = selectedUsers.value.filter(user => user.status !== nextStatus)
  if (targets.length === 0 || updating.value) return
  statusAction.value = { users: targets, status: nextStatus }
}

async function confirmStatus(): Promise<void> {
  const action = statusAction.value
  if (!action || updating.value) return
  statusAction.value = null
  updating.value = true
  notice.value = ''
  try {
    const results = await Promise.allSettled(action.users.map(user => agentAPI.setMappedUserStatus(user.main_user_id, action.status)))
    const succeeded = results.filter(result => result.status === 'fulfilled').length
    const failed = results.length - succeeded
    notice.value = failed
      ? `已更新 ${succeeded} 名用户，${failed} 名用户更新失败；列表已按代理站实际状态刷新。`
      : action.users.length === 1 && action.status === 'disabled'
        ? '该用户在本代理站的访问已停用，新的 AgentAPI 请求将被拒绝。'
        : action.users.length === 1
          ? '该用户在本代理站的访问已启用。'
      : action.status === 'disabled'
        ? `已停用 ${succeeded} 名用户在本代理站的访问。`
        : `已启用 ${succeeded} 名用户在本代理站的访问。`
    selectedIds.value = []
    await load()
  } catch (err) {
    notice.value = errorMessage(err, '更新用户状态失败，请稍后重试。')
    await load()
  } finally {
    updating.value = false
  }
}

async function changePage(next: number): Promise<void> {
  if (next === page.value) return
  page.value = next
  selectedIds.value = []
  await load()
}

async function changePageSize(next: number): Promise<void> {
  if (next === pageSize.value) return
  pageSize.value = next
  page.value = 1
  selectedIds.value = []
  await load()
}

onMounted(load)
onUnmounted(() => {
  sequence += 1
})
</script>

<template>
  <TablePageLayout>
    <template #actions>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">代理站用户</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">这里只管理本站关联关系、本站访问状态和本站 API Key；不会修改 Sub2API 全局账号配置。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button v-if="selectedUsers.length" class="btn btn-secondary" type="button" :disabled="updating" @click="requestBulkStatus('active')">
            <Icon name="checkCircle" size="sm" class="mr-2" />批量启用
          </button>
          <button v-if="selectedUsers.length" class="btn btn-danger" type="button" :disabled="updating" @click="requestBulkStatus('disabled')">
            <Icon name="ban" size="sm" class="mr-2" />批量停用
          </button>
          <button class="btn btn-secondary" type="button" :disabled="loading" title="刷新" @click="load">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>
      <p v-if="error" role="alert" class="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <p v-if="notice" class="mt-4 rounded-xl bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>
    </template>

    <template #filters>
      <div class="flex flex-wrap items-center gap-3">
        <div class="w-full md:w-72">
          <AgentSearchInput v-model="searchInput" aria-label="搜索关联用户" maxlength="128" placeholder="邮箱、姓名或 Sub2API 用户编号" @search="applySearch" />
        </div>
        <AgentSelect v-model="status" :options="statusOptions" aria-label="筛选用户状态" class="w-full sm:w-40" @change="applyStatusFilter" />
        <div class="relative ml-auto">
          <button class="btn btn-secondary" type="button" @click="showColumns = !showColumns">
            <Icon name="grid" size="sm" class="mr-2" />列设置
          </button>
          <div v-if="showColumns" class="absolute right-0 top-full z-40 mt-2 w-48 rounded-xl border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800">
            <button v-for="column in columnChoices" :key="column.key" class="flex w-full items-center justify-between px-4 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700" type="button" @click="toggleColumn(column.key)">
              <span>{{ column.label }}</span><Icon v-if="visibleColumns.has(column.key)" name="check" size="sm" class="text-primary-500" />
            </button>
          </div>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable
        :columns="columns"
        :data="users"
        :loading="loading"
        row-key="main_user_id"
        selectable
        :selected-keys="selectedIds"
        :selection-label="user => `选择 ${user.email || user.main_user_id}`"
        :actions-count="2"
        default-sort-key="created_at"
        default-sort-order="desc"
        sort-storage-key="agent-admin-users-sort"
        @update:selected-keys="selectedIds = $event.filter(id => String(id) !== ownerMainUserId)"
      >
        <template #cell-user="{ row: user }">
          <div class="flex min-w-56 items-center gap-3">
            <div class="flex h-9 w-9 flex-none items-center justify-center rounded-full bg-primary-100 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">{{ (user.display_name || user.email || '?').charAt(0).toUpperCase() }}</div>
            <div class="min-w-0"><div class="truncate font-medium text-gray-900 dark:text-white">{{ user.display_name || user.email || user.main_user_id }}</div><div class="truncate text-xs text-gray-500">{{ user.email || user.main_user_id }} · {{ user.main_user_id }}</div></div>
          </div>
        </template>
        <template #cell-status="{ row: user }"><div class="flex flex-wrap items-center gap-2"><AgentStatusBadge :status="user.status" :label="user.status === 'active' ? '已启用' : '已停用'" /><span v-if="isOwner(user)" class="badge badge-purple">站长</span></div></template>
        <template #cell-balance="{ row: user }"><span v-if="!user.balance_error" class="font-medium">¥{{ money(user.balance_cents) }}</span><span v-else class="text-xs text-amber-600 dark:text-amber-400">暂时无法读取</span></template>
        <template #cell-created_at="{ value }"><span class="whitespace-nowrap text-sm">{{ dateTime(value) }}</span></template>
        <template #cell-updated_at="{ value }"><span class="whitespace-nowrap text-sm">{{ dateTime(value) }}</span></template>
        <template #cell-actions="{ row: user }">
          <div class="flex flex-wrap items-center gap-2">
            <button class="btn btn-ghost btn-sm" type="button" @click="keyUser = user"><Icon name="key" size="sm" class="mr-1.5" />管理 API Key</button>
            <button v-if="!isOwner(user)" :class="['btn btn-sm', user.status === 'active' ? 'btn-danger' : 'btn-success']" type="button" :disabled="updating" @click="requestStatus(user)">{{ user.status === 'active' ? '停用账号' : '启用账号' }}</button>
            <span v-else class="text-xs text-gray-500">站长账号不可停用</span>
          </div>
        </template>
        <template #empty>{{ error ? '加载关联用户失败，请刷新重试。' : '暂无符合条件的关联用户。' }}</template>
      </DataTable>
    </template>

    <template #pagination>
      <AgentPagination v-if="!loading && total > 0" :total="total" :page="page" :page-size="pageSize" item-label="名用户" :page-size-options="[10, 25, 50, 100, 200]" @update:page="changePage" @update:page-size="changePageSize" />
    </template>
  </TablePageLayout>

  <BaseDialog :show="keyUser !== null" :title="`管理 ${keyUser?.email || keyUser?.main_user_id || ''} 的 API Key`" width="wide" @close="keyUser = null">
    <AgentUserKeys v-if="keyUser" :key="keyUser.main_user_id" :user-id="keyUser.main_user_id" />
  </BaseDialog>
  <AgentConfirmDialog :open="statusAction !== null" :title="statusAction?.status === 'disabled' ? '停用本站访问' : '启用本站访问'" :message="confirmationMessage" :confirm-label="statusAction?.status === 'disabled' ? '确认停用' : '确认启用'" :destructive="statusAction?.status === 'disabled'" @cancel="statusAction = null" @confirm="confirmStatus" />
</template>
