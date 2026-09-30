<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { agentAPI, type AgentAPIKeyView } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import AgentConfirmDialog from './AgentConfirmDialog.vue'
const props = defineProps<{ userId: string }>()
const keys = ref<AgentAPIKeyView[]>([])
const name = ref('')
const secret = ref('')
const error = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)
const revokeTarget = ref<{ userId: string; id: number; name: string } | null>(null)
let sequence = 0
let memberEpoch = 0
onBeforeUnmount(() => { memberEpoch++; sequence++ })
async function load() {
  const current = ++sequence
  error.value = ''
  keys.value = []
  loading.value = true
  try {
    const result = await agentAPI.keys.listForUser(props.userId)
    if (current === sequence) keys.value = result.items
  } catch (err) { if (current === sequence) error.value = errorMessage(err, '读取 Key 失败') }
  finally { if (current === sequence) loading.value = false }
}
watch(() => props.userId, () => { memberEpoch++; busy.value = false; revokeTarget.value = null; secret.value = ''; notice.value = ''; name.value = ''; void load() }, { immediate: true })
async function create() {
  if (busy.value) return
  const epoch = memberEpoch
  busy.value = true
  error.value = ''
  notice.value = ''
  secret.value = ''
  const userId = props.userId
  try {
    const result = await agentAPI.keys.createForUser(userId, name.value)
    if (epoch !== memberEpoch) return
    secret.value = result.key
    notice.value = '完整密钥仅显示一次，请在关闭提示前复制保存。'
    await load()
  } catch (err) { if (epoch === memberEpoch) error.value = errorMessage(err, '创建 Key 失败') }
  finally { if (epoch === memberEpoch) busy.value = false }
}
async function copySecret() {
  if (!secret.value) return
  try {
    await navigator.clipboard.writeText(secret.value)
    notice.value = '已复制到剪贴板，请像保护密码一样妥善保管。'
  } catch {
    notice.value = '剪贴板访问被阻止，请手动选中并复制密钥。'
  }
}
function formatDateTime(value?: string): string {
  if (!value) return '从未使用'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}
async function revoke() {
  const target = revokeTarget.value
  if (!target || busy.value || target.userId !== props.userId) return
  const epoch = memberEpoch
  revokeTarget.value = null
  busy.value = true
  error.value = ''
  try {
    await agentAPI.keys.revokeForUser(target.userId, target.id)
    if (epoch === memberEpoch) { secret.value = ''; await load() }
  }
  catch (err) { if (epoch === memberEpoch) error.value = errorMessage(err, '撤销 Key 失败') }
  finally { if (epoch === memberEpoch) busy.value = false }
}
</script>
<template>
  <section class="space-y-5" aria-label="用户 API Key 管理">
    <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
      <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
        <Icon name="key" size="lg" />
      </div>
      <div class="min-w-0">
        <h2 class="truncate font-medium text-gray-900 dark:text-white">用户 {{ userId }} 的代理站 API Key</h2>
        <p class="mt-0.5 text-sm text-gray-500 dark:text-dark-400">客户端使用本站地址和此密钥；请求由代理站转发到 Sub2API，并由主站向该用户计费。</p>
      </div>
    </div>

    <form class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800" @submit.prevent="create">
      <label for="member-key-name" class="input-label">创建新密钥</label>
      <div class="mt-2 flex flex-col gap-3 sm:flex-row">
        <div class="min-w-0 flex-1">
          <AgentInput id="member-key-name" v-model="name" aria-label="Key 名称" maxlength="100" placeholder="密钥名称，例如：桌面客户端" :disabled="busy" />
        </div>
        <button type="submit" :disabled="busy" class="btn btn-primary shrink-0">
          <Icon name="plus" size="sm" class="mr-2" />{{ busy ? '正在处理…' : '创建 Key' }}
        </button>
      </div>
      <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">密钥只属于当前代理站和当前用户，不会暴露主站管理员 Key 或主站身份令牌。</p>
    </form>

    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>

    <div v-if="secret" class="space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/60 dark:bg-amber-950/30">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="font-medium text-amber-900 dark:text-amber-100">新密钥 — 请立即复制</h3>
          <p class="mt-0.5 text-xs text-amber-700 dark:text-amber-300">{{ notice }}</p>
        </div>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary btn-sm" @click="copySecret"><Icon name="copy" size="sm" class="mr-1.5" />复制</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="secret = ''; notice = ''"><Icon name="eyeOff" size="sm" class="mr-1.5" />隐藏</button>
        </div>
      </div>
      <code class="block select-all break-all rounded-lg bg-white p-3 text-sm text-slate-900 dark:bg-slate-950 dark:text-slate-100">{{ secret }}</code>
    </div>

    <div v-if="loading" class="flex justify-center py-10" aria-label="正在加载 API Key">
      <Icon name="refresh" size="xl" class="animate-spin text-primary-500" />
    </div>
    <div v-else-if="keys.length === 0" class="rounded-xl border border-dashed border-gray-300 py-10 text-center dark:border-dark-600">
      <Icon name="key" size="xl" class="mx-auto text-gray-400" />
      <p class="mt-3 text-sm font-medium text-gray-700 dark:text-dark-200">该用户暂无 API Key</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">可在上方创建一枚仅用于本站网关的新密钥。</p>
    </div>
    <ul v-else class="max-h-96 space-y-3 overflow-y-auto pr-1">
      <li v-for="key in keys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0 flex-1">
            <div class="mb-1 flex flex-wrap items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ key.name || '未命名密钥' }}</span>
              <AgentStatusBadge :status="key.status" :label="key.status === 'active' ? '启用中' : '已撤销'" />
            </div>
            <p class="truncate font-mono text-sm text-gray-500 dark:text-dark-400">{{ key.prefix }}…</p>
          </div>
          <button v-if="key.status === 'active'" type="button" :disabled="busy" class="btn btn-danger btn-sm shrink-0" @click="revokeTarget = { userId, id: key.id, name: key.name }">
            <Icon name="trash" size="sm" class="mr-1.5" />撤销 Key
          </button>
        </div>
        <div class="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
          <span>创建时间：{{ formatDateTime(key.created_at) }}</span>
          <span>最后使用：{{ formatDateTime(key.last_used_at) }}</span>
        </div>
      </li>
    </ul>
    <AgentConfirmDialog :open="revokeTarget !== null" title="撤销用户 API Key" :message="`确认撤销用户 ${revokeTarget?.userId ?? ''} 的 ${revokeTarget?.name ?? ''}？使用此 Key 的后续调用将被拒绝，此操作不可恢复。`" confirm-label="确认撤销" destructive @cancel="revokeTarget = null" @confirm="revoke" />
  </section>
</template>
