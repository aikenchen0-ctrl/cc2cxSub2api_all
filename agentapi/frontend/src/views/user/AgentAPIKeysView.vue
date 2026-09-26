<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentAPIKeyView } from '@/agent/api'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import { statusLabel } from '@/agent/locale'
import { errorMessage } from '@/agent/client'

const keys = ref<AgentAPIKeyView[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const name = ref('')
const oneTimeKey = ref('')
const revokeConfirmation = ref<AgentAPIKeyView | null>(null)
const revokeConfirmationMessage = computed(() => {
  const key = revokeConfirmation.value
  return key ? `确定撤销“${key.name || key.prefix}”吗？撤销后无法继续使用此密钥。` : ''
})

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
  } catch (err) {
    error.value = errorMessage(err, '加载 API 密钥失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function create(): Promise<void> {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await agentAPI.keys.create(name.value.trim() || 'AgentAPI 密钥')
    oneTimeKey.value = response.key
    keys.value = [response.item, ...keys.value]
    name.value = ''
    notice.value = '完整密钥仅显示一次，请在关闭此提示前复制保存。'
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
    if (oneTimeKey.value && oneTimeKey.value.startsWith(key.prefix)) oneTimeKey.value = ''
  } catch (err) {
    error.value = errorMessage(err, '撤销 API 密钥失败，请稍后重试。')
  }
}

async function copyKey(): Promise<void> {
  if (!oneTimeKey.value) return
  try {
    await navigator.clipboard.writeText(oneTimeKey.value)
    notice.value = '已复制到剪贴板。请像保护密码一样妥善保管此密钥。'
  } catch {
    notice.value = '剪贴板访问被阻止，请手动选中并复制密钥。'
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-5xl space-y-6 p-6">
    <header>
      <p class="text-sm text-slate-500">AgentAPI</p>
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">API 密钥</h1>
      <p class="mt-1 text-sm text-slate-500">调用此代理网关时，请在 <code>Authorization: Bearer</code> 请求头中使用 AgentAPI 密钥。</p>
    </header>

    <section class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
      AgentAPI 密钥仅用于本地网关。此处不会显示主站管理员密钥或主站身份令牌。新密钥以哈希形式保存，完整密钥仅显示一次。
    </section>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>

    <section v-if="oneTimeKey" class="space-y-3 rounded-xl border border-blue-200 bg-blue-50 p-5 dark:border-blue-900/60 dark:bg-blue-950/30">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="font-semibold text-blue-900 dark:text-blue-100">新密钥 — 请立即复制</h2>
        <button class="rounded-lg bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700" type="button" @click="copyKey">复制密钥</button>
      </div>
      <code class="block select-all break-all rounded-lg bg-white p-3 text-sm text-slate-900 dark:bg-slate-950 dark:text-slate-100">{{ oneTimeKey }}</code>
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold text-slate-900 dark:text-white">创建密钥</h2>
      <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="create">
        <input v-model="name" class="min-w-0 flex-1 rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="100" placeholder="密钥名称，例如：桌面客户端" type="text">
        <button class="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-white dark:text-slate-900" :disabled="saving" type="submit">{{ saving ? '正在创建…' : '创建密钥' }}</button>
      </form>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700"><h2 class="font-semibold text-slate-900 dark:text-white">我的密钥</h2></div>
      <div v-if="loading" class="p-5 text-sm text-slate-500">正在加载…</div>
      <div v-else-if="keys.length === 0" class="p-5 text-sm text-slate-500">暂无 AgentAPI 密钥。</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">名称</th><th class="px-5 py-3">前缀</th><th class="px-5 py-3">创建时间</th><th class="px-5 py-3">最后使用时间</th><th class="px-5 py-3">状态</th><th class="px-5 py-3"></th></tr></thead>
          <tbody>
            <tr v-for="key in keys" :key="key.id" class="border-t dark:border-slate-700">
              <td class="px-5 py-4 font-medium text-slate-900 dark:text-white">{{ key.name || '—' }}</td>
              <td class="px-5 py-4 font-mono text-xs text-slate-500">{{ key.prefix }}…</td>
              <td class="px-5 py-4 text-slate-500">{{ formatDate(key.created_at) }}</td>
              <td class="px-5 py-4 text-slate-500">{{ formatDate(key.last_used_at) }}</td>
              <td class="px-5 py-4"><span :class="key.status === 'active' ? 'text-emerald-600' : 'text-slate-400'">{{ statusLabel(key.status) }}</span></td>
              <td class="px-5 py-4 text-right"><button v-if="key.status === 'active'" class="text-sm text-red-600 hover:underline" type="button" @click="requestRevoke(key)">撤销</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

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
