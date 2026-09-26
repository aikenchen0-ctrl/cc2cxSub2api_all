<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, isAuthoritativeAgentUsageSource, type AgentUsageView as UsageItem } from '@/agent/api'
import AgentPagination from '@/components/AgentPagination.vue'
import { enumLabel, statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'

const items = ref<UsageItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(true)
const error = ref('')
let loadSequence = 0

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

function usd(nanos: number): string {
  const [whole, fraction = ''] = (nanos / 1_000_000_000).toFixed(9).split('.')
  return `$${whole}.${fraction.replace(/0+$/, '').padEnd(2, '0')}`
}

function usageSource(source?: string): string {
  if (isAuthoritativeAgentUsageSource(source)) return 'Sub2API 用量记录'
  if (source === 'owner_balance_delta_fallback') return '主账户余额差额估算'
  return '暂无单次请求用量详情'
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.getUsage(page.value, pageSize.value)
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
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">用量记录</h1>
        <p class="mt-1 text-sm text-slate-500">如能匹配到 Sub2API 用量记录，此处会显示令牌数量和费用明细。</p>
      </div>
      <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">刷新</button>
    </header>
    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <div class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div v-if="loading" class="p-5 text-sm text-slate-500">正在加载…</div>
      <div v-else-if="items.length === 0" class="p-5 text-sm text-slate-500">暂无用量记录。</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">请求</th><th class="px-5 py-3">模型 / 路由</th><th class="px-5 py-3">Sub2API 用量</th><th class="px-5 py-3">本地钱包</th><th class="px-5 py-3">状态</th><th class="px-5 py-3">创建时间</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.request_id" class="border-t dark:border-slate-700">
              <td class="max-w-xs px-5 py-4"><div class="truncate font-mono text-xs">{{ item.request_id }}</div><div v-if="item.usage_id" class="mt-1 truncate text-[11px] text-slate-400">用量编号 {{ item.usage_id }}</div></td>
              <td class="px-5 py-4"><div>{{ item.model || '未知模型' }}</div><div v-if="item.inbound_endpoint" class="mt-1 text-xs text-slate-500">{{ item.inbound_endpoint }}</div><div v-if="item.upstream_model || item.upstream_response_model" class="mt-1 text-xs text-slate-500">上游模型 {{ item.upstream_model || '—' }}<span v-if="item.upstream_response_model"> · 响应模型 {{ item.upstream_response_model }}</span></div><div v-if="item.upstream_model_mismatch === true" class="mt-1 text-xs text-amber-600">上游模型与请求的模型不一致</div></td>
              <td class="min-w-72 px-5 py-4 text-xs">
                <div :class="item.usage_source === 'owner_balance_delta_fallback' ? 'text-amber-600' : 'text-slate-500'">{{ usageSource(item.usage_source) }}</div>
                <template v-if="isAuthoritativeAgentUsageSource(item.usage_source)">
                  <div class="mt-1 text-slate-700 dark:text-slate-300">令牌 · 输入 {{ item.input_tokens }} / 输出 {{ item.output_tokens }} / 缓存读取 {{ item.cache_read_tokens }} / 缓存写入 {{ item.cache_creation_tokens }}（5 分钟 {{ item.cache_creation_5m_tokens }} / 1 小时 {{ item.cache_creation_1h_tokens }}）</div>
                  <div v-if="item.image_count || item.image_input_tokens || item.image_output_tokens" class="mt-1 text-slate-700 dark:text-slate-300">图片 {{ item.image_count }} 张 · 输入 {{ item.image_input_tokens }} / 输出 {{ item.image_output_tokens }} · 费用 {{ usd(item.image_input_cost_usd_nanos) }} / {{ usd(item.image_output_cost_usd_nanos) }}</div>
                  <div v-if="item.image_size || item.image_input_size || item.image_output_size || item.media_type" class="mt-1 text-slate-500">{{ item.media_type || '媒体' }}<span v-if="item.image_size"> · 尺寸 {{ item.image_size }}</span><span v-if="item.image_input_size"> · 输入尺寸 {{ item.image_input_size }}</span><span v-if="item.image_output_size"> · 输出尺寸 {{ item.image_output_size }}</span></div>
                  <div class="mt-1 text-slate-700 dark:text-slate-300">{{ item.actual_cost_reported ? '实际费用' : '暂无实际费用，按总费用结算' }} {{ usd(item.actual_cost_reported ? item.actual_cost_usd_nanos : item.total_cost_usd_nanos) }} · 标准费用 {{ usd(item.total_cost_usd_nanos) }}</div>
                  <div class="mt-1 text-slate-500">输入费用 {{ usd(item.input_cost_usd_nanos) }} · 输出费用 {{ usd(item.output_cost_usd_nanos) }} · 缓存费用 {{ usd(item.cache_read_cost_usd_nanos + item.cache_creation_cost_usd_nanos) }}</div>
                  <div v-if="item.reasoning_effort || item.service_tier || item.request_type || item.billing_mode || item.duration_ms || item.first_token_ms || item.rate_multiplier || item.long_context_billing_applied || item.openai_ws_mode || item.native_compaction_v2 || item.cache_ttl_overridden" class="mt-1 text-slate-500"><span v-if="item.reasoning_effort">推理强度 {{ enumLabel(item.reasoning_effort) }} · </span><span v-if="item.service_tier">服务等级 {{ enumLabel(item.service_tier) }} · </span><span v-if="item.request_type">请求类型 {{ enumLabel(item.request_type) }} · </span><span v-if="item.billing_mode">计费方式 {{ enumLabel(item.billing_mode) }} · </span><span v-if="item.duration_ms">耗时 {{ item.duration_ms }} 毫秒 · </span><span v-if="item.first_token_ms">首字延迟 {{ item.first_token_ms }} 毫秒 · </span><span v-if="item.rate_multiplier">倍率 ×{{ item.rate_multiplier }} · </span><span v-if="item.long_context_billing_applied">长上下文计费 · </span><span v-if="item.openai_ws_mode">兼容接口长连接模式 · </span><span v-if="item.native_compaction_v2">原生压缩 v2 · </span><span v-if="item.cache_ttl_overridden">缓存有效期已覆盖</span></div>
                </template>
              </td>
              <td class="px-5 py-4"><div>{{ money(item.actual_cents) }}</div><div class="mt-1 text-xs text-slate-500">预留 {{ money(item.reserved_cents) }}</div></td>
              <td class="px-5 py-4"><span :class="item.settlement_status === 'confirmed' ? 'text-emerald-600' : item.settlement_status === 'pending' ? 'text-amber-600' : 'text-slate-500'">{{ statusLabel(item.settlement_status) }}</span></td>
              <td class="px-5 py-4 text-slate-500">{{ new Date(item.created_at).toLocaleString('zh-CN') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
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
