<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAdminSubscription, type AgentUserView } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentPagination from '@/components/AgentPagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import AgentGroupBadge from '@/components/common/AgentGroupBadge.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const subscriptions = ref<AgentAdminSubscription[]>([])
const users = ref<AgentUserView[]>([])
const loading = ref(false)
const usersLoading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const status = ref('')
const selectedUser = ref('')
const selectedGroup = ref('')
const selectedPlatform = ref('')
const detail = ref<AgentAdminSubscription | null>(null)
const knownGroups = ref<Array<{ id: number; name: string; platform: string }>>([])

const columns = [
  { key: 'user_email', label: '本站用户' },
  { key: 'group', label: '订阅分组' },
  { key: 'usage', label: '额度使用' },
  { key: 'expires_at', label: '到期时间' },
  { key: 'status', label: '状态' },
  { key: 'actions', label: '操作' },
]

const statusOptions = [
  { value: '', label: '全部状态' }, { value: 'active', label: '生效中' }, { value: 'expired', label: '已过期' },
  { value: 'revoked', label: '已撤销' }, { value: 'suspended', label: '已暂停' },
]

const platformOptions = computed(() => [...new Set(knownGroups.value.map(item => item.platform).filter(Boolean))].sort())
const userOptions = computed(() => [
  { value: '', label: '全部本站用户' },
  ...users.value.map(user => ({ value: user.main_user_id, label: user.display_name || user.email || user.main_user_id })),
])
const groupOptions = computed(() => [
  { value: '', label: '全部分组' },
  ...knownGroups.value.map(group => ({ value: String(group.id), label: `${group.name} (#${group.id})` })),
])
const platformSelectOptions = computed(() => [
  { value: '', label: '全部平台' },
  ...platformOptions.value.map(platform => ({ value: platform, label: platform })),
])
const hasRows = computed(() => total.value > 0)

function displayUser(item: AgentAdminSubscription) {
  return item.user_display_name || item.user_email || item.main_user_id
}

function statusLabel(value: string) {
  const labels: Record<string, string> = { active: '生效中', expired: '已过期', revoked: '已撤销', suspended: '已暂停' }
  return labels[value] || value
}

function date(value?: string) {
  if (!value) return '永久有效'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('zh-CN')
}

function remaining(value?: string) {
  if (!value) return ''
  const time = new Date(value).getTime()
  if (!Number.isFinite(time)) return ''
  const days = Math.ceil((time - Date.now()) / 86400000)
  if (days < 0) return `已过期 ${Math.abs(days)} 天`
  if (days === 0) return '今天到期'
  return `剩余 ${days} 天`
}

function money(value: number | null | undefined) {
  return `$${Number(value || 0).toFixed(2)}`
}

function quotaRows(item: AgentAdminSubscription) {
  return [
    { label: '日', used: item.daily_usage_usd, limit: item.group?.daily_limit_usd },
    { label: '周', used: item.weekly_usage_usd, limit: item.group?.weekly_limit_usd },
    { label: '月', used: item.monthly_usage_usd, limit: item.group?.monthly_limit_usd },
  ].filter(row => row.limit != null)
}

function percent(used: number, limit: number | null | undefined) {
  if (!limit) return '0%'
  return `${Math.min(100, Math.max(0, used / limit * 100))}%`
}

function barClass(used: number, limit: number | null | undefined) {
  if (!limit) return 'bg-gray-400'
  const ratio = used / limit
  return ratio >= 0.9 ? 'bg-red-500' : ratio >= 0.7 ? 'bg-amber-500' : 'bg-emerald-500'
}

function rememberGroups(items: AgentAdminSubscription[]) {
  const byID = new Map(knownGroups.value.map(item => [item.id, item]))
  for (const item of items) {
    if (!item.group) continue
    byID.set(item.group.id, { id: item.group.id, name: item.group.name, platform: item.group.platform })
  }
  knownGroups.value = [...byID.values()].sort((left, right) => left.name.localeCompare(right.name))
}

async function loadUsers() {
  usersLoading.value = true
  try { users.value = (await agentAPI.getUsers(1, 500)).items || [] }
  catch (cause) { error.value = errorMessage(cause, '本站用户加载失败，请稍后重试。') }
  finally { usersLoading.value = false }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.adminSubscriptions.list(page.value, pageSize.value, {
      ...(status.value ? { status: status.value } : {}),
      ...(selectedUser.value ? { main_user_id: selectedUser.value } : {}),
      ...(selectedGroup.value ? { group_id: Number(selectedGroup.value) } : {}),
      ...(selectedPlatform.value ? { platform: selectedPlatform.value } : {}),
    })
    subscriptions.value = result.items || []
    total.value = result.total || 0
    rememberGroups(subscriptions.value)
  } catch (cause) {
    subscriptions.value = []
    total.value = 0
    error.value = errorMessage(cause, '订阅列表加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
}

function changePage(value: number) {
  page.value = value
  void load()
}

function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
  void load()
}

onMounted(() => {
  void loadUsers()
  void load()
})
</script>

<template>
  <section class="space-y-4" aria-label="本站订阅管理">
    <div class="card flex flex-wrap items-center gap-3 p-4">
      <AgentSelect v-model="selectedUser" :options="userOptions" class="min-w-56" aria-label="筛选订阅用户" :disabled="usersLoading" @change="applyFilters" />
      <AgentSelect v-model="status" :options="statusOptions" class="w-40" aria-label="筛选订阅状态" @change="applyFilters" />
      <AgentSelect v-model="selectedGroup" :options="groupOptions" class="min-w-44" aria-label="筛选订阅分组" @change="applyFilters" />
      <AgentSelect v-model="selectedPlatform" :options="platformSelectOptions" class="w-40" aria-label="筛选订阅平台" @change="applyFilters" />
      <button class="btn btn-secondary ml-auto" type="button" :disabled="loading" aria-label="刷新订阅管理" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
    </div>

    <div class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-200">
      这里按当前代理站用户归属聚合 Sub2API 主站订阅和额度事实。本站站长可查看归属与使用情况，但不能免费分配、延长、恢复或重置主站订阅，也不能查看其他代理站用户。
    </div>
    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>

    <div class="card overflow-hidden">
      <DataTable :columns="columns" :data="subscriptions" :loading="loading" row-key="id" :clickable-rows="true" @row-click="detail = $event">
        <template #empty><div class="py-10 text-center text-gray-500">暂无符合条件的本站用户订阅</div></template>
        <template #cell-user_email="{ row }"><div class="min-w-44"><p class="font-medium text-gray-900 dark:text-white">{{ displayUser(row) }}</p><p class="font-mono text-xs text-gray-400">ID {{ row.main_user_id }}</p></div></template>
        <template #cell-group="{ row }"><div class="min-w-44"><AgentGroupBadge :group-id="row.group_id" :name="row.group?.name || `分组 #${row.group_id}`" :platform="row.group?.platform" :subscription-type="row.group?.subscription_type || 'subscription'" :rate-multiplier="row.group?.rate_multiplier ?? 1" :peak-rate-enabled="row.group?.peak_rate_enabled" :peak-start="row.group?.peak_start" :peak-end="row.group?.peak_end" :peak-rate-multiplier="row.group?.peak_rate_multiplier" :always-show-rate="true" /></div></template>
        <template #cell-usage="{ row }">
          <div v-if="quotaRows(row).length" class="min-w-64 space-y-2">
            <div v-for="quota in quotaRows(row)" :key="quota.label" class="grid grid-cols-[24px_1fr_auto] items-center gap-2"><span class="text-xs text-gray-500">{{ quota.label }}</span><div class="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600"><div class="h-full rounded-full" :class="barClass(quota.used, quota.limit)" :style="{ width: percent(quota.used, quota.limit) }" /></div><span class="text-xs text-gray-500">{{ money(quota.used) }}/{{ money(quota.limit) }}</span></div>
          </div>
          <span v-else class="badge badge-success">不限周期额度</span>
        </template>
        <template #cell-expires_at="{ value }"><div><p class="text-sm">{{ date(value) }}</p><p v-if="remaining(value)" class="text-xs text-gray-400">{{ remaining(value) }}</p></div></template>
        <template #cell-status="{ value }"><AgentStatusBadge :status="value" :label="statusLabel(value)" /></template>
        <template #cell-actions="{ row }"><button class="text-sm font-medium text-primary-600" type="button" @click.stop="detail = row">详情</button></template>
      </DataTable>
      <AgentPagination v-if="hasRows" :total="total" :page="page" :page-size="pageSize" item-label="条订阅" @update:page="changePage" @update:page-size="changePageSize" />
    </div>
  </section>

  <BaseDialog :show="detail !== null" title="订阅详情" width="wide" @close="detail = null">
    <div v-if="detail" class="space-y-5">
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <p class="text-xs text-gray-500">本站用户</p>
          <p class="font-medium">{{ displayUser(detail) }}</p>
          <p v-if="detail.user_email" class="text-sm text-gray-500 dark:text-gray-400">{{ detail.user_email }}</p>
          <p class="font-mono text-xs text-gray-400">ID {{ detail.main_user_id }}</p>
        </div>
        <div><p class="text-xs text-gray-500">订阅分组</p><AgentGroupBadge class="mt-1" :group-id="detail.group_id" :name="detail.group?.name || `分组 #${detail.group_id}`" :platform="detail.group?.platform" :subscription-type="detail.group?.subscription_type || 'subscription'" :rate-multiplier="detail.group?.rate_multiplier ?? 1" :peak-rate-enabled="detail.group?.peak_rate_enabled" :peak-start="detail.group?.peak_start" :peak-end="detail.group?.peak_end" :peak-rate-multiplier="detail.group?.peak_rate_multiplier" :always-show-rate="true" /></div>
        <div><p class="text-xs text-gray-500">生效时间</p><p>{{ date(detail.starts_at) }}</p></div>
        <div><p class="text-xs text-gray-500">到期时间</p><p>{{ date(detail.expires_at) }}</p><p class="text-xs text-gray-400">{{ remaining(detail.expires_at) }}</p></div>
        <div><p class="text-xs text-gray-500">状态</p><AgentStatusBadge class="mt-1" :status="detail.status" :label="statusLabel(detail.status)" /></div>
        <div><p class="text-xs text-gray-500">费率倍率</p><p>×{{ detail.group?.rate_multiplier ?? 1 }}</p></div>
      </div>
      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
        <h3 class="font-medium text-gray-900 dark:text-white">周期额度</h3>
        <div v-if="quotaRows(detail).length" class="mt-4 space-y-4"><div v-for="quota in quotaRows(detail)" :key="quota.label"><div class="flex justify-between text-sm"><span>{{ quota.label }}额度</span><span>{{ money(quota.used) }} / {{ money(quota.limit) }}</span></div><div class="mt-2 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600"><div class="h-full rounded-full" :class="barClass(quota.used, quota.limit)" :style="{ width: percent(quota.used, quota.limit) }" /></div></div></div>
        <p v-else class="mt-3 text-sm text-emerald-600">当前订阅未设置日、周或月周期额度。</p>
      </div>
    </div>
  </BaseDialog>
</template>
