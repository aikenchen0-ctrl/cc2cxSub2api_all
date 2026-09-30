<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { agentAPI, type AgentOrder } from '@/agent/api'
import AgentPagination from '@/components/AgentPagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import AgentTextArea from '@/components/common/AgentTextArea.vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'

const router = useRouter()
const orders = ref<AgentOrder[]>([])
const eligibleProviders = ref(new Set<string>())
const loading = ref(false)
const actionLoading = ref(false)
const error = ref('')
const success = ref('')
const status = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const cancelTarget = ref<AgentOrder | null>(null)
const refundTarget = ref<AgentOrder | null>(null)
const refundReason = ref('')

const columns = [
  { key: 'id', label: '订单 ID' }, { key: 'out_trade_no', label: '订单号' },
  { key: 'pay_amount', label: '支付金额' }, { key: 'payment_type', label: '支付方式' },
  { key: 'order_type', label: '订单类型' }, { key: 'status', label: '状态' },
  { key: 'created_at', label: '创建时间' }, { key: 'actions', label: '操作' },
]
const statusOptions = [
  { value: '', label: '全部状态' }, { value: 'PENDING', label: '待支付' }, { value: 'COMPLETED', label: '已完成' },
  { value: 'FAILED', label: '失败' }, { value: 'REFUND_REQUESTED', label: '退款申请中' }, { value: 'REFUNDED', label: '已退款' },
]
const statusLabel: Record<string, string> = {
  PENDING: '待支付', COMPLETED: '已完成', FAILED: '失败',
  REFUND_REQUESTED: '退款申请中', REFUNDED: '已退款', CANCELLED: '已取消',
}
const hasOrders = computed(() => total.value > 0)

function money(value: number, currency = 'USD') {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: currency || 'USD' }).format(value) }
  catch { return `${currency || '$'} ${value.toFixed(2)}` }
}
function date(value: string) { return value ? new Date(value).toLocaleString('zh-CN') : '—' }
function canRefund(order: AgentOrder) {
  return order.status === 'COMPLETED' && !!order.provider_instance_id && eligibleProviders.value.has(order.provider_instance_id)
}
async function load() {
  loading.value = true; error.value = ''
  try {
    const result = await agentAPI.orders.list(page.value, pageSize.value, status.value)
    orders.value = result.items || []; total.value = result.total || 0
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '订单加载失败' }
  finally { loading.value = false }
}
async function loadEligibility() {
  try { eligibleProviders.value = new Set((await agentAPI.orders.refundEligibleProviders()).provider_instance_ids || []) }
  catch { eligibleProviders.value = new Set() }
}
async function confirmCancel() {
  if (!cancelTarget.value) return
  actionLoading.value = true; error.value = ''
  try { await agentAPI.orders.cancel(cancelTarget.value.id); success.value = '订单已取消'; cancelTarget.value = null; await load() }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '取消订单失败' }
  finally { actionLoading.value = false }
}
async function confirmRefund() {
  if (!refundTarget.value || refundReason.value.trim().length < 2) return
  actionLoading.value = true; error.value = ''
  try { await agentAPI.orders.requestRefund(refundTarget.value.id, refundReason.value.trim()); success.value = '退款申请已提交'; refundTarget.value = null; refundReason.value = ''; await load() }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '退款申请失败' }
  finally { actionLoading.value = false }
}
function changeFilter() { page.value = 1; void load() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function openRefund(order: AgentOrder) { refundTarget.value = order; refundReason.value = '' }

onMounted(() => { void load(); void loadEligibility() })
</script>

<template>
  <main class="space-y-4">
    <header class="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
      <div><p class="text-sm font-medium text-primary-600 dark:text-primary-300">MAIN-SITE ORDERS</p><h1 class="mt-1 text-2xl font-bold tracking-tight sm:text-3xl">订单记录</h1><p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">订单、余额与退款状态均来自 Sub2API 主站，代理站不创建第二套计费事实。</p></div>
      <button class="btn btn-primary" type="button" @click="router.push('/purchase')">充值 / 购买</button>
    </header>
    <div class="card flex flex-wrap items-center gap-3 p-4">
      <AgentSelect v-model="status" :options="statusOptions" class="w-44" aria-label="订单状态" @change="changeFilter" />
      <button class="btn btn-secondary ml-auto" type="button" :disabled="loading" aria-label="刷新订单" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
    </div>
    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>
    <p v-if="success" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700" role="status">{{ success }}</p>
    <div class="card overflow-hidden">
      <DataTable :columns="columns" :data="orders" :loading="loading" row-key="id">
        <template #empty><div class="py-8 text-center text-gray-500">暂无主站订单</div></template>
        <template #cell-id="{ value }"><span class="font-mono">#{{ value }}</span></template>
        <template #cell-out_trade_no="{ value }"><span class="font-mono text-xs">{{ value }}</span></template>
        <template #cell-pay_amount="{ value, row }"><div><span class="font-semibold">{{ money(value, row.currency) }}</span><p v-if="row.amount !== row.pay_amount" class="text-xs text-gray-400">到账 {{ money(row.amount) }}</p></div></template>
        <template #cell-payment_type="{ value }"><span class="capitalize">{{ value || '—' }}</span></template>
        <template #cell-order_type="{ value }">{{ value === 'subscription' ? '订阅' : '余额充值' }}</template>
        <template #cell-status="{ value }"><AgentStatusBadge :status="value" :label="statusLabel[value] || value" /></template>
        <template #cell-created_at="{ value }"><span class="text-xs text-gray-500">{{ date(value) }}</span></template>
        <template #cell-actions="{ row }"><div class="flex flex-wrap gap-2"><button v-if="row.status === 'PENDING'" class="text-xs font-medium text-amber-600" @click="cancelTarget = row">取消</button><button v-if="canRefund(row)" class="text-xs font-medium text-purple-600" @click="openRefund(row)">申请退款</button><span v-if="row.status !== 'PENDING' && !canRefund(row)" class="text-xs text-gray-400">—</span></div></template>
      </DataTable>
      <AgentPagination v-if="hasOrders" :total="total" :page="page" :page-size="pageSize" item-label="笔订单" @update:page="changePage" @update:page-size="changePageSize" />
    </div>
  </main>

  <BaseDialog :show="cancelTarget !== null" title="取消订单" width="narrow" @close="cancelTarget = null"><p class="text-sm text-gray-600 dark:text-gray-300">确定取消订单 #{{ cancelTarget?.id }} 吗？</p><template #footer><button class="btn btn-secondary" @click="cancelTarget = null">返回</button><button class="btn btn-danger" :disabled="actionLoading" data-testid="confirm-cancel" @click="confirmCancel">确认取消</button></template></BaseDialog>
  <BaseDialog :show="refundTarget !== null" title="申请退款" @close="refundTarget = null"><div class="space-y-3"><p class="text-sm text-gray-500">订单 #{{ refundTarget?.id }} · {{ refundTarget ? money(refundTarget.amount) : '' }}</p><label class="block text-sm font-medium" for="refund-reason">退款原因</label><AgentTextArea id="refund-reason" v-model="refundReason" class="min-h-24" maxlength="500" placeholder="请填写退款原因（2–500 字）" /></div><template #footer><button class="btn btn-secondary" @click="refundTarget = null">返回</button><button class="btn btn-primary" :disabled="actionLoading || refundReason.trim().length < 2" data-testid="confirm-refund" @click="confirmRefund">提交申请</button></template></BaseDialog>
</template>
