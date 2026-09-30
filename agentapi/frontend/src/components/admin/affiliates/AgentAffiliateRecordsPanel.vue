<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  agentAPI,
  type AgentAffiliateInviteRecord,
  type AgentAffiliateRebateRecord,
  type AgentAffiliateRecordFilters,
  type AgentAffiliateTransferRecord,
} from '@/agent/api'
import AgentPagination from '@/components/AgentPagination.vue'
import DataTable from '@/components/common/DataTable.vue'
import AgentDateRangePicker from '@/components/common/AgentDateRangePicker.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import { errorMessage } from '@/agent/locale'

type RecordType = 'invites' | 'rebates' | 'transfers'
type AffiliateRecord = AgentAffiliateInviteRecord | AgentAffiliateRebateRecord | AgentAffiliateTransferRecord

const props = defineProps<{ type: RecordType }>()
const records = ref<AffiliateRecord[]>([])
const loading = ref(false)
const error = ref('')
const filters = reactive({ search: '', start_at: '', end_at: '' })
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const sortBy = ref('created_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const columns = computed(() => {
  if (props.type === 'invites') return [
    { key: 'inviter', label: '邀请人', sortable: true },
    { key: 'invitee', label: '被邀请人', sortable: true },
    { key: 'aff_code', label: '邀请码', sortable: true },
    { key: 'total_rebate', label: '累计返利', sortable: true },
    { key: 'created_at', label: '邀请时间', sortable: true },
  ]
  if (props.type === 'rebates') return [
    { key: 'order', label: '主站订单', sortable: true },
    { key: 'inviter', label: '邀请人', sortable: true },
    { key: 'invitee', label: '被邀请人', sortable: true },
    { key: 'order_amount', label: '订单金额', sortable: true },
    { key: 'pay_amount', label: '支付金额', sortable: true },
    { key: 'rebate_amount', label: '返利金额', sortable: true },
    { key: 'payment_type', label: '支付方式', sortable: true },
    { key: 'order_status', label: '订单状态', sortable: true },
    { key: 'created_at', label: '返利时间', sortable: true },
  ]
  return [
    { key: 'user', label: '本站用户', sortable: true },
    { key: 'amount', label: '划转金额', sortable: true },
    { key: 'balance_after', label: '划转后余额', sortable: true },
    { key: 'available_quota_after', label: '划转后可用返利', sortable: true },
    { key: 'frozen_quota_after', label: '划转后冻结返利', sortable: true },
    { key: 'history_quota_after', label: '历史返利', sortable: true },
    { key: 'created_at', label: '划转时间', sortable: true },
  ]
})

const emptyText = computed(() => ({ invites: '暂无本站用户邀请记录', rebates: '暂无本站用户返利记录', transfers: '暂无本站用户划转记录' })[props.type])

function params(): AgentAffiliateRecordFilters {
  return {
    ...(filters.search.trim() ? { search: filters.search.trim() } : {}),
    ...(filters.start_at ? { start_at: filters.start_at } : {}),
    ...(filters.end_at ? { end_at: filters.end_at } : {}),
    sort_by: sortBy.value,
    sort_order: sortOrder.value,
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = props.type === 'invites'
      ? await agentAPI.adminAffiliates.invites(page.value, pageSize.value, params())
      : props.type === 'rebates'
        ? await agentAPI.adminAffiliates.rebates(page.value, pageSize.value, params())
        : await agentAPI.adminAffiliates.transfers(page.value, pageSize.value, params())
    records.value = result.items || []
    total.value = result.total || 0
  } catch (cause) {
    records.value = []
    total.value = 0
    error.value = errorMessage(cause, '推广记录加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function reloadFirstPage() { page.value = 1; void load() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function changeSort(key: string, order: 'asc' | 'desc') { sortBy.value = key; sortOrder.value = order; page.value = 1; void load() }
function money(value?: number | null) { return value == null ? '—' : `$${Number(value).toFixed(2)}` }
function date(value: string) { const parsed = new Date(value); return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('zh-CN') }
function user(email: string, name: string, id: number) { return name || email || `用户 #${id}` }

onMounted(load)
</script>

<template>
  <section class="space-y-4" :aria-label="emptyText">
    <div class="card flex flex-wrap items-center gap-3 p-4">
      <AgentSearchInput v-model="filters.search" class="w-full md:w-80" aria-label="搜索推广记录" placeholder="搜索用户、邀请码或订单" @search="reloadFirstPage" />
      <AgentDateRangePicker v-model:start-date="filters.start_at" v-model:end-date="filters.end_at" aria-label="推广记录日期范围" @change="reloadFirstPage" />
      <button class="btn btn-secondary ml-auto" type="button" :disabled="loading" aria-label="刷新推广记录" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
    </div>

    <div class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-200">
      记录由 Sub2API 主站权威计费数据生成；这里只聚合当前代理站名下用户。跨代理站邀请关系会在服务端隐藏，站长不能修改主站返利或余额。
    </div>
    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>

    <div class="card overflow-hidden">
      <DataTable :columns="columns" :data="records" :loading="loading" row-key="created_at" :server-side-sort="true" default-sort-key="created_at" default-sort-order="desc" @sort="changeSort">
        <template #empty><div class="py-10 text-center text-gray-500">{{ emptyText }}</div></template>
        <template #cell-inviter="{ row }"><div><p class="font-medium">{{ user(row.inviter_email, row.inviter_username, row.inviter_id) }}</p><p class="text-xs text-gray-400">{{ row.inviter_email }} · ID {{ row.inviter_id }}</p></div></template>
        <template #cell-invitee="{ row }"><div><p class="font-medium">{{ user(row.invitee_email, row.invitee_username, row.invitee_id) }}</p><p class="text-xs text-gray-400">{{ row.invitee_email }} · ID {{ row.invitee_id }}</p></div></template>
        <template #cell-user="{ row }"><div><p class="font-medium">{{ user(row.user_email, row.username, row.user_id) }}</p><p class="text-xs text-gray-400">{{ row.user_email }} · ID {{ row.user_id }}</p></div></template>
        <template #cell-aff_code="{ value }"><span class="font-mono">{{ value || '—' }}</span></template>
        <template #cell-order="{ row }"><div><p class="font-mono">#{{ row.order_id }}</p><p class="max-w-48 truncate text-xs text-gray-400">{{ row.out_trade_no }}</p></div></template>
        <template #cell-total_rebate="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-order_amount="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-pay_amount="{ value }"><span>¥{{ Number(value || 0).toFixed(2) }}</span></template>
        <template #cell-rebate_amount="{ value }"><span class="font-semibold text-emerald-600">{{ money(value) }}</span></template>
        <template #cell-amount="{ value }"><span class="font-semibold text-emerald-600">{{ money(value) }}</span></template>
        <template #cell-balance_after="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-available_quota_after="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-frozen_quota_after="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-history_quota_after="{ value }"><span>{{ money(value) }}</span></template>
        <template #cell-order_status="{ value }"><AgentStatusBadge :status="value || 'unknown'" :label="value || '—'" /></template>
        <template #cell-created_at="{ value }"><span>{{ date(value) }}</span></template>
      </DataTable>
      <AgentPagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" item-label="条推广记录" @update:page="changePage" @update:page-size="changePageSize" />
    </div>
  </section>
</template>
