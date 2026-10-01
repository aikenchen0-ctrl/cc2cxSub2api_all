<script setup lang="ts">
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentSearchInput from '@/components/common/AgentSearchInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import DataTable from '@/components/common/DataTable.vue'
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAPIKeyView } from '@/agent/api'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import { statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'
import { useAgentToast } from '@/composables/useAgentToast'

const { showSuccess, showWarning } = useAgentToast()

const showCreate = ref(false)
const search = ref('')
const filterStatus = ref('')
const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '启用中' },
  { value: 'revoked', label: '已撤销' },
]
const filteredKeys = computed(() => keys.value.filter(key => (!filterStatus.value || key.status === filterStatus.value) && (key.name + ' ' + key.prefix).toLowerCase().includes(search.value.trim().toLowerCase())))
const keys = ref<AgentAPIKeyView[]>([])
const configuredAPIBaseURL = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const name = ref('')
const copiedKeyID = ref<number | null>(null)
const addressCopied = ref(false)
const revokeConfirmation = ref<AgentAPIKeyView | null>(null)
const revokeConfirmationMessage = computed(() => {
  const key = revokeConfirmation.value
  return key ? `确定撤销“${key.name || key.prefix}”吗？撤销后无法继续使用此密钥。` : ''
})
const apiBaseURL = computed(() => configuredAPIBaseURL.value.trim() || `${window.location.origin}/v1`)

function formatDate(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const response = await agentAPI.keys.list()
    keys.value = response.items
    configuredAPIBaseURL.value = response.api_base_url || ''
  } catch (err) {
    error.value = errorMessage(err, '加载 API 密钥失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function create(): Promise<void> {
  if (saving.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await agentAPI.keys.create(name.value.trim() || 'AgentAPI 密钥')
    showCreate.value = false
    keys.value = [{ ...response.item, key: response.item.key || response.key }, ...keys.value]
    name.value = ''
    notice.value = 'API 密钥已创建，可随时在密钥列表中复制。'
    showSuccess('API 密钥已创建。')
  } catch (err) {
    error.value = errorMessage(err, '创建 API 密钥失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}

function requestRevoke(key: AgentAPIKeyView): void {
  if (key.status === 'active') revokeConfirmation.value = key
}

async function confirmRevoke(): Promise<void> {
  const key = revokeConfirmation.value
  if (!key || key.status !== 'active') {
    revokeConfirmation.value = null
    return
  }
  revokeConfirmation.value = null
  error.value = ''
  try {
    await agentAPI.keys.revoke(key.id)
    key.status = 'revoked'
    showSuccess('API 密钥已撤销。')
  } catch (err) {
    error.value = errorMessage(err, '撤销 API 密钥失败，请稍后重试。')
  }
}

async function copyStoredKey(key: AgentAPIKeyView): Promise<void> {
  if (!key.key) {
    showWarning('密钥正在更新，请刷新后重试。')
    return
  }
  try {
    await navigator.clipboard.writeText(key.key)
    copiedKeyID.value = key.id
    notice.value = '已复制到剪贴板。请像保护密码一样妥善保管此密钥。'
    showSuccess('密钥已复制到剪贴板。')
  } catch {
    notice.value = '剪贴板访问被阻止，请手动选中并复制密钥。'
    showWarning('剪贴板访问被阻止，请手动复制密钥。')
  }
}

async function copyAPIBaseURL(): Promise<void> {
  try {
    await navigator.clipboard.writeText(apiBaseURL.value)
    addressCopied.value = true
    showSuccess('API 调用地址已复制。')
  } catch {
    showWarning('剪贴板访问被阻止，请手动复制 API 调用地址。')
  }
}

onMounted(load)
</script>

<template>
  <main class="space-y-6">
    <header>
      <p class="text-sm text-slate-500">AgentAPI</p>
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">API 密钥</h1>
      <p class="mt-1 text-sm text-slate-500">调用此代理网关时，请在 <code>Authorization: Bearer</code> 请求头中使用 AgentAPI 密钥。</p>
    </header>

    <section class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
      API 密钥仅用于本站模型接口。所有密钥都会加密保存，可随时点击密钥旁的复制图标复制完整值。
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
      <p class="text-sm font-medium text-gray-900 dark:text-white">API 调用地址</p>
      <div class="mt-2 flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900">
        <code class="min-w-0 flex-1 select-all break-all text-sm text-gray-700 dark:text-dark-200">{{ apiBaseURL }}</code>
        <button type="button" data-testid="copy-api-base-url" class="shrink-0 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200" :aria-label="addressCopied ? '调用地址已复制' : '复制 API 调用地址'" :title="addressCopied ? '已复制' : '复制到剪贴板'" @click="copyAPIBaseURL">
          <Icon v-if="addressCopied" name="check" size="sm" />
          <Icon v-else name="clipboard" size="sm" />
        </button>
      </div>
    </section>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>

    <TablePageLayout>
      <template #actions>
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-lg font-semibold">我的密钥</h2>
          <div class="flex gap-3">
            <button class="btn btn-secondary" :disabled="loading" aria-label="刷新密钥" @click="load"><Icon name="refresh" size="md" /></button>
            <button class="btn btn-primary" data-testid="open-create-key" @click="showCreate = true"><Icon name="plus" size="md" class="mr-2" />创建密钥</button>
          </div>
        </div>
      </template>
      <template #filters>
        <div class="flex flex-wrap gap-3">
          <AgentSearchInput v-model="search" class="w-full sm:w-64" aria-label="搜索密钥" placeholder="搜索名称或前缀" />
          <AgentSelect v-model="filterStatus" :options="statusOptions" class="w-40" aria-label="密钥状态" />
        </div>
      </template>
      <template #table>
      <DataTable :columns='[{"key":"name","label":"名称"},{"key":"key","label":"API Key"},{"key":"created_at","label":"创建时间"},{"key":"last_used_at","label":"最后使用时间"},{"key":"status","label":"状态"},{"key":"actions","label":"操作"}]' :data="filteredKeys" :loading="loading" row-key="id">
        <template #cell-name="{ row: key }"><div class="max-w-xl whitespace-normal">{{ key.name || '—' }}</div></template>
        <template #cell-key="{ row: key }">
          <div class="flex max-w-xl items-center gap-2">
            <code class="min-w-0 select-all break-all text-xs">{{ key.key || `${key.prefix}…` }}</code>
            <button type="button" :data-testid="`copy-key-${key.id}`" class="shrink-0 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300" :class="copiedKeyID === key.id ? 'text-green-500' : ''" :aria-label="copiedKeyID === key.id ? '密钥已复制' : '复制 API 密钥'" :title="copiedKeyID === key.id ? '已复制' : '复制到剪贴板'" @click="copyStoredKey(key)">
              <Icon v-if="copiedKeyID === key.id" name="check" size="sm" />
              <Icon v-else name="clipboard" size="sm" />
            </button>
          </div>
        </template>
        <template #cell-created_at="{ row: key }"><div class="max-w-xl whitespace-normal">{{ formatDate(key.created_at) }}</div></template>
        <template #cell-last_used_at="{ row: key }"><div class="max-w-xl whitespace-normal">{{ formatDate(key.last_used_at) }}</div></template>
        <template #cell-status="{ row: key }"><div class="max-w-xl whitespace-normal"><AgentStatusBadge :status="key.status" :label="statusLabel(key.status)" /></div></template>
        <template #cell-actions="{ row: key }"><div class="max-w-xl whitespace-normal"><button v-if="key.status === 'active'" class="text-sm text-red-600 hover:underline" type="button" @click="requestRevoke(key)">撤销</button></div></template>
        <template #empty>暂无 AgentAPI 密钥。</template>
      </DataTable>
      </template>
      <template #pagination><p class="text-sm text-gray-500">显示 {{ filteredKeys.length }} / {{ keys.length }} 个密钥</p></template>
    </TablePageLayout>
    <BaseDialog :show="showCreate" title="创建 API 密钥" :close-on-escape="!saving" :show-close-button="!saving" @close="showCreate = false">
      <form class="space-y-4" @submit.prevent="create">
        <label for="key-name" class="input-label">密钥名称</label>
        <AgentInput id="key-name" v-model="name" maxlength="100" placeholder="密钥名称，例如：桌面客户端" :disabled="saving" />
        <p class="text-sm text-gray-500">密钥只用于本站；创建后会加密保存，可随时在密钥列表中复制。</p>
        <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" :disabled="saving" @click="showCreate = false">取消</button><button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '正在创建…' : '创建密钥' }}</button></div>
      </form>
    </BaseDialog>

    <AgentConfirmDialog
      :open="revokeConfirmation !== null"
      title="撤销 API 密钥"
      :message="revokeConfirmationMessage"
      confirm-label="确认撤销"
      destructive
      @cancel="revokeConfirmation = null"
      @confirm="confirmRevoke"
    />
  </main>
</template>
