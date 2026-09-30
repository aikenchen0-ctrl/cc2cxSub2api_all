<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAdminOrder, type AgentUserView } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentPagination from '@/components/AgentPagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const orders = ref<AgentAdminOrder[]>([])
const users = ref<AgentUserView[]>([])
const loading = ref(false)
const usersLoading = ref(false)
const actionLoading = ref(false)
const error = ref('')
const success = ref('')
const status = ref('')
const selectedUser = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const detail = ref<AgentAdminOrder | null>(null)
const cancelTarget = ref<AgentAdminOrder | null>(null)

const columns = [
  { key: 'id', label: '订单 ID' },
  { key: 'user_email', label: '本站用户' },
  { key: 'out_trade_no', label: '订单号' },
  { key: 'pay_amount', label: '支付金额' },
  { key: 'payment_type', label: '支付方式' },
  { key: 'order_type', label: '订单类型' },
  { key: 'status', label: '状态' },
  { key: 'created_at', label: '创建时间' },
  { key: 'actions', label: '操作' },
]

const statusOptions = [
  { value: '', label: '全部状态' }, { value: 'PENDING', label: '待支付' }, { value: 'PAID', label: '已支付' }, { value: 'COMPLETED', label: '已完成' },
  { value: 'EXPIRED', label: '已过期' }, { value: 'CANCELLED', label: '已取消' }, { value: 'FAILED', label: '失败' },
  { value: 'REFUND_REQUESTED', label: '退款申请中' }, { value: 'REFUND_PENDING', label: '退款处理中' },
  { value: 'PARTIALLY_REFUNDED', label: '部分退款' }, { value: 'REFUNDED', label: '已退款' }, { value: 'REFUND_FAILED', label: '退款失败' },
]
const userOptions = computed(() => [
  { value: '', label: '全部本站用户' },
  ...users.value.map(user => ({ value: user.main_user_id, label: user.display_name || user.email || user.main_user_id })),
])

const statusLabel: Record<string, string> = {
  PENDING: '待支付', PAID: '已支付', COMPLETED: '已完成', EXPIRED: '已过期',
  CANCELLED: '已取消', FAILED: '失败', REFUND_REQUESTED: '退款申请中',
  REFUNDING: '退款处理中', REFUND_PENDING: '退款处理中', PARTIALLY_REFUNDED: '部分退款',
  REFUNDED: '已退款', REFUND_FAILED: '退款失败',
}

const hasOrders = computed(() => total.value > 0)

function money(value: number, currency = 'USD') {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: currency || 'USD' }).format(value) }
  catch { return `${currency || '$'} ${Number(value || 0).toFixed(2)}` }
}

function date(value?: string) {
  return value ? new Date(value).toLocaleString('zh-CN') : '—'
}

function displayUser(order: AgentAdminOrder) {
  return order.user_display_name || order.user_email || order.main_user_id
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const result = await agentAPI.getUsers(1, 500)
    users.value = result.items || []
  } catch (cause) {
    error.value = errorMessage(cause, '本站用户加载失败，请稍后重试。')
  } finally {
    usersLoading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.adminOrders.list(page.value, pageSize.value, status.value, selectedUser.value)
    orders.value = result.items || []
    total.value = result.total || 0
  } catch (cause) {
    error.value = errorMessage(cause, '订单加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function changeFilter() {
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

async function confirmCancel() {
  if (!cancelTarget.value) return
  actionLoading.value = true
  error.value = ''
  success.value = ''
  try {
    await agentAPI.adminOrders.cancel(cancelTarget.value.main_user_id, cancelTarget.value.id)
    success.value = `订单 #${cancelTarget.value.id} 已取消。`
    cancelTarget.value = null
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '取消订单失败，请稍后重试。')
  } finally {
    actionLoading.value = false
  }
}

onMounted(() => {
  void loadUsers()
  void load()
})
</script>

<template>
  <section class="space-y-4" aria-label="本站订单管理">
    <div class="card flex flex-wrap items-center gap-3 p-4">
      <AgentSelect v-model="selectedUser" :options="userOptions" class="min-w-56" aria-label="筛选本站用户" :disabled="usersLoading" @change="changeFilter" />
      <AgentSelect v-model="status" :options="statusOptions" class="w-44" aria-label="筛选订单状态" @change="changeFilter" />
      <button class="btn btn-secondary ml-auto" type="button" :disabled="loading" aria-label="刷新站点订单" @click="load">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新
      </button>
    </div>

    <p class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-200">
      这里只聚合当前代理站已登记用户的主站订单。支付、到账和退款事实仍以 Sub2API 主站为准；本站不能查看其他代理站用户或修改主站支付配置。
    </p>
    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>
    <p v-if="success" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700" role="status">{{ success }}</p>

    <div class="card overflow-hidden">
      <DataTable :columns="columns" :data="orders" :loading="loading" row-key="id" :clickable-rows="true" @row-click="detail = $event">
        <template #empty><div class="py-8 text-center text-gray-500">暂无本站用户订单</div></template>
        <template #cell-id="{ value }"><span class="font-mono">#{{ value }}</span></template>
        <template #cell-user_email="{ row }"><div class="min-w-40"><p class="font-medium text-gray-900 dark:text-white">{{ displayUser(row) }}</p><p class="font-mono text-xs text-gray-400">ID {{ row.main_user_id }}</p></div></template>
        <template #cell-out_trade_no="{ value }"><span class="font-mono text-xs">{{ value }}</span></template>
        <template #cell-pay_amount="{ value, row }"><div><span class="font-semibold">{{ money(value, row.currency) }}</span><p v-if="row.amount !== row.pay_amount" class="text-xs text-gray-400">到账 {{ money(row.amount) }}</p></div></template>
        <template #cell-payment_type="{ value }"><span class="capitalize">{{ value || '—' }}</span></template>
        <template #cell-order_type="{ value }">{{ value === 'subscription' ? '订阅' : '余额充值' }}</template>
        <template #cell-status="{ value }"><AgentStatusBadge :status="value" :label="statusLabel[value] || value" /></template>
        <template #cell-created_at="{ value }"><span class="text-xs text-gray-500">{{ date(value) }}</span></template>
        <template #cell-actions="{ row }"><div class="flex gap-2"><button class="text-xs font-medium text-primary-600" type="button" @click.stop="detail = row">详情</button><button v-if="row.status === 'PENDING'" class="text-xs font-medium text-amber-600" type="button" @click.stop="cancelTarget = row">取消</button></div></template>
      </DataTable>
      <AgentPagination v-if="hasOrders" :total="total" :page="page" :page-size="pageSize" item-label="笔订单" @update:page="changePage" @update:page-size="changePageSize" />
    </div>
  </section>

  <BaseDialog :show="detail !== null" title="订单详情" width="wide" @close="detail = null">
    <div v-if="detail" class="grid gap-4 sm:grid-cols-2">
      <div><p class="text-xs text-gray-500">本站用户</p><p class="font-medium">{{ displayUser(detail) }}</p><p class="font-mono text-xs text-gray-400">{{ detail.main_user_id }}</p></div>
      <div><p class="text-xs text-gray-500">订单编号</p><p class="font-mono text-sm">{{ detail.out_trade_no }}</p></div>
      <div><p class="text-xs text-gray-500">状态</p><AgentStatusBadge class="mt-1" :status="detail.status" :label="statusLabel[detail.status] || detail.status" /></div>
      <div><p class="text-xs text-gray-500">支付金额</p><p class="font-medium">{{ money(detail.pay_amount, detail.currency) }}</p></div>
      <div><p class="text-xs text-gray-500">到账金额</p><p>{{ money(detail.amount) }}</p></div>
      <div><p class="text-xs text-gray-500">支付方式</p><p>{{ detail.payment_type || '—' }}</p></div>
      <div><p class="text-xs text-gray-500">创建时间</p><p>{{ date(detail.created_at) }}</p></div>
      <div><p class="text-xs text-gray-500">完成时间</p><p>{{ date(detail.completed_at) }}</p></div>
      <div v-if="detail.refund_amount"><p class="text-xs text-gray-500">退款金额</p><p class="text-red-600">{{ money(detail.refund_amount) }}</p></div>
      <div v-if="detail.refund_request_reason" class="sm:col-span-2"><p class="text-xs text-gray-500">退款申请原因</p><p class="whitespace-pre-wrap">{{ detail.refund_request_reason }}</p></div>
    </div>
  </BaseDialog>

  <BaseDialog :show="cancelTarget !== null" title="取消待支付订单" width="narrow" @close="cancelTarget = null">
    <p class="text-sm text-gray-600 dark:text-gray-300">确定取消 {{ cancelTarget ? displayUser(cancelTarget) : '' }} 的订单 #{{ cancelTarget?.id }} 吗？此操作会直接作用于该用户在 Sub2API 主站的订单。</p>
    <template #footer><button class="btn btn-secondary" type="button" @click="cancelTarget = null">返回</button><button class="btn btn-danger" type="button" :disabled="actionLoading" data-testid="confirm-admin-order-cancel" @click="confirmCancel">确认取消</button></template>
  </BaseDialog>
</template>
