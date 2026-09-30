<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentTenantBackupRecord, type AgentTenantBackupSnapshot } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const backups = ref<AgentTenantBackupRecord[]>([])
const parts = ref<string[]>([])
const loading = ref(true)
const busy = ref<'create' | 'import' | number | null>(null)
const loadError = ref('')
const actionMessage = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const confirmAction = ref<'restore' | 'delete' | null>(null)
const confirmTarget = ref<AgentTenantBackupRecord | null>(null)

const partLabels: Record<string, string> = {
  branding: '品牌信息',
  model_policy: '模型权限',
  announcements: '公告',
  content_pages: '内容页面',
  plan_policies: '套餐展示策略',
}

const includedLabels = computed(() => parts.value.map(item => partLabels[item] || item))
const excludedLabels = ['用户与登录会话', 'API Key 与凭据', '余额、订单与订阅事实', '用量与结算记录', '支付密钥与主站全局设置']

function formatTime(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const result = await agentAPI.adminBackups.list()
    backups.value = result.items
    parts.value = result.parts
  } catch (cause) {
    backups.value = []
    loadError.value = errorMessage(cause, '备份记录加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function createBackup(): Promise<void> {
  if (busy.value) return
  busy.value = 'create'
  actionMessage.value = ''
  try {
    await agentAPI.adminBackups.create()
    await load()
    actionMessage.value = '当前代理站配置备份已创建。'
  } catch (cause) {
    actionMessage.value = errorMessage(cause, '创建备份失败，请稍后重试。')
  } finally {
    busy.value = null
  }
}

function openImport(): void {
  if (!busy.value) fileInput.value?.click()
}

async function importBackup(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || busy.value) return
  if (file.size > 2 * 1024 * 1024) {
    actionMessage.value = '备份文件不能超过 2 MB。'
    return
  }
  busy.value = 'import'
  actionMessage.value = ''
  try {
    const snapshot = JSON.parse(await file.text()) as AgentTenantBackupSnapshot
    await agentAPI.adminBackups.import(snapshot)
    await load()
    actionMessage.value = '备份文件已导入；导入不会自动覆盖当前配置。'
  } catch (cause) {
    actionMessage.value = errorMessage(cause, '导入失败，请确认文件来自当前代理站且格式完整。')
  } finally {
    busy.value = null
  }
}

async function downloadBackup(item: AgentTenantBackupRecord): Promise<void> {
  if (busy.value) return
  busy.value = item.id
  actionMessage.value = ''
  try {
    const result = await agentAPI.adminBackups.download(item.id)
    const blob = new Blob([JSON.stringify(result.snapshot, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = result.record.file_name
    link.click()
    URL.revokeObjectURL(url)
    actionMessage.value = '备份文件已生成下载。'
  } catch (cause) {
    actionMessage.value = errorMessage(cause, '下载备份失败，请稍后重试。')
  } finally {
    busy.value = null
  }
}

function requestRestore(item: AgentTenantBackupRecord): void {
  if (busy.value) return
  confirmAction.value = 'restore'
  confirmTarget.value = item
}

function requestRemove(item: AgentTenantBackupRecord): void {
  if (busy.value) return
  confirmAction.value = 'delete'
  confirmTarget.value = item
}

function cancelConfirm(): void {
  if (busy.value) return
  confirmAction.value = null
  confirmTarget.value = null
}

async function confirmBackupAction(): Promise<void> {
  const action = confirmAction.value
  const item = confirmTarget.value
  if (!action || !item || busy.value) return
  confirmAction.value = null
  confirmTarget.value = null
  busy.value = item.id
  actionMessage.value = ''
  try {
    if (action === 'restore') {
      await agentAPI.adminBackups.restore(item.id)
      await load()
      actionMessage.value = '代理站配置已从备份恢复。'
    } else {
      await agentAPI.adminBackups.remove(item.id)
      backups.value = backups.value.filter(candidate => candidate.id !== item.id)
      actionMessage.value = '备份记录已删除。'
    }
  } catch (cause) {
    actionMessage.value = errorMessage(cause, action === 'restore' ? '恢复备份失败；当前配置未被部分覆盖。' : '删除备份失败，请稍后重试。')
  } finally {
    busy.value = null
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-5 p-6" aria-label="备份管理">
    <header class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">系统管理</p>
        <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">备份管理</h1>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">沿用主站备份页的操作方式，但快照只覆盖当前代理站自己的展示与治理配置。恢复不会改动 Sub2API 主站权威数据。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <input ref="fileInput" class="sr-only" type="file" accept=".json,application/json" aria-label="选择租户备份文件" @change="importBackup">
        <button type="button" class="btn btn-secondary" :disabled="Boolean(busy)" @click="openImport"><Icon name="upload" size="sm" />{{ busy === 'import' ? '导入中…' : '导入备份' }}</button>
        <button type="button" class="btn btn-secondary" :disabled="loading || Boolean(busy)" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
        <button type="button" class="btn btn-primary" :disabled="Boolean(busy)" @click="createBackup"><Icon name="plus" size="sm" />{{ busy === 'create' ? '创建中…' : '创建配置备份' }}</button>
      </div>
    </header>

    <div v-if="loadError" class="flex items-center justify-between gap-4 rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/10 dark:text-red-300" role="alert"><span>{{ loadError }}</span><button type="button" class="btn btn-secondary btn-sm" @click="load">重试</button></div>
    <p v-if="actionMessage" class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/10 dark:text-blue-300" role="status">{{ actionMessage }}</p>

    <section class="grid gap-4 lg:grid-cols-2" aria-label="备份范围">
      <article class="card p-5">
        <div class="flex items-center gap-3"><span class="flex h-10 w-10 items-center justify-center rounded-xl bg-green-50 text-green-600 dark:bg-green-900/20 dark:text-green-300"><Icon name="database" /></span><div><h2 class="font-semibold text-gray-900 dark:text-white">快照包含</h2><p class="text-sm text-gray-500">当前 agent_id 的可迁移配置</p></div></div>
        <div class="mt-4 flex flex-wrap gap-2"><span v-for="label in includedLabels" :key="label" class="badge badge-success">{{ label }}</span><span v-if="!includedLabels.length" class="text-sm text-gray-400">加载范围中…</span></div>
      </article>
      <article class="card p-5">
        <div class="flex items-center gap-3"><span class="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300"><Icon name="shield" /></span><div><h2 class="font-semibold text-gray-900 dark:text-white">明确排除</h2><p class="text-sm text-gray-500">主站事实、用户数据与任何秘密</p></div></div>
        <div class="mt-4 flex flex-wrap gap-2"><span v-for="label in excludedLabels" :key="label" class="badge badge-warning">{{ label }}</span></div>
      </article>
    </section>

    <section class="card overflow-hidden" aria-labelledby="backup-list-title">
      <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6"><h2 id="backup-list-title" class="font-semibold text-gray-900 dark:text-white">配置备份</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">导入文件先保存为候选记录，只有点击恢复并确认后才会事务性替换当前租户配置。</p></div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-100 text-left text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800/60"><tr><th class="px-5 py-3">文件</th><th class="px-5 py-3">状态</th><th class="px-5 py-3">大小</th><th class="px-5 py-3">创建者</th><th class="px-5 py-3">创建时间</th><th class="px-5 py-3">最近恢复</th><th class="px-5 py-3 text-right">操作</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-if="loading"><td colspan="7" class="px-5 py-12 text-center text-gray-500"><Icon name="refresh" class="mx-auto mb-2 animate-spin" />正在加载备份记录…</td></tr>
            <tr v-else-if="!backups.length"><td colspan="7" class="px-5 py-12 text-center text-gray-500"><Icon name="inbox" class="mx-auto mb-2" />尚未创建租户配置备份。</td></tr>
            <tr v-for="item in backups" v-else :key="item.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/40">
              <td class="px-5 py-4"><p class="font-medium text-gray-900 dark:text-white">{{ item.file_name }}</p><p class="mt-1 font-mono text-xs text-gray-400">#{{ item.id }}</p></td>
              <td class="px-5 py-4"><span class="badge badge-success">已完成</span></td>
              <td class="whitespace-nowrap px-5 py-4 text-gray-600 dark:text-gray-300">{{ formatBytes(item.size_bytes) }}</td>
              <td class="px-5 py-4 font-mono text-xs text-gray-500">{{ item.created_by || '—' }}</td>
              <td class="whitespace-nowrap px-5 py-4 text-gray-500">{{ formatTime(item.started_at) }}</td>
              <td class="whitespace-nowrap px-5 py-4 text-gray-500">{{ formatTime(item.restored_at) }}</td>
              <td class="px-5 py-4"><div class="flex justify-end gap-1"><button type="button" class="btn btn-secondary btn-sm" :disabled="Boolean(busy)" :aria-label="`下载 ${item.file_name}`" @click="downloadBackup(item)"><Icon name="download" size="sm" />下载</button><button type="button" class="btn btn-secondary btn-sm" :disabled="Boolean(busy)" :aria-label="`恢复 ${item.file_name}`" @click="requestRestore(item)"><Icon name="sync" size="sm" />恢复</button><button type="button" class="btn btn-danger btn-sm" :disabled="Boolean(busy)" :aria-label="`删除 ${item.file_name}`" @click="requestRemove(item)"><Icon name="trash" size="sm" />删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-800 dark:border-sky-900/40 dark:bg-sky-900/10 dark:text-sky-200">
      <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />
      <p>安全边界：服务端只允许同源站长操作，备份中的 source_agent_id 必须与当前代理站一致。浏览器不会获得 SuperKey、主站管理员 Key、卫星应用凭据或主站用户令牌。</p>
    </section>
    <AgentConfirmDialog
      :open="confirmTarget !== null"
      :title="confirmAction === 'restore' ? '恢复配置备份' : '删除配置备份'"
      :message="confirmTarget ? (confirmAction === 'restore' ? `确认恢复备份“${confirmTarget.file_name}”？当前租户配置将被该快照事务性替换。` : `确认删除备份“${confirmTarget.file_name}”？此操作不可撤销。`) : ''"
      :confirm-label="confirmAction === 'restore' ? '确认恢复' : '删除'"
      :destructive="confirmAction === 'delete'"
      @cancel="cancelConfirm"
      @confirm="confirmBackupAction"
    />
  </main>
</template>
