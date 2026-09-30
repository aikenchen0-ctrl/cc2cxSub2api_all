<script setup lang="ts">
import { ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { agentPasskeyAPI, type PasskeyCredentialSummary } from '@/agent/passkeys'
import { errorMessage } from '@/agent/client'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentInput from '@/components/common/AgentInput.vue'

const props = defineProps<{ enabled: boolean }>()
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const showAdd = ref(false)
const newName = ref('')
const newPassword = ref('')
const renameTarget = ref<PasskeyCredentialSummary | null>(null)
const renameName = ref('')
const deleteTarget = ref<PasskeyCredentialSummary | null>(null)
const deletePassword = ref('')
const credentials = ref<PasskeyCredentialSummary[]>([])
const supported = agentPasskeyAPI.isSupported()

async function load(): Promise<void> {
  if (!props.enabled) {
    credentials.value = []
    return
  }
  loading.value = true
  error.value = ''
  try {
    credentials.value = await agentPasskeyAPI.list()
  } catch (err) {
    error.value = errorMessage(err, '加载 Passkey 失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function add(): Promise<void> {
  if (!newPassword.value || busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await agentPasskeyAPI.register(newName.value.trim(), newPassword.value)
    newName.value = ''
    newPassword.value = ''
    showAdd.value = false
    notice.value = 'Passkey 已添加。'
    await load()
  } catch (err) {
    if (!(err instanceof DOMException && err.name === 'NotAllowedError')) {
      error.value = errorMessage(err, '添加 Passkey 失败。')
    }
  } finally {
    busy.value = false
  }
}

function openRename(credential: PasskeyCredentialSummary): void {
  if (busy.value) return
  renameTarget.value = credential
  renameName.value = credential.name
}

function closeDelete(): void {
  if (busy.value) return
  deleteTarget.value = null
  deletePassword.value = ''
}

function closeRename(): void {
  if (busy.value) return
  renameTarget.value = null
  renameName.value = ''
}

async function rename(): Promise<void> {
  const credential = renameTarget.value
  const name = renameName.value.trim()
  if (!credential || !name || busy.value) return
  if (name === credential.name) {
    closeRename()
    return
  }
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await agentPasskeyAPI.rename(credential.id, name)
    credential.name = name
    renameTarget.value = null
    renameName.value = ''
    notice.value = 'Passkey 名称已更新。'
  } catch (err) {
    error.value = errorMessage(err, '重命名 Passkey 失败。')
  } finally {
    busy.value = false
  }
}

async function remove(): Promise<void> {
  const target = deleteTarget.value
  if (!target || !deletePassword.value || busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await agentPasskeyAPI.remove(target.id, deletePassword.value)
    credentials.value = credentials.value.filter((item) => item.id !== target.id)
    deleteTarget.value = null
    deletePassword.value = ''
    notice.value = 'Passkey 已删除。'
  } catch (err) {
    error.value = errorMessage(err, '删除 Passkey 失败。')
  } finally {
    busy.value = false
  }
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(new Date(value))
}

watch(() => props.enabled, () => { void load() }, { immediate: true })
</script>

<template>
  <section data-testid="profile-passkey-card" class="card">
    <div class="flex items-start justify-between gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-medium text-gray-900 dark:text-white">Passkey</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">使用设备指纹、面容或安全密钥登录；凭据由 Sub2API 主站统一保存与验证。</p>
      </div>
      <button v-if="enabled && supported && !showAdd" type="button" class="btn btn-primary shrink-0" :disabled="busy" @click="showAdd = true">
        添加 Passkey
      </button>
    </div>
    <div class="space-y-4 px-6 py-6">
      <p v-if="!enabled" class="text-sm text-gray-500 dark:text-gray-400">当前代理站域名未包含在主站 Passkey 的 RP Origin 配置中，此功能暂不可用。</p>
      <p v-else-if="!supported" class="text-sm text-amber-600 dark:text-amber-300">当前浏览器不支持 Passkey。</p>
      <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200">{{ notice }}</p>

      <form v-if="enabled && supported && showAdd" class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700" @submit.prevent="add">
        <div class="grid gap-4 sm:grid-cols-2">
          <div><label for="agent-passkey-name" class="input-label">名称</label><AgentInput id="agent-passkey-name" v-model="newName" maxlength="100" placeholder="例如：办公电脑" /></div>
          <div><label for="agent-passkey-password" class="input-label">当前密码</label><AgentInput id="agent-passkey-password" v-model="newPassword" type="password" autocomplete="current-password" required /></div>
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="busy" @click="showAdd = false; newName = ''; newPassword = ''">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="busy || !newPassword">{{ busy ? '处理中…' : '继续' }}</button>
        </div>
      </form>

      <p v-if="loading" role="status" class="py-5 text-center text-sm text-gray-500">正在加载 Passkey…</p>
      <div v-else-if="enabled && supported && credentials.length === 0" class="rounded-xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700">尚未添加 Passkey。</div>
      <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
        <div v-for="credential in credentials" :key="credential.id" class="flex items-center justify-between gap-4 py-4 first:pt-0 last:pb-0">
          <div class="min-w-0">
            <div class="flex items-center gap-2"><Icon name="key" size="md" class="shrink-0 text-primary-500" /><strong class="truncate text-sm text-gray-900 dark:text-white">{{ credential.name }}</strong><span v-if="credential.backup" class="badge badge-success">已同步</span></div>
            <p class="mt-1 text-xs text-gray-500">创建于 {{ formatDate(credential.created_at) }}<template v-if="credential.last_used_at"> · 最近使用 {{ formatDate(credential.last_used_at) }}</template></p>
          </div>
          <div class="flex shrink-0 gap-2"><button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="openRename(credential)">重命名</button><button type="button" class="btn btn-ghost btn-sm text-red-600" :disabled="busy" @click="deleteTarget = credential; deletePassword = ''">删除</button></div>
        </div>
      </div>
    </div>

    <BaseDialog
      :show="renameTarget !== null"
      title="重命名 Passkey"
      width="narrow"
      :close-on-escape="!busy"
      :show-close-button="!busy"
      @close="closeRename"
    >
      <form id="agent-passkey-rename-form" @submit.prevent="rename">
        <label for="agent-passkey-rename-name" class="input-label">Passkey 名称</label>
        <AgentInput id="agent-passkey-rename-name" v-model="renameName" maxlength="100" required autofocus placeholder="例如：办公电脑" />
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="closeRename">取消</button>
        <button type="submit" form="agent-passkey-rename-form" class="btn btn-primary" :disabled="busy || !renameName.trim()">{{ busy ? '处理中…' : '保存名称' }}</button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="deleteTarget !== null"
      title="删除 Passkey"
      width="narrow"
      :close-on-escape="!busy"
      :show-close-button="!busy"
      @close="closeDelete"
    >
      <form id="agent-passkey-delete-form" @submit.prevent="remove">
        <p class="text-sm text-gray-500 dark:text-gray-400">将删除“{{ deleteTarget?.name }}”。请输入当前密码确认。</p>
        <label for="agent-passkey-delete-password" class="input-label mt-4">当前密码</label>
        <AgentInput id="agent-passkey-delete-password" v-model="deletePassword" type="password" autocomplete="current-password" required autofocus />
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="closeDelete">取消</button>
        <button type="submit" form="agent-passkey-delete-form" class="btn btn-danger" :disabled="busy || !deletePassword">{{ busy ? '处理中…' : '删除' }}</button>
      </template>
    </BaseDialog>
  </section>
</template>
