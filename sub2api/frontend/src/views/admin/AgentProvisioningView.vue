<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-8">
      <header class="page-header mb-0 rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 class="page-title flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <Icon name="server" size="sm" class="text-primary-500" />
              {{ t('agentProvisioning.title') }}
            </h1>
            <p class="page-description mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('agentProvisioning.description') }}
            </p>
          </div>
          <div class="flex items-center gap-3">
            <span class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400" :title="t('agentProvisioning.streamStatus')">
              <span class="h-2 w-2 rounded-full" :class="streamConnected ? 'bg-green-500' : 'bg-gray-400'"></span>
              {{ streamConnected ? t('agentProvisioning.live') : t('agentProvisioning.reconnecting') }}
            </span>
            <button type="button" class="btn btn-primary" @click="showCreateForm = !showCreateForm">
              {{ showCreateForm ? t('common.cancel') : t('agentProvisioning.create') }}
            </button>
          </div>
        </div>
      </header>

      <div class="rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-200">
        <p class="font-semibold">{{ t('agentProvisioning.controlPlaneNoticeTitle') }}</p>
        <p class="mt-1">{{ t('agentProvisioning.controlPlaneNotice') }}</p>
      </div>

      <section v-if="showCreateForm" class="rounded-2xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
        <h2 class="mb-4 text-base font-bold text-gray-900 dark:text-white">{{ t('agentProvisioning.create') }}</h2>
        <form class="grid gap-4 md:grid-cols-2" @submit.prevent="createAgent">
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.slug') }}</span>
            <input v-model.trim="form.requestedSlug" class="input w-full" required maxlength="63" pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?" placeholder="team-name" />
            <span class="block text-xs text-gray-500">{{ t('agentProvisioning.domainSuffix') }}</span>
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.displayName') }}</span>
            <input v-model.trim="form.displayName" class="input w-full" required maxlength="100" />
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.ownerMainUserId') }}</span>
            <input v-model.trim="form.ownerMainUserId" class="input w-full" required inputmode="numeric" pattern="(u_)?[0-9]+" placeholder="123" />
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.planId') }}</span>
            <input v-model.trim="form.planId" class="input w-full" required maxlength="64" pattern="[A-Za-z0-9][A-Za-z0-9._-]*" />
            <span class="block text-xs text-gray-500">{{ t('agentProvisioning.planIdHint') }}</span>
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.brandName') }}</span>
            <input v-model.trim="form.brandName" class="input w-full" maxlength="100" />
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t('agentProvisioning.logoUrl') }}</span>
            <input v-model.trim="form.logoUrl" class="input w-full" type="url" maxlength="2048" placeholder="https://…" />
          </label>
          <div class="flex flex-wrap items-center gap-3 md:col-span-2">
            <button type="submit" class="btn btn-primary" :disabled="creating">
              {{ creating ? t('common.loading') : t('agentProvisioning.submitCreate') }}
            </button>
            <span v-if="createError" class="text-sm text-red-600 dark:text-red-400">{{ createError }}</span>
          </div>
        </form>
      </section>

      <TablePageLayout>
        <template #filters>
          <div class="flex flex-wrap items-end gap-3 rounded-xl bg-white p-4 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
            <label class="min-w-52 flex-1 space-y-1 text-sm text-gray-600 dark:text-gray-300">
              <span>{{ t('agentProvisioning.search') }}</span>
              <input v-model="searchDraft" class="input w-full" maxlength="100" :placeholder="t('agentProvisioning.searchPlaceholder')" @keyup.enter="applyFilters" />
            </label>
            <label class="w-48 space-y-1 text-sm text-gray-600 dark:text-gray-300">
              <span>{{ t('agentProvisioning.status') }}</span>
              <select v-model="statusFilter" class="input w-full">
                <option value="">{{ t('agentProvisioning.allStatuses') }}</option>
                <option v-for="status in statuses" :key="status" :value="status">{{ statusLabel(status) }}</option>
              </select>
            </label>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="applyFilters">{{ t('common.search') }}</button>
            <button type="button" class="btn btn-ghost" :disabled="loading" @click="loadAgents">{{ t('common.refresh') }}</button>
          </div>
        </template>

        <template #table>
          <div v-if="loadError" role="alert" class="mb-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
            {{ loadError }}
          </div>
          <DataTable :columns="columns" :data="agents" :loading="loading" row-key="agent_id">
            <template #cell-slug="{ row }">
              <div class="min-w-40">
                <div class="font-semibold text-gray-900 dark:text-white">{{ row.slug }}</div>
                <a class="text-xs text-primary-600 hover:underline dark:text-primary-400" :href="`https://${row.domain}`" target="_blank" rel="noopener noreferrer">{{ row.domain }}</a>
              </div>
            </template>
            <template #cell-status="{ row }">
              <span class="inline-flex rounded-full px-2.5 py-1 text-xs font-semibold" :class="statusClass(row.status)">{{ statusLabel(row.status) }}</span>
            </template>
            <template #cell-updated_at="{ value }">
              <span class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
            </template>
            <template #cell-recent_error="{ value }">
              <span v-if="value" class="block max-w-64 whitespace-normal break-words text-xs text-red-700 dark:text-red-300">{{ value }}</span>
              <span v-else class="text-xs text-gray-400">—</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex flex-wrap gap-2">
                <button v-if="['pending', 'provisioning', 'failed'].includes(row.status)" type="button" class="btn btn-sm btn-primary" :disabled="transitioningId === row.agent_id" @click="askTransition(row, 'activate')">{{ t('agentProvisioning.activate') }}</button>
                <button v-if="row.status === 'failed' && row.can_retry" type="button" class="btn btn-sm btn-secondary" :disabled="transitioningId === row.agent_id" @click="askTransition(row, 'retry')">{{ t('agentProvisioning.retry') }}</button>
                <button v-if="row.status === 'active'" type="button" class="btn btn-sm btn-secondary" :disabled="transitioningId === row.agent_id" @click="askTransition(row, 'suspend')">{{ t('agentProvisioning.suspend') }}</button>
                <button v-else-if="row.status === 'suspended'" type="button" class="btn btn-sm btn-secondary" :disabled="transitioningId === row.agent_id" @click="askTransition(row, 'resume')">{{ t('agentProvisioning.resume') }}</button>
                <button v-if="row.status !== 'revoked'" type="button" class="btn btn-sm btn-danger" :disabled="transitioningId === row.agent_id" @click="askTransition(row, 'revoke')">{{ t('agentProvisioning.revoke') }}</button>
              </div>
            </template>
            <template #empty>
              <div class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('agentProvisioning.empty') }}</div>
            </template>
          </DataTable>
        </template>

        <template #pagination>
          <Pagination
            v-if="total > 0"
            :page="page"
            :total="total"
            :page-size="pageSize"
            :page-size-options="[10, 20, 50, 100]"
            @update:page="changePage"
            @update:pageSize="changePageSize"
          />
        </template>
      </TablePageLayout>
    </div>

    <ConfirmDialog
      :show="Boolean(pendingTransition)"
      :title="transitionTitle"
      :message="transitionMessage"
      :confirm-text="transitionConfirmText"
      :cancel-text="t('common.cancel')"
      :danger="pendingTransition?.operation === 'revoke'"
      @confirm="confirmTransition"
      @cancel="cancelTransition"
    >
      <label v-if="pendingTransition?.operation === 'revoke'" class="block space-y-2">
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('agentProvisioning.confirmRevoke', { id: pendingTransition.agent.agent_id }) }}</span>
        <input v-model.trim="revokeConfirmation" class="input w-full" autocomplete="off" />
      </label>
      <label v-if="pendingTransition?.operation === 'activate'" class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input v-model="readinessConfirmed" class="mt-1" type="checkbox" />
        <span>{{ t('agentProvisioning.confirmReadiness', { domain: pendingTransition.agent.domain }) }}</span>
      </label>
      <p v-if="transitionError" class="text-sm text-red-600 dark:text-red-400">{{ transitionError }}</p>
    </ConfirmDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  activateAgentProvisioningAgent,
  createAgentProvisioningAgent,
  listAgentProvisioningAgents,
  resumeAgentProvisioningAgent,
  retryAgentProvisioningAgent,
  revokeAgentProvisioningAgent,
  streamAgentProvisioningUpdates,
  suspendAgentProvisioningAgent,
  type AgentProvisioningAgent,
  type AgentProvisioningStatus
} from '@/api/admin/agentProvisioning'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const statuses: AgentProvisioningStatus[] = ['pending', 'provisioning', 'active', 'suspended', 'failed', 'revoked']
const columns: Column[] = [
  { key: 'slug', label: t('agentProvisioning.slug') },
  { key: 'display_name', label: t('agentProvisioning.displayName') },
  { key: 'owner_main_user_id', label: t('agentProvisioning.ownerMainUserId') },
  { key: 'plan_id', label: t('agentProvisioning.planId') },
  { key: 'status', label: t('agentProvisioning.status') },
  { key: 'current_step', label: t('agentProvisioning.currentStep') },
  { key: 'domain_status', label: t('agentProvisioning.domainStatus') },
  { key: 'recent_error', label: t('agentProvisioning.recentError') },
  { key: 'updated_at', label: t('agentProvisioning.updatedAt') },
  { key: 'actions', label: t('common.actions') }
]

const agents = ref<AgentProvisioningAgent[]>([])
const loading = ref(false)
const creating = ref(false)
const showCreateForm = ref(false)
const loadError = ref('')
const createError = ref('')
const transitionError = ref('')
const searchDraft = ref('')
const search = ref('')
const statusFilter = ref<AgentProvisioningStatus | ''>('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const streamConnected = ref(false)
const revokeConfirmation = ref('')
const transitioningId = ref('')
const form = reactive({ requestedSlug: '', displayName: '', ownerMainUserId: '', planId: '', brandName: '', logoUrl: '' })
const pendingTransition = ref<{ agent: AgentProvisioningAgent; operation: 'activate' | 'suspend' | 'resume' | 'retry' | 'revoke' } | null>(null)
const transitionTitle = computed(() => pendingTransition.value ? t(`agentProvisioning.${pendingTransition.value.operation}Title`) : '')
const transitionMessage = computed(() => pendingTransition.value ? t(`agentProvisioning.${pendingTransition.value.operation}Message`, { domain: pendingTransition.value.agent.domain }) : '')
const transitionConfirmText = computed(() => pendingTransition.value ? t(`agentProvisioning.${pendingTransition.value.operation}`) : '')
const readinessConfirmed = ref(false)

let listController: AbortController | null = null
let streamController: AbortController | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let disposed = false

function statusLabel(status: AgentProvisioningStatus): string {
  return t(`agentProvisioning.statuses.${status}`)
}

function statusClass(status: AgentProvisioningStatus): string {
  return {
    pending: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200',
    provisioning: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200',
    active: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200',
    suspended: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-200',
    failed: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-200',
    revoked: 'bg-gray-200 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }[status]
}

function formatDate(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function apiError(error: unknown, fallback: string): string {
  return extractApiErrorMessage(error) || fallback
}

async function loadAgents(): Promise<void> {
  listController?.abort()
  const controller = new AbortController()
  listController = controller
  loading.value = true
  loadError.value = ''
  try {
    const result = await listAgentProvisioningAgents({
      page: page.value,
      page_size: pageSize.value,
      q: search.value || undefined,
      status: statusFilter.value || undefined
    }, controller.signal)
    agents.value = result.items
    total.value = result.total
  } catch (error) {
    if (!controller.signal.aborted) loadError.value = apiError(error, t('agentProvisioning.loadFailed'))
  } finally {
    if (listController === controller) loading.value = false
  }
}

function applyFilters(): void {
  search.value = searchDraft.value.trim()
  page.value = 1
  void loadAgents()
}

function changePage(nextPage: number): void {
  page.value = nextPage
  void loadAgents()
}

function changePageSize(nextSize: number): void {
  pageSize.value = nextSize
  page.value = 1
  void loadAgents()
}

async function createAgent(): Promise<void> {
  creating.value = true
  createError.value = ''
  try {
    await createAgentProvisioningAgent({
      requested_slug: form.requestedSlug.toLowerCase(),
      display_name: form.displayName,
      owner_main_user_id: form.ownerMainUserId,
      plan_id: form.planId,
      brand: { name: form.brandName || form.displayName, ...(form.logoUrl ? { logo_url: form.logoUrl } : {}) },
      domain_mode: 'platform_subdomain'
    })
    Object.assign(form, { requestedSlug: '', displayName: '', ownerMainUserId: '', planId: '', brandName: '', logoUrl: '' })
    showCreateForm.value = false
    search.value = ''
    searchDraft.value = ''
    statusFilter.value = ''
    page.value = 1
    await loadAgents()
  } catch (error) {
    createError.value = apiError(error, t('agentProvisioning.createFailed'))
  } finally {
    creating.value = false
  }
}

function askTransition(agent: AgentProvisioningAgent, operation: 'activate' | 'suspend' | 'resume' | 'retry' | 'revoke'): void {
  pendingTransition.value = { agent, operation }
  revokeConfirmation.value = ''
  readinessConfirmed.value = false
  transitionError.value = ''
}

function cancelTransition(): void {
  pendingTransition.value = null
  revokeConfirmation.value = ''
  readinessConfirmed.value = false
  transitionError.value = ''
}

async function confirmTransition(): Promise<void> {
  const pending = pendingTransition.value
  if (!pending) return
  if (transitioningId.value) return
  if (pending.operation === 'revoke' && revokeConfirmation.value !== pending.agent.agent_id) {
    transitionError.value = t('agentProvisioning.confirmationMismatch')
    return
  }
  if (pending.operation === 'activate' && !readinessConfirmed.value) {
    transitionError.value = t('agentProvisioning.readinessConfirmationRequired')
    return
  }
  transitioningId.value = pending.agent.agent_id
  transitionError.value = ''
  try {
    if (pending.operation === 'activate') await activateAgentProvisioningAgent(pending.agent.agent_id)
    else if (pending.operation === 'suspend') await suspendAgentProvisioningAgent(pending.agent.agent_id)
    else if (pending.operation === 'resume') await resumeAgentProvisioningAgent(pending.agent.agent_id)
    else if (pending.operation === 'retry') await retryAgentProvisioningAgent(pending.agent.agent_id)
    else await revokeAgentProvisioningAgent(pending.agent.agent_id, revokeConfirmation.value)
    cancelTransition()
    await loadAgents()
  } catch (error) {
    transitionError.value = apiError(error, t('agentProvisioning.actionFailed'))
  } finally {
    transitioningId.value = ''
  }
}

async function connectStream(): Promise<void> {
  if (disposed) return
  const controller = new AbortController()
  streamController = controller
  try {
    await streamAgentProvisioningUpdates({
      signal: controller.signal,
      onResync: () => { streamConnected.value = true; void loadAgents() },
      onAgent: () => { streamConnected.value = true; void loadAgents() }
    })
  } catch {
    if (!controller.signal.aborted) streamConnected.value = false
  } finally {
    if (streamController === controller) streamController = null
    if (!disposed) {
      streamConnected.value = false
      reconnectTimer = setTimeout(() => { void connectStream() }, 3000)
    }
  }
}

onMounted(() => {
  void loadAgents()
  void connectStream()
})

onUnmounted(() => {
  disposed = true
  listController?.abort()
  streamController?.abort()
  if (reconnectTimer) clearTimeout(reconnectTimer)
})
</script>
