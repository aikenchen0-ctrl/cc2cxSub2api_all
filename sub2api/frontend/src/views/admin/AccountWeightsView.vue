<template>
  <AppLayout>
    <div class="flex h-full min-h-0 flex-col gap-4 p-4 md:p-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.accountWeights.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountWeights.description') }}</p>
        </div>
        <button class="btn btn-secondary" :disabled="loading" @click="loadAccounts">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('admin.accountWeights.refresh') }}
        </button>
      </div>

      <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
        <div v-for="item in summary" :key="item.label" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</div>
          <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
        <input v-model="search" class="input min-w-[220px] flex-1" :placeholder="t('admin.accountWeights.searchPlaceholder')" />
        <div class="relative flex items-center">
          <Icon name="sort" size="sm" class="pointer-events-none absolute left-3 text-gray-400" />
          <select v-model="sortKey" class="input w-56 pl-9">
          <option value="base">{{ t('admin.accountWeights.sort.base') }}</option>
          <option value="sticky">{{ t('admin.accountWeights.sort.sticky') }}</option>
          <option value="priority">{{ t('admin.accountWeights.sort.priority') }}</option>
          <option value="priority_factor">{{ t('admin.accountWeights.sort.priorityFactor') }}</option>
          <option value="load_factor">{{ t('admin.accountWeights.sort.loadFactor') }}</option>
          <option value="queue_factor">{{ t('admin.accountWeights.sort.queueFactor') }}</option>
          <option value="error_rate_factor">{{ t('admin.accountWeights.sort.errorRateFactor') }}</option>
          <option value="ttft_factor">{{ t('admin.accountWeights.sort.ttftFactor') }}</option>
          <option value="reset_factor">{{ t('admin.accountWeights.sort.resetFactor') }}</option>
          <option value="quota_headroom_factor">{{ t('admin.accountWeights.sort.quotaHeadroomFactor') }}</option>
          <option value="upstream_cost_factor">{{ t('admin.accountWeights.sort.upstreamCostFactor') }}</option>
          <option value="concurrency">{{ t('admin.accountWeights.sort.concurrency') }}</option>
          <option value="capacity">{{ t('admin.accountWeights.sort.capacity') }}</option>
          <option value="multiplier">{{ t('admin.accountWeights.sort.multiplier') }}</option>
          <option value="name">{{ t('admin.accountWeights.sort.name') }}</option>
          </select>
        </div>
        <button class="btn btn-secondary !px-2" :title="sortDescending ? t('admin.accountWeights.sortDescending') : t('admin.accountWeights.sortAscending')" :aria-label="sortDescending ? t('admin.accountWeights.sortDescending') : t('admin.accountWeights.sortAscending')" @click="sortDescending = !sortDescending">
          <Icon :name="sortDescending ? 'arrowDown' : 'arrowUp'" size="sm" />
        </button>
        <div class="relative flex items-center">
          <Icon name="filter" size="sm" class="pointer-events-none absolute left-3 text-gray-400" />
          <select v-model="statusFilter" class="input w-36 pl-9">
          <option value="">{{ t('admin.accountWeights.allStatuses') }}</option>
          <option value="active">{{ t('admin.accountWeights.status.active') }}</option>
          <option value="inactive">{{ t('admin.accountWeights.status.inactive') }}</option>
          <option value="error">{{ t('admin.accountWeights.status.error') }}</option>
          </select>
        </div>
      </div>

      <div class="min-h-0 flex-1 overflow-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <table class="min-w-[1050px] w-full text-left text-sm">
          <thead class="sticky top-0 z-10 bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900 dark:text-gray-400">
            <tr>
              <th v-for="column in tableColumns" :key="column" class="px-4 py-3">{{ t(`admin.accountWeights.columns.${column}`) }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="account in filteredAccounts" :key="account.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
              <td class="px-4 py-3"><a v-if="accountHomepageUrl(account)" :href="accountHomepageUrl(account)" target="_blank" rel="noopener noreferrer" class="font-medium text-primary-600 hover:underline dark:text-primary-400" :title="accountHomepageUrl(account)">{{ account.name }}</a><div v-else class="font-medium text-gray-900 dark:text-white" :title="t('admin.accountWeights.baseUrlMissing')">{{ account.name }}</div><div class="text-xs text-gray-400">#{{ account.id }}</div></td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ account.platform }} / {{ account.type }}</td>
              <td class="px-4 py-3 font-semibold text-primary-600 dark:text-primary-400">{{ score(account, 'base') }}</td>
              <td class="px-4 py-3">{{ score(account, 'sticky') }}</td>
              <td class="px-4 py-3">{{ integer(account.priority) }}</td>
              <td class="px-4 py-3">{{ factor(account, 'priority') }}</td>
              <td class="px-4 py-3">{{ integer(account.current_concurrency) }} / {{ integer(account.concurrency) }}</td>
              <td class="px-4 py-3">{{ factor(account, 'load') }}</td>
              <td v-for="key in factorKeys" :key="key" class="px-4 py-3">{{ factor(account, key) }}</td>
              <td class="px-4 py-3">{{ number(account.rate_multiplier, 2) }}</td>
              <td class="px-4 py-3"><span :class="statusClass(account.status)">{{ statusLabel(account.status) }}</span></td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ groups(account) }}</td>
            </tr>
            <tr v-if="!loading && filteredAccounts.length === 0"><td colspan="17" class="px-4 py-12 text-center text-gray-500">{{ t('admin.accountWeights.empty') }}</td></tr>
          </tbody>
        </table>
      </div>
      <p class="text-xs text-gray-400">{{ t('admin.accountWeights.factorNote') }}</p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import accountsApi from '@/api/admin/accounts'
import type { Account } from '@/types'
import { sanitizeUrl } from '@/utils/url'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const accounts = ref<Account[]>([])
const loading = ref(false)
const search = ref('')
const statusFilter = ref('')
const sortKey = ref('base')
const sortDescending = ref(true)
const factorKeys = ['queue', 'error_rate', 'ttft', 'reset', 'quota_headroom', 'upstream_cost']
const tableColumns = ['account', 'platformType', 'baseScore', 'stickyScore', 'priorityRaw', 'priorityFactor', 'concurrencyCapacity', 'loadFactor', 'queueFactor', 'errorRateFactor', 'ttftFactor', 'resetFactor', 'quotaHeadroomFactor', 'upstreamCostFactor', 'multiplier', 'status', 'groups']

async function loadAccounts() {
  loading.value = true
  try {
    const pageSize = 500
    const filters = { include_scheduler_score: '1', sort_by: 'name', sort_order: 'asc' as const }
    const first = await accountsApi.list(1, pageSize, filters)
    const items = [...first.items]
    for (let page = 2; page <= first.pages; page += 1) {
      const result = await accountsApi.list(page, pageSize, filters)
      items.push(...result.items)
    }
    accounts.value = items
  } finally { loading.value = false }
}
function base(a: Account) { return a.scheduler_score?.base_score ?? a.scheduler_scores?.[0]?.base_score ?? null }
function sticky(a: Account) { return a.scheduler_score?.sticky_score ?? a.scheduler_scores?.[0]?.sticky_score ?? null }
function score(a: Account, kind: 'base' | 'sticky') { const v = kind === 'base' ? base(a) : sticky(a); return v == null ? '-' : (a.scheduler_score?.sticky_score_infinity && kind === 'sticky' ? '+∞' : v.toFixed(2)) }
function number(v: number | null | undefined, digits = 2) { return v == null ? '-' : Number(v).toFixed(digits) }
function integer(v: number | null | undefined) { return v == null ? '-' : String(v) }
function factor(a: Account, key: string) { const value = a.scheduler_score?.factors?.[key]; return value == null ? '—' : value.toFixed(3) }
function groups(a: Account) { return a.scheduler_scores?.map(g => g.group_name || t('admin.accountWeights.groupFallback', { id: g.group_id ?? '-' })).join(', ') || a.groups?.map(g => g.name).join(', ') || '-' }
function accountHomepageUrl(a: Account) { const raw = (a as any).credentials?.base_url; if (typeof raw !== 'string') return ''; return sanitizeUrl(raw) }
const factorSortKeys: Record<string, string> = { priority_factor: 'priority', load_factor: 'load', queue_factor: 'queue', error_rate_factor: 'error_rate', ttft_factor: 'ttft', reset_factor: 'reset', quota_headroom_factor: 'quota_headroom', upstream_cost_factor: 'upstream_cost' }
function sortValue(x: Account) { const factorKey = factorSortKeys[sortKey.value]; if (factorKey) return x.scheduler_score?.factors?.[factorKey] ?? -Infinity; return sortKey.value === 'base' ? (base(x) ?? -Infinity) : sortKey.value === 'sticky' ? (sticky(x) ?? -Infinity) : sortKey.value === 'priority' ? x.priority : sortKey.value === 'concurrency' ? (x.current_concurrency ?? 0) : sortKey.value === 'capacity' ? x.concurrency : x.rate_multiplier ?? 0 }
const filteredAccounts = computed(() => accounts.value.filter(a => (!statusFilter.value || a.status === statusFilter.value) && (!search.value || `${a.name} ${a.platform} ${a.type}`.toLowerCase().includes(search.value.toLowerCase()))).sort((a,b) => { const direction = sortDescending.value ? -1 : 1; return sortKey.value === 'name' ? direction * a.name.localeCompare(b.name) : direction * (sortValue(a) - sortValue(b)) }))
const summary = computed(() => [{ label: t('admin.accountWeights.summary.total'), value: accounts.value.length }, { label: t('admin.accountWeights.summary.scored'), value: accounts.value.filter(a => base(a) != null).length }, { label: t('admin.accountWeights.summary.schedulable'), value: accounts.value.filter(a => a.schedulable).length }, { label: t('admin.accountWeights.summary.highestBase'), value: base(accounts.value.slice().sort((a,b) => (base(b) ?? -Infinity) - (base(a) ?? -Infinity))[0] || ({} as Account)) == null ? '-' : number(base(accounts.value.slice().sort((a,b) => (base(b) ?? -Infinity) - (base(a) ?? -Infinity))[0] || ({} as Account))) }])
function statusLabel(s: Account['status']) { return s === 'active' ? t('admin.accountWeights.status.active') : s === 'inactive' ? t('admin.accountWeights.status.inactive') : t('admin.accountWeights.status.error') }
function statusClass(s: Account['status']) { return s === 'active' ? 'text-emerald-600' : s === 'error' ? 'text-red-600' : 'text-gray-500' }
onMounted(loadAccounts)
</script>
