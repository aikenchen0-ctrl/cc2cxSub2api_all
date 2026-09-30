<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <section class="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="relative px-6 py-7 sm:px-8">
          <div class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(59,130,246,0.14),transparent_38%),radial-gradient(circle_at_bottom_left,rgba(99,102,241,0.10),transparent_42%)]" />
          <div class="relative flex flex-col gap-6 lg:flex-row lg:items-center lg:justify-between">
            <div class="max-w-3xl">
              <div class="mb-3 inline-flex items-center gap-2 rounded-full border border-primary-200 bg-primary-50 px-3 py-1 text-xs font-semibold tracking-wide text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300"><Icon name="shield" size="sm" />{{ t('admin.upstreamAudit.eyebrow') }}</div>
              <h1 class="text-2xl font-bold tracking-tight text-gray-950 dark:text-white sm:text-3xl">{{ t('admin.upstreamAudit.heroTitle') }}</h1>
              <p class="mt-3 max-w-2xl text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('admin.upstreamAudit.heroDescription') }}</p>
            </div>
            <div class="flex flex-col items-stretch gap-2 sm:flex-row lg:flex-col">
              <button type="button" class="btn btn-primary min-w-44" :disabled="starting || running" @click="startAudit"><Icon name="play" size="sm" class="mr-2" />{{ running ? t('admin.upstreamAudit.running') : t('admin.upstreamAudit.startScan') }}</button>
              <button type="button" class="btn btn-secondary min-w-44" :disabled="loading" @click="refresh"><Icon name="refresh" size="sm" class="mr-2" :class="loading && 'animate-spin'" />{{ t('admin.upstreamAudit.refreshAccounts') }}</button>
              <span v-if="error" class="max-w-64 text-center text-xs text-red-600 dark:text-red-400">{{ error }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div v-for="metric in metrics" :key="metric.label" class="card p-4"><div class="flex items-center gap-3"><div class="rounded-xl p-2.5" :class="metric.iconClass"><Icon :name="metric.icon" size="md" /></div><div class="min-w-0"><p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p><p class="mt-1 truncate text-xl font-bold text-gray-950 dark:text-white">{{ metric.value }}</p></div></div></div>
      </section>

      <section v-if="job" class="card px-6 py-5">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h2 class="font-semibold text-gray-950 dark:text-white">{{ t('admin.upstreamAudit.progress.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.upstreamAudit.progress.accounts', { done: job.scanned_accounts, total: job.total_accounts }) }} · {{ t('admin.upstreamAudit.progress.models', { done: job.completed, total: job.matched_models }) }}</p></div><span class="rounded-full px-3 py-1 text-xs font-semibold" :class="running ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'">{{ running ? t('admin.upstreamAudit.progress.running') : t('admin.upstreamAudit.progress.completed') }}</span></div>
        <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"><div class="h-full rounded-full bg-primary-600 transition-all" :style="{ width: `${progress}%` }" /></div>
      </section>

      <section class="card overflow-hidden">
        <div class="border-b border-gray-100 px-4 pt-3 dark:border-dark-700 sm:px-6"><nav class="flex gap-1 overflow-x-auto"><button v-for="tab in tabs" :key="tab.key" type="button" class="whitespace-nowrap border-b-2 px-4 py-3 text-sm font-medium" :class="activeTab === tab.key ? 'border-primary-600 text-primary-700 dark:border-primary-400 dark:text-primary-300' : 'border-transparent text-gray-500 dark:text-gray-400'" @click="activeTab = tab.key">{{ tab.label }}</button></nav></div>

        <div v-if="activeTab === 'queue'">
          <div class="border-b border-gray-100 px-6 py-5 dark:border-dark-700"><h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('admin.upstreamAudit.queue.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.upstreamAudit.queue.description') }}</p></div>
          <div v-if="items.length" class="divide-y divide-gray-100 dark:divide-dark-700">
            <div class="hidden grid-cols-[1.35fr_1.2fr_1.2fr_.75fr_.75fr_.6fr] gap-4 bg-gray-50 px-6 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900/50 dark:text-gray-400 md:grid"><span>{{ t('admin.upstreamAudit.queue.columns.account') }}</span><span>{{ t('admin.upstreamAudit.queue.columns.declared') }}</span><span>{{ t('admin.upstreamAudit.queue.columns.prediction') }}</span><span>{{ t('admin.upstreamAudit.queue.columns.similarity') }}</span><span>{{ t('admin.upstreamAudit.queue.columns.status') }}</span><span>{{ t('admin.upstreamAudit.queue.columns.action') }}</span></div>
            <div v-for="item in items" :key="`${item.account_id}-${item.declared_model}`" class="grid gap-3 px-6 py-4 text-sm md:grid-cols-[1.35fr_1.2fr_1.2fr_.75fr_.75fr_.6fr] md:items-center md:gap-4"><div><p class="font-medium text-gray-900 dark:text-white">{{ item.account_name }}</p><p class="mt-0.5 text-xs text-gray-400">#{{ item.account_id }} · {{ item.platform }}</p></div><code class="break-all text-xs text-gray-700 dark:text-gray-200">{{ item.declared_model }}</code><code class="break-all text-xs text-gray-700 dark:text-gray-200">{{ item.prediction || '—' }}</code><span class="font-medium text-gray-900 dark:text-white">{{ percent(item.declared_similarity) }}</span><span class="w-fit rounded-full px-2.5 py-1 text-xs font-semibold" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span><span class="text-xs text-gray-500 dark:text-gray-400">{{ item.used_outputs || 0 }}/{{ item.attempts || 0 }}</span><p v-if="item.error" class="text-xs text-red-600 dark:text-red-400 md:col-span-6">{{ item.error }}</p></div>
          </div>
          <div v-else class="flex flex-col items-center px-6 py-14 text-center"><div class="flex h-14 w-14 items-center justify-center rounded-2xl bg-gray-100 text-gray-400 dark:bg-dark-700"><Icon name="inbox" size="xl" /></div><h3 class="mt-4 font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamAudit.queue.emptyTitle') }}</h3><p class="mt-2 max-w-xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('admin.upstreamAudit.queue.emptyDescription') }}</p></div>
          <div v-if="job?.account_errors?.length" class="border-t border-amber-200 bg-amber-50 px-6 py-4 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-900/20 dark:text-amber-200"><p class="font-semibold">{{ t('admin.upstreamAudit.queue.scanErrors', { count: job.account_errors.length }) }}</p><p class="mt-1 text-xs">{{ job.account_errors.slice(0, 5).map(entry => `${entry.account_name}: ${entry.error}`).join(' · ') }}</p></div>
        </div>

        <div v-else-if="activeTab === 'library'" class="p-6"><div class="mb-5 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between"><div><h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('admin.upstreamAudit.library.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.upstreamAudit.library.description') }}</p></div><code class="text-xs text-gray-400">{{ bankHash }}</code></div><div class="grid gap-5 lg:grid-cols-2"><div v-for="group in modelGroups" :key="group.family" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><div class="mb-3 flex items-center justify-between"><h3 class="font-semibold text-gray-900 dark:text-white">{{ group.label }}</h3><span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-600 dark:bg-dark-700">{{ group.models.length }} {{ t('admin.upstreamAudit.library.models') }}</span></div><div class="flex flex-wrap gap-2"><code v-for="model in group.models" :key="model" class="rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200">{{ model }}</code></div></div></div><div class="mt-5 flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"><Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0" /><span>{{ t('admin.upstreamAudit.library.calibration') }}</span></div></div>

        <div v-else class="p-6"><div class="mb-5"><h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('admin.upstreamAudit.settings.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.upstreamAudit.settings.description') }}</p></div><div class="grid gap-4 md:grid-cols-2"><div v-for="entry in policyItems" :key="entry.title" class="rounded-xl border border-gray-200 p-5 dark:border-dark-700"><div class="flex items-start gap-3"><div class="rounded-lg bg-primary-50 p-2 text-primary-600 dark:bg-primary-900/20"><Icon :name="entry.icon" size="sm" /></div><div><h3 class="font-semibold text-gray-900 dark:text-white">{{ entry.title }}</h3><p class="mt-1.5 text-sm leading-5 text-gray-500 dark:text-gray-400">{{ entry.description }}</p></div></div></div></div></div>
      </section>

      <section class="flex items-start gap-3 rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-900/20 dark:text-blue-200"><Icon name="infoCircle" size="md" class="mt-0.5 shrink-0" /><div><strong class="font-semibold">{{ t('admin.upstreamAudit.noticeTitle') }}</strong><p class="mt-1 leading-6">{{ t('admin.upstreamAudit.notice') }}</p></div></section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { UpstreamAuditJob, UpstreamAuditStatus } from '@/api/admin'

type TabKey = 'queue' | 'library' | 'settings'
type IconName = InstanceType<typeof Icon>['$props']['name']
const { t } = useI18n()
const activeTab = ref<TabKey>('queue')
const accountCount = ref<number | null>(null)
const loading = ref(false)
const starting = ref(false)
const error = ref('')
const supportedModels = ref<string[]>([])
const bankHash = ref('')
const job = ref<UpstreamAuditJob | null>(null)
let timer: ReturnType<typeof setTimeout> | null = null

const running = computed(() => job.value?.status === 'scanning')
const items = computed(() => job.value?.items ?? [])
const progress = computed(() => { if (!job.value) return 0; if (!running.value) return 100; const a = job.value.total_accounts ? job.value.scanned_accounts / job.value.total_accounts : 0; const m = job.value.matched_models ? job.value.completed / job.value.matched_models : 0; return Math.min(99, Math.round((a * 0.35 + m * 0.65) * 100)) })
const metrics = computed(() => [
  { label: t('admin.upstreamAudit.metrics.accounts'), value: loading.value ? t('admin.upstreamAudit.metrics.loading') : (accountCount.value ?? job.value?.total_accounts ?? '—'), icon: 'server' as IconName, iconClass: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300' },
  { label: t('admin.upstreamAudit.metrics.supported'), value: supportedModels.value.length || 16, icon: 'database' as IconName, iconClass: 'bg-violet-100 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300' },
  { label: t('admin.upstreamAudit.metrics.matched'), value: job.value?.matched_models ?? 0, icon: 'search' as IconName, iconClass: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300' },
  { label: t('admin.upstreamAudit.metrics.lastRun'), value: job.value ? date(job.value.completed_at || job.value.started_at) : t('admin.upstreamAudit.metrics.never'), icon: 'clock' as IconName, iconClass: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300' }
])
const tabs = computed(() => [{ key: 'queue' as const, label: t('admin.upstreamAudit.tabs.queue') }, { key: 'library' as const, label: t('admin.upstreamAudit.tabs.library') }, { key: 'settings' as const, label: t('admin.upstreamAudit.tabs.settings') }])
const modelGroups = computed(() => [{ family: 'gpt', label: t('admin.upstreamAudit.library.gpt'), models: supportedModels.value.filter(model => model.startsWith('gpt-')) }, { family: 'claude', label: t('admin.upstreamAudit.library.claude'), models: supportedModels.value.filter(model => model.startsWith('claude-')) }])
const policyItems = computed(() => [
  { title: t('admin.upstreamAudit.settings.scopeTitle'), description: t('admin.upstreamAudit.settings.scopeDescription'), icon: 'server' as IconName },
  { title: t('admin.upstreamAudit.settings.retriesTitle'), description: t('admin.upstreamAudit.settings.retriesDescription'), icon: 'refresh' as IconName },
  { title: t('admin.upstreamAudit.settings.languagesTitle'), description: t('admin.upstreamAudit.settings.languagesDescription'), icon: 'globe' as IconName },
  { title: t('admin.upstreamAudit.settings.thresholdTitle'), description: t('admin.upstreamAudit.settings.thresholdDescription'), icon: 'chart' as IconName }
])

async function refresh() { loading.value = true; error.value = ''; try { const [accounts, overview] = await Promise.all([adminAPI.accounts.list(1, 1, {}), adminAPI.upstreamAudit.getOverview()]); accountCount.value = accounts.total; supportedModels.value = overview.supported_models; bankHash.value = overview.bank_sha256; job.value = overview.latest_job; poll() } catch (reason) { error.value = message(reason) } finally { loading.value = false } }
async function startAudit() { starting.value = true; error.value = ''; try { job.value = await adminAPI.upstreamAudit.start(); activeTab.value = 'queue'; poll() } catch (reason) { error.value = message(reason) } finally { starting.value = false } }
function poll() { if (timer) clearTimeout(timer); timer = null; if (!job.value || !running.value) return; timer = setTimeout(async () => { try { if (job.value) job.value = await adminAPI.upstreamAudit.getJob(job.value.id) } catch (reason) { error.value = message(reason) } poll() }, 2000) }
function statusLabel(status: UpstreamAuditStatus) { return t(`admin.upstreamAudit.status.${status}`) }
function statusClass(status: UpstreamAuditStatus) { return { verified: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300', mismatch: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300', failed: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300', probing: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300', pending: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' }[status] }
function percent(value?: number) { return typeof value === 'number' ? `${(value * 100).toFixed(1)}%` : '—' }
function date(value?: string) { if (!value) return '—'; const parsed = new Date(value); return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString() }
function message(reason: unknown) { return reason && typeof reason === 'object' && 'message' in reason ? String(reason.message) : t('admin.upstreamAudit.loadFailed') }
onMounted(() => { void refresh() })
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>
