<script setup lang="ts">
import DataTable from '@/components/common/DataTable.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import { onMounted, ref } from 'vue'
import { agentAPI, isAuthoritativeAgentUsageSource, type AgentUsageView as UsageItem } from '@/agent/api'
import AgentPagination from '@/components/AgentPagination.vue'
import AgentUsageInsights from '@/components/AgentUsageInsights.vue'
import { enumLabel, statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'

const items = ref<UsageItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(true)
const error = ref('')
const model = ref('')
const requestID = ref('')
const startTime = ref('')
const endTime = ref('')
const appliedFilters = ref<{ model?: string; request_id?: string; start_time?: string; end_time?: string }>({})
let loadSequence = 0

async function applyFilters(): Promise<void> {
  if ((startTime.value && !Number.isFinite(Date.parse(startTime.value))) || (endTime.value && !Number.isFinite(Date.parse(endTime.value)))) {
    error.value = '请输入有效时间。'
    return
  }
  if (startTime.value && endTime.value && Date.parse(startTime.value) >= Date.parse(endTime.value)) {
    error.value = '开始时间必须早于结束时间。'
    return
  }
  appliedFilters.value = {
    ...(model.value.trim() ? { model: model.value.trim() } : {}),
    ...(requestID.value.trim() ? { request_id: requestID.value.trim() } : {}),
    ...(startTime.value ? { start_time: new Date(startTime.value).toISOString() } : {}),
    ...(endTime.value ? { end_time: new Date(endTime.value).toISOString() } : {}),
  }
  page.value = 1
  await load()
}

async function resetFilters(): Promise<void> {
  model.value = requestID.value = startTime.value = endTime.value = ''
  await applyFilters()
}

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

function usd(nanos: number): string {
  const [whole, fraction = ''] = (nanos / 1_000_000_000).toFixed(9).split('.')
  return `$${whole}.${fraction.replace(/0+$/, '').padEnd(2, '0')}`
}

function usageSource(source?: string): string {
  if (source === 'user_balance_delta_fallback') return '账户余额差额估算'
  if (isAuthoritativeAgentUsageSource(source)) return '已确认用量记录'
  if (source === 'owner_balance_delta_fallback') return '旧版主账户余额差额估算'
  return '暂无单次请求用量详情'
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  try {
    const result = Object.keys(appliedFilters.value).length
      ? await agentAPI.getUsage(page.value, pageSize.value, { ...appliedFilters.value })
      : await agentAPI.getUsage(page.value, pageSize.value)
    if (sequence !== loadSequence) return
    total.value = result.total
    const lastPage = Math.max(1, Math.ceil(result.total / pageSize.value))
    if (page.value > lastPage) {
      page.value = lastPage
      await load()
      return
    }
    items.value = result.items
  } catch (err) {
    if (sequence !== loadSequence) return
    error.value = errorMessage(err, '加载用量记录失败，请稍后刷新重试。')
    items.value = []
    total.value = 0
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function changePage(nextPage: number): Promise<void> {
  if (nextPage < 1 || nextPage > Math.max(1, Math.ceil(total.value / pageSize.value)) || nextPage === page.value) return
  page.value = nextPage
  await load()
}

async function changePageSize(nextPageSize: number): Promise<void> {
  if (nextPageSize === pageSize.value) return
  pageSize.value = nextPageSize
  page.value = 1
  await load()
}

onMounted(load)
</script>

<template>
  <main class="space-y-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">用量记录</h1>
        <p class="mt-1 text-sm text-slate-500">查看通过本站发起的请求及已同步用量记录。</p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">刷新</button>
    </header>
    <AgentUsageInsights />
    <form class="card p-6" @submit.prevent="applyFilters">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div class="flex flex-1 flex-wrap items-end gap-4">
          <div class="w-full sm:w-auto sm:min-w-[220px]"><label for="usage-model" class="input-label">模型（精确匹配）</label><AgentInput id="usage-model" v-model="model" maxlength="256" /></div>
          <div class="w-full sm:w-auto sm:min-w-[220px]"><label for="usage-request" class="input-label">请求编号（精确匹配）</label><AgentInput id="usage-request" v-model="requestID" maxlength="256" /></div>
          <div class="w-full sm:w-auto"><label for="usage-start" class="input-label">开始时间</label><AgentInput id="usage-start" v-model="startTime" type="datetime-local" /></div>
          <div class="w-full sm:w-auto"><label for="usage-end" class="input-label">结束时间（不包含）</label><AgentInput id="usage-end" v-model="endTime" type="datetime-local" /></div>
        </div>
        <div class="flex gap-3"><button class="btn btn-primary" type="submit">应用筛选</button><button class="btn btn-secondary" type="button" @click="resetFilters">重置</button></div>
      </div>
      <p class="input-hint mt-3">时间使用浏览器本地时区；筛选作用于下方全量本站记录和分页总数，不改变上方独立时间窗口的统计。</p>
    </form>
    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <div class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <DataTable :columns='[{"key":"request_id","label":"请求"},{"key":"model","label":"模型 / 路由"},{"key":"usage","label":"用量"},{"key":"actual_cents","label":"实际结算"},{"key":"settlement_status","label":"状态"},{"key":"created_at","label":"创建时间"}]' :data="items" :loading="loading" row-key="request_id">
        <template #cell-request_id="{ row: item }"><div class="max-w-xl whitespace-normal"><div class="truncate font-mono text-xs">{{ item.request_id }}</div><div v-if="item.usage_id" class="mt-1 truncate text-[11px] text-slate-400">用量编号 {{ item.usage_id }}</div></div></template>
        <template #cell-model="{ row: item }"><div class="max-w-xl whitespace-normal"><div>{{ item.model || '未知模型' }}</div><div v-if="item.inbound_endpoint" class="mt-1 text-xs text-slate-500">{{ item.inbound_endpoint }}</div><div v-if="item.upstream_model || item.upstream_response_model" class="mt-1 text-xs text-slate-500">实际模型 {{ item.upstream_model || '—' }}<span v-if="item.upstream_response_model"> · 响应模型 {{ item.upstream_response_model }}</span></div><div v-if="item.upstream_model_mismatch === true" class="mt-1 text-xs text-amber-600">实际模型与请求模型不一致</div></div></template>
        <template #cell-usage="{ row: item }"><div class="max-w-xl whitespace-normal">
                <div :class="item.usage_source === 'owner_balance_delta_fallback' ? 'text-amber-600' : 'text-slate-500'">{{ usageSource(item.usage_source) }}</div>
                <template v-if="isAuthoritativeAgentUsageSource(item.usage_source)">
                  <div class="mt-1 text-slate-700 dark:text-slate-300">令牌 · 输入 {{ item.input_tokens }} / 输出 {{ item.output_tokens }} / 缓存读取 {{ item.cache_read_tokens }} / 缓存写入 {{ item.cache_creation_tokens }}（5 分钟 {{ item.cache_creation_5m_tokens }} / 1 小时 {{ item.cache_creation_1h_tokens }}）</div>
                  <div v-if="item.image_count || item.image_input_tokens || item.image_output_tokens" class="mt-1 text-slate-700 dark:text-slate-300">图片 {{ item.image_count }} 张 · 输入 {{ item.image_input_tokens }} / 输出 {{ item.image_output_tokens }} · 费用 {{ usd(item.image_input_cost_usd_nanos) }} / {{ usd(item.image_output_cost_usd_nanos) }}</div>
                  <div v-if="item.image_size || item.image_input_size || item.image_output_size || item.media_type" class="mt-1 text-slate-500">{{ item.media_type || '媒体' }}<span v-if="item.image_size"> · 尺寸 {{ item.image_size }}</span><span v-if="item.image_input_size"> · 输入尺寸 {{ item.image_input_size }}</span><span v-if="item.image_output_size"> · 输出尺寸 {{ item.image_output_size }}</span></div>
                  <div class="mt-1 text-slate-700 dark:text-slate-300"><span v-if="item.actual_cost_reported">实际费用 {{ usd(item.actual_cost_usd_nanos) }}</span><span v-else>实际费用待确认</span> · 标准费用 {{ usd(item.total_cost_usd_nanos) }}</div>
                  <div class="mt-1 text-slate-500">输入费用 {{ usd(item.input_cost_usd_nanos) }} · 输出费用 {{ usd(item.output_cost_usd_nanos) }} · 缓存费用 {{ usd(item.cache_read_cost_usd_nanos + item.cache_creation_cost_usd_nanos) }}</div>
                  <div v-if="item.reasoning_effort || item.service_tier || item.request_type || item.billing_mode || item.duration_ms || item.first_token_ms || item.rate_multiplier || item.long_context_billing_applied || item.openai_ws_mode || item.native_compaction_v2 || item.cache_ttl_overridden" class="mt-1 text-slate-500"><span v-if="item.reasoning_effort">推理强度 {{ enumLabel(item.reasoning_effort) }} · </span><span v-if="item.service_tier">服务等级 {{ enumLabel(item.service_tier) }} · </span><span v-if="item.request_type">请求类型 {{ enumLabel(item.request_type) }} · </span><span v-if="item.billing_mode">计费方式 {{ enumLabel(item.billing_mode) }} · </span><span v-if="item.duration_ms">耗时 {{ item.duration_ms }} 毫秒 · </span><span v-if="item.first_token_ms">首字延迟 {{ item.first_token_ms }} 毫秒 · </span><span v-if="item.rate_multiplier">倍率 ×{{ item.rate_multiplier }} · </span><span v-if="item.long_context_billing_applied">长上下文计费 · </span><span v-if="item.openai_ws_mode">兼容接口长连接模式 · </span><span v-if="item.native_compaction_v2">原生压缩 v2 · </span><span v-if="item.cache_ttl_overridden">缓存有效期已覆盖</span></div>
                </template>
              </div></template>
        <template #cell-actual_cents="{ row: item }"><div class="max-w-xl whitespace-normal"><div>{{ money(item.actual_cents) }}</div><div class="mt-1 text-xs text-slate-500">请求前校验 {{ money(item.reserved_cents) }}</div></div></template>
        <template #cell-settlement_status="{ row: item }"><div class="max-w-xl whitespace-normal"><AgentStatusBadge :status="item.settlement_status" :label="statusLabel(item.settlement_status)" /></div></template>
        <template #cell-created_at="{ row: item }"><div class="max-w-xl whitespace-normal">{{ new Date(item.created_at).toLocaleString('zh-CN') }}</div></template>
        <template #empty>暂无用量记录。</template>
      </DataTable>
    </div>
    <AgentPagination
      v-if="!loading && total > 0"
      :total="total"
      :page="page"
      :page-size="pageSize"
      item-label="条用量记录"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </main>
</template>
