<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { notifyAgentAccountFactsChanged } from '@/agent/accountFacts'
import { agentAPI, type AgentAffiliateDetail } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const detail = ref<AgentAffiliateDetail | null>(null)
const loading = ref(true)
const loadError = ref('')
const transferring = ref(false)
const transferError = ref('')
const success = ref('')
const confirmTransfer = ref(false)
const copied = ref<'code' | 'link' | ''>('')

const inviteLink = computed(() => {
  const code = detail.value?.aff_code || ''
  if (!code) return ''
  const suffix = `/register?aff=${encodeURIComponent(code)}`
  return typeof window === 'undefined' ? suffix : `${window.location.origin}${suffix}`
})

const rebateRate = computed(() => {
  const value = detail.value?.effective_rebate_rate_percent ?? 0
  const rounded = Math.round(value * 100) / 100
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
})

function money(value: number): string {
  return `$${Number.isFinite(value) ? value.toFixed(2) : '0.00'}`
}

function dateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

async function load(silent = false): Promise<void> {
  if (!silent) loading.value = true
  loadError.value = ''
  try {
    detail.value = await agentAPI.affiliate.get()
  } catch (error) {
    loadError.value = errorMessage(error, '推广返利信息加载失败，请稍后重试。')
  } finally {
    if (!silent) loading.value = false
  }
}

async function copy(value: string, type: 'code' | 'link'): Promise<void> {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    copied.value = type
    window.setTimeout(() => { if (copied.value === type) copied.value = '' }, 1600)
  } catch {
    copied.value = ''
  }
}

async function transfer(): Promise<void> {
  if (!detail.value || detail.value.aff_quota <= 0 || transferring.value) return
  confirmTransfer.value = false
  transferring.value = true
  transferError.value = ''
  success.value = ''
  try {
    const result = await agentAPI.affiliate.transfer()
    success.value = `已将 ${money(result.transferred_quota)} 返利转入主站余额，当前主站余额 ${money(result.balance)}。`
    notifyAgentAccountFactsChanged({ balanceCents: Math.round(result.balance * 100), refreshSubscriptions: false })
    await load(true)
  } catch (error) {
    transferError.value = errorMessage(error, '返利转入余额失败，请稍后重试。')
  } finally {
    transferring.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div><h2 class="text-2xl font-bold text-gray-900 dark:text-white">推广返利</h2><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">邀请关系、返利累计与余额结算均由 Sub2API 主站统一维护。</p></div>
      <button class="btn btn-secondary btn-sm self-start" type="button" :disabled="loading" @click="load()"><Icon name="refresh" size="sm" class="mr-1" :class="{ 'animate-spin': loading }" />刷新</button>
    </div>

    <div v-if="loading" class="card flex min-h-64 items-center justify-center"><Icon name="refresh" class="animate-spin text-primary-500" /><span class="ml-2 text-sm text-gray-500">正在读取主站返利数据…</span></div>
    <div v-else-if="loadError" class="card border-red-200 bg-red-50 p-6 text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300" role="alert"><p>{{ loadError }}</p><button class="btn btn-secondary btn-sm mt-4" type="button" @click="load()">重新加载</button></div>

    <template v-else-if="detail">
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <article class="card p-5"><p class="text-sm text-gray-500 dark:text-dark-400">当前返利比例</p><p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ rebateRate }}%</p></article>
        <article class="card p-5"><p class="text-sm text-gray-500 dark:text-dark-400">已邀请用户</p><p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ detail.aff_count.toLocaleString() }}</p></article>
        <article class="card p-5"><p class="text-sm text-gray-500 dark:text-dark-400">可转入余额</p><p class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ money(detail.aff_quota) }}</p></article>
        <article class="card p-5"><p class="text-sm text-gray-500 dark:text-dark-400">累计返利</p><p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ money(detail.aff_history_quota) }}</p><p v-if="detail.aff_frozen_quota > 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">其中冻结 {{ money(detail.aff_frozen_quota) }}</p></article>
      </div>

      <section class="card p-6">
        <div class="flex items-center gap-3"><div class="flex h-11 w-11 items-center justify-center rounded-xl bg-primary-100 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300"><Icon name="users" /></div><div><h3 class="font-semibold">分享邀请</h3><p class="text-sm text-gray-500 dark:text-dark-400">新用户通过本站邀请链接注册后，归属关系落库到主站。</p></div></div>
        <div class="mt-5 grid gap-4 lg:grid-cols-2">
          <div><p class="mb-2 text-sm font-medium">邀请码</p><div class="flex gap-2 rounded-xl border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900"><code class="min-w-0 flex-1 px-2 py-1 text-sm">{{ detail.aff_code }}</code><button class="btn btn-secondary btn-sm" type="button" @click="copy(detail.aff_code, 'code')"><Icon name="copy" size="sm" class="mr-1" />{{ copied === 'code' ? '已复制' : '复制' }}</button></div></div>
          <div><p class="mb-2 text-sm font-medium">本站邀请链接</p><div class="flex gap-2 rounded-xl border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900"><code class="min-w-0 flex-1 truncate px-2 py-1 text-sm" :title="inviteLink">{{ inviteLink }}</code><button class="btn btn-secondary btn-sm" type="button" @click="copy(inviteLink, 'link')"><Icon name="copy" size="sm" class="mr-1" />{{ copied === 'link' ? '已复制' : '复制' }}</button></div></div>
        </div>
        <div class="mt-5 rounded-xl border border-primary-200 bg-primary-50 p-4 text-sm text-primary-800 dark:border-primary-900/40 dark:bg-primary-900/20 dark:text-primary-200"><p class="font-medium">返利说明</p><ol class="mt-2 space-y-1"><li>1. 邀请用户必须通过本站注册链接注册。</li><li>2. 邀请用户完成符合主站规则的充值后，你可获得 {{ rebateRate }}% 返利。</li><li>3. 可用返利可一次性转入主站余额并用于模型调用。</li><li v-if="detail.aff_frozen_quota > 0">4. 冻结返利会按主站规则到期后自动转为可用。</li></ol></div>
      </section>

      <section class="card p-6">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h3 class="font-semibold">返利转入余额</h3><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">操作直接写入当前用户的主站账户，不经过代理站本地计费。</p></div><button class="btn btn-primary" type="button" :disabled="transferring || detail.aff_quota <= 0" @click="confirmTransfer = true"><Icon :name="transferring ? 'refresh' : 'dollar'" size="sm" class="mr-1" :class="{ 'animate-spin': transferring }" />{{ transferring ? '转入中…' : '全部转入余额' }}</button></div>
        <p v-if="detail.aff_quota <= 0" class="mt-3 text-sm text-amber-600 dark:text-amber-400">当前没有可转入的返利。</p>
        <p v-if="success" class="mt-4 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300" role="status">{{ success }}</p>
        <p v-if="transferError" class="mt-4 rounded-xl bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" role="alert">{{ transferError }}</p>
      </section>

      <section class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"><h3 class="font-semibold">邀请用户</h3></div>
        <div v-if="detail.invitees.length === 0" class="empty-state py-12"><Icon name="users" size="xl" class="mb-3 text-gray-400" /><p class="text-sm text-gray-500">暂无邀请记录</p></div>
        <div v-else class="overflow-x-auto"><table class="w-full min-w-[620px] text-left text-sm"><thead><tr class="border-b border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400"><th class="px-6 py-3 font-medium">邮箱</th><th class="px-4 py-3 font-medium">用户名</th><th class="px-4 py-3 text-right font-medium">累计返利</th><th class="px-6 py-3 font-medium">加入时间</th></tr></thead><tbody><tr v-for="(item, index) in detail.invitees" :key="`${item.email}-${item.created_at || index}`" class="border-b border-gray-100 last:border-0 dark:border-dark-800"><td class="px-6 py-4 font-medium">{{ item.email || '—' }}</td><td class="px-4 py-4 text-gray-600 dark:text-dark-300">{{ item.username || '—' }}</td><td class="px-4 py-4 text-right font-semibold text-emerald-600 dark:text-emerald-400">{{ money(item.total_rebate) }}</td><td class="px-6 py-4 text-gray-600 dark:text-dark-300">{{ dateTime(item.created_at) }}</td></tr></tbody></table></div>
      </section>
    </template>

    <BaseDialog :show="confirmTransfer" title="确认转入主站余额" width="narrow" @close="confirmTransfer = false"><p class="text-sm leading-6 text-gray-600 dark:text-dark-300">将当前全部可用返利 <strong>{{ money(detail?.aff_quota || 0) }}</strong> 转入当前主站账户余额。该操作由主站执行。</p><template #footer><button class="btn btn-secondary" type="button" @click="confirmTransfer = false">取消</button><button class="btn btn-primary" type="button" @click="transfer">确认转入</button></template></BaseDialog>
  </div>
</template>
