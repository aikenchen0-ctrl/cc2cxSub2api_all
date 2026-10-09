<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentAffiliateDetail, type AgentAffiliateInvite, type AgentAffiliateRebate, type AgentAffiliateTransferRecord } from '@/agent/api'
import { errorMessage } from '@/agent/locale'

const detail = ref<AgentAffiliateDetail | null>(null)
const invites = ref<AgentAffiliateInvite[]>([])
const rebates = ref<AgentAffiliateRebate[]>([])
const transfers = ref<AgentAffiliateTransferRecord[]>([])
const bindCode = ref('')
const tab = ref<'invites' | 'rebates' | 'transfers'>('invites')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

function money(value: number): string {
  return `$${value.toFixed(2)}`
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [nextDetail, nextInvites, nextRebates, nextTransfers] = await Promise.all([
      agentAPI.affiliate.get(),
      agentAPI.affiliate.invites(),
      agentAPI.affiliate.rebates(),
      agentAPI.affiliate.transfers(),
    ])
    detail.value = nextDetail
    invites.value = nextInvites.items || []
    rebates.value = nextRebates.items || []
    transfers.value = nextTransfers.items || []
  } catch (cause) {
    detail.value = null
    error.value = errorMessage(cause, '推广返利加载失败。主站明细未返回时不会显示为 0。')
  } finally {
    loading.value = false
  }
}

async function transfer(): Promise<void> {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await agentAPI.affiliate.transfer()
    notice.value = `已划转 ${money(result.transferred_quota)} 到主站余额。`
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '划转失败，主站余额未被本地估算改写。')
  } finally {
    saving.value = false
  }
}

async function bind(): Promise<void> {
  if (bindCode.value.trim().length < 4) return
  saving.value = true
  error.value = ''
  try {
    await agentAPI.affiliate.bind(bindCode.value.trim())
    notice.value = '邀请码已绑定到当前账户。'
    bindCode.value = ''
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '绑定邀请码失败。')
  } finally {
    saving.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="space-y-4">
    <header class="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-300">AFFILIATE</p>
        <h1 class="mt-1 text-2xl font-bold">推广返利</h1>
        <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">邀请码、返利和划转都以当前登录用户的主站记录为准。</p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">刷新</button>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ notice }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取主站返利…</section>
    <template v-else-if="detail">
      <section class="grid gap-4 sm:grid-cols-3">
        <div class="card p-4"><p class="text-xs text-gray-500">邀请码</p><p class="mt-1 font-mono text-lg">{{ detail.aff_code }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">可划转额度</p><p class="mt-1 text-lg font-semibold">{{ money(detail.aff_quota) }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">返利比例</p><p class="mt-1 text-lg font-semibold">{{ detail.effective_rebate_rate_percent }}%</p></div>
      </section>
      <section class="card flex flex-col gap-3 p-4 sm:flex-row">
        <button class="btn btn-primary" type="button" :disabled="saving || detail.aff_quota <= 0" @click="transfer">划转到余额</button>
        <input v-model="bindCode" class="input sm:max-w-xs" maxlength="32" placeholder="绑定邀请码" aria-label="邀请码">
        <button class="btn btn-secondary" type="button" :disabled="saving || bindCode.trim().length < 4" @click="bind">绑定</button>
      </section>
      <div class="flex gap-2">
        <button class="btn btn-secondary" :class="{ 'btn-primary': tab === 'invites' }" type="button" @click="tab = 'invites'">邀请记录</button>
        <button class="btn btn-secondary" :class="{ 'btn-primary': tab === 'rebates' }" type="button" @click="tab = 'rebates'">返利记录</button>
        <button class="btn btn-secondary" :class="{ 'btn-primary': tab === 'transfers' }" type="button" @click="tab = 'transfers'">划转记录</button>
      </div>
      <section class="card overflow-x-auto p-4">
        <table v-if="tab === 'invites'" class="w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">用户</th><th>返利</th><th>时间</th></tr></thead>
          <tbody><tr v-for="item in invites" :key="item.email + item.created_at"><td class="py-2">{{ item.username || item.email }}</td><td>{{ money(item.total_rebate) }}</td><td>{{ item.created_at || '—' }}</td></tr><tr v-if="!invites.length"><td colspan="3" class="py-8 text-center text-gray-500">暂无邀请记录</td></tr></tbody>
        </table>
        <table v-else-if="tab === 'rebates'" class="w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">订单号</th><th>返利</th><th>状态</th></tr></thead>
          <tbody><tr v-for="item in rebates" :key="item.out_trade_no"><td class="py-2 font-mono text-xs">{{ item.out_trade_no }}</td><td>{{ money(item.rebate_amount) }}</td><td>{{ item.order_status }}</td></tr><tr v-if="!rebates.length"><td colspan="3" class="py-8 text-center text-gray-500">暂无返利记录</td></tr></tbody>
        </table>
        <table v-else class="w-full text-left text-sm">
          <thead><tr class="text-xs text-gray-400"><th class="py-2">金额</th><th>划转后余额</th><th>时间</th></tr></thead>
          <tbody><tr v-for="(item, index) in transfers" :key="index"><td class="py-2">{{ money(item.amount) }}</td><td>{{ item.balance_after == null ? '—' : money(item.balance_after) }}</td><td>{{ item.created_at || '—' }}</td></tr><tr v-if="!transfers.length"><td colspan="3" class="py-8 text-center text-gray-500">暂无划转记录</td></tr></tbody>
        </table>
      </section>
    </template>
  </main>
</template>
