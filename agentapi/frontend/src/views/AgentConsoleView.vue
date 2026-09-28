<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse, type AgentModelPolicy, type AgentUserView, type AuditEventView, type SettlementView } from '@/agent/api'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import AgentPagination from '@/components/AgentPagination.vue'
import { errorMessage, operationLabel, statusLabel, targetLabel } from '@/agent/locale'
import { applyBranding, applyPageTitle } from '@/agent/branding'

const context = ref<AgentContextResponse | null>(null)
const users = ref<AgentUserView[]>([])
const usersTotal = ref(0)
const usersPage = ref(1)
const usersPageSize = ref(25)
const usersSearchInput = ref('')
const usersSearch = ref('')
const failedSections = ref<string[]>([])
const loading = ref(true)
const error = ref('')
const notice = ref('')
const reconciling = ref(false)
const statusUpdatingUser = ref('')
const statusConfirmation = ref<AgentUserView | null>(null)
const statusConfirmationMessage = computed(() => {
  const user = statusConfirmation.value
  if (!user) return ''
  const account = user.email || user.main_user_id
  return user.status === 'active'
    ? `确定停用 ${account} 在本代理站的访问吗？现有 Session 和客户端 Key 将立即停止使用。`
    : `确定重新启用 ${account} 在本代理站的访问吗？启用后 Session 和客户端 Key 可恢复使用。`
})
const settlements = ref<SettlementView[]>([])
const auditEvents = ref<AuditEventView[]>([])
const brandingName = ref('')
const brandingSiteName = ref('')
const brandingLogo = ref('')
const brandingSaving = ref(false)
const modelPolicy = ref<AgentModelPolicy | null>(null)
const enabledModels = ref<string[]>([])
const modelPolicySaving = ref(false)
let loadSequence = 0
let usersSearchTimer: ReturnType<typeof setTimeout> | undefined

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  const results = await Promise.allSettled([
    agentAPI.getContext(), agentAPI.getUsers(usersPage.value, usersPageSize.value, usersSearch.value), agentAPI.getSettlements(), agentAPI.getAuditEvents(), agentAPI.getAdminModelPolicy(),
  ])
  if (sequence !== loadSequence) return

  const failed: string[] = []
  const [contextResult, usersResult, settlementsResult, auditResult, policyResult] = results
  if (contextResult.status === 'fulfilled') {
    const agentContext = contextResult.value
    context.value = agentContext
    brandingName.value = agentContext.agent.name
    brandingSiteName.value = agentContext.agent.site_name
    brandingLogo.value = agentContext.agent.site_logo || ''
  } else failed.push('agent details')
  if (usersResult.status === 'fulfilled') {
    const lastPage = Math.max(1, Math.ceil(usersResult.value.total / usersPageSize.value))
    if (usersPage.value > lastPage) {
      usersPage.value = lastPage
      await load()
      return
    }
    users.value = usersResult.value.items
    usersTotal.value = usersResult.value.total
  } else failed.push('mapped users')
  if (settlementsResult.status === 'fulfilled') settlements.value = settlementsResult.value.items
  else failed.push('pending settlements')
  if (auditResult.status === 'fulfilled') auditEvents.value = auditResult.value.items
  else failed.push('audit events')
  if (policyResult.status === 'fulfilled') {
    modelPolicy.value = policyResult.value
    enabledModels.value = [...policyResult.value.enabled]
  } else failed.push('model access policy')
  failedSections.value = failed
  error.value = failed.length
    ? `以下管理信息加载失败：${failed.map(sectionLabel).join('、')}。请刷新重试。`
    : ''
  if (sequence === loadSequence) {
    loading.value = false
  }
}

function loadFailed(section: string): boolean {
  return failedSections.value.includes(section)
}

function sectionLabel(section: string): string {
  const labels: Record<string, string> = {
    'agent details': '代理站信息',
    'mapped users': '关联用户',
    'pending settlements': '待结算记录',
    'audit events': '审计记录',
    'model access policy': '模型权限',
  }
  return labels[section] || '其他信息'
}

async function changeUsersPage(nextPage: number): Promise<void> {
  if (nextPage < 1 || nextPage > Math.max(1, Math.ceil(usersTotal.value / usersPageSize.value)) || nextPage === usersPage.value) return
  usersPage.value = nextPage
  await load()
}

async function changeUsersPageSize(nextPageSize: number): Promise<void> {
  if (nextPageSize === usersPageSize.value) return
  usersPageSize.value = nextPageSize
  usersPage.value = 1
  await load()
}

function scheduleUsersSearch(): void {
  if (usersSearchTimer) clearTimeout(usersSearchTimer)
  usersSearchTimer = setTimeout(() => {
    const nextSearch = usersSearchInput.value.trim()
    if (nextSearch === usersSearch.value) return
    usersSearch.value = nextSearch
    usersPage.value = 1
    void load()
  }, 300)
}

async function reconcile(): Promise<void> {
  reconciling.value = true
  notice.value = ''
  try {
    const result = await agentAPI.reconcileSettlements()
    settlements.value = result.items
    notice.value = result.total ? `已核对 ${result.total} 条结算记录。` : '没有待核对的结算记录。'
    await load()
  } catch (err) {
    notice.value = errorMessage(err, '核对结算记录失败，请稍后重试。')
  } finally {
    reconciling.value = false
  }
}

function isAgentOwner(user: AgentUserView): boolean {
  return user.main_user_id === (context.value?.agent.owner_main_user_id || '')
}

function updateUserStatus(user: AgentUserView): void {
  if (statusUpdatingUser.value) return
  statusConfirmation.value = user
}

async function confirmUserStatusUpdate(): Promise<void> {
  const user = statusConfirmation.value
  if (!user) return
  statusConfirmation.value = null
  const status = user.status === 'active' ? 'disabled' : 'active'
  statusUpdatingUser.value = user.main_user_id
  notice.value = ''
  try {
    const updated = await agentAPI.setMappedUserStatus(user.main_user_id, status)
    const index = users.value.findIndex((item) => item.main_user_id === user.main_user_id)
    if (index >= 0) users.value[index] = updated
    notice.value = status === 'disabled'
      ? '该用户在本代理站的访问已停用，新的 AgentAPI 请求将被拒绝。'
      : '该用户在本代理站的访问已启用。'
    await load()
  } catch (err) {
    notice.value = errorMessage(err, '更新用户状态失败。为确保安全，该用户在代理站的访问仍保持关闭。')
    // A disable request closes the local gate before calling Sub2API. Refresh
    // the table even when the main-site update fails so the UI reflects that
    // safe partial state instead of implying the user is still active.
    try {
      const result = await agentAPI.getUsers(usersPage.value, usersPageSize.value, usersSearch.value)
      users.value = result.items
      usersTotal.value = result.total
    } catch {
      // Keep the operation error visible if the follow-up read also fails.
    }
  } finally {
    statusUpdatingUser.value = ''
  }
}

async function saveBranding(): Promise<void> {
  brandingSaving.value = true
  notice.value = ''
  try {
    const updated = await agentAPI.updateBranding({
      name: brandingName.value.trim(),
      site_name: brandingSiteName.value.trim(),
      site_logo: brandingLogo.value.trim(),
    })
    if (context.value) {
      context.value.agent.name = updated.name
      context.value.agent.site_name = updated.site_name
      context.value.agent.site_logo = updated.site_logo || undefined
    }
    brandingName.value = updated.name
    brandingSiteName.value = updated.site_name
    brandingLogo.value = updated.site_logo || ''
    applyBranding(updated.site_name, updated.site_logo)
    applyPageTitle('代理站管理')
    notice.value = '品牌信息已保存，新页面将使用更新后的名称和标志。'
  } catch (err) {
    notice.value = errorMessage(err, '更新品牌信息失败，请稍后重试。')
  } finally {
    brandingSaving.value = false
  }
}

async function saveModelPolicy(): Promise<void> {
  modelPolicySaving.value = true
  notice.value = ''
  try {
    const updated = await agentAPI.updateAdminModelPolicy(enabledModels.value)
    modelPolicy.value = updated
    enabledModels.value = [...updated.enabled]
    notice.value = `模型权限已保存，已启用 ${updated.enabled.length} 个模型。`
  } catch (err) {
    notice.value = errorMessage(err, '更新模型权限失败，请稍后重试。')
  } finally {
    modelPolicySaving.value = false
  }
}

onMounted(load)
onUnmounted(() => {
  loadSequence += 1
  if (usersSearchTimer) clearTimeout(usersSearchTimer)
})
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">代理站管理</h1>
        <p class="mt-1 text-sm text-slate-500">管理本站用户、品牌与模型权限；余额和计费均由各用户的 Sub2API 账户负责。</p>
      </div>
      <div class="flex flex-wrap gap-2"><button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">刷新</button><button class="rounded-lg border px-4 py-2 text-sm disabled:opacity-50" type="button" :disabled="reconciling" @click="reconcile">{{ reconciling ? '正在核对…' : '核对待结算记录' }}</button></div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700">{{ notice }}</p>
    <p v-if="loading" class="text-sm text-slate-500">正在加载…</p>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="font-semibold">品牌信息</h2>
          <p class="mt-1 text-sm text-slate-500">自定义代理站显示名称和标志。计费身份与域名不可在此修改。</p>
        </div>
        <button class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50" type="button" :disabled="brandingSaving" @click="saveBranding">{{ brandingSaving ? '正在保存…' : '保存品牌信息' }}</button>
      </div>
      <div class="mt-4 grid gap-4 md:grid-cols-3">
        <label class="text-sm font-medium">代理站名称<input v-model="brandingName" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="100" type="text"></label>
        <label class="text-sm font-medium">站点名称<input v-model="brandingSiteName" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="160" type="text"></label>
        <label class="text-sm font-medium">标志图片网址或路径<input v-model="brandingLogo" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="2048" placeholder="/logo.svg 或 https://…" type="text"></label>
      </div>
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="font-semibold">模型权限</h2>
          <p class="mt-1 text-sm text-slate-500">限制此代理站可用的公开卫星模型。停用的模型会从 <code>/v1/models</code> 中隐藏，并在计费或调用上游前被拒绝。</p>
          <p v-if="modelPolicy" class="mt-1 text-xs text-slate-500">已启用 {{ enabledModels.length }} / {{ modelPolicy.catalog.length }} 个公开模型。</p>
        </div>
        <button class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50" type="button" :disabled="modelPolicySaving || !modelPolicy" @click="saveModelPolicy">{{ modelPolicySaving ? '正在保存…' : '保存模型权限' }}</button>
      </div>
      <div v-if="modelPolicy" class="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        <label v-for="model in modelPolicy.catalog" :key="model" class="flex items-center gap-3 rounded-lg border px-3 py-2 text-sm dark:border-slate-700">
          <input v-model="enabledModels" class="h-4 w-4 accent-blue-600" type="checkbox" :value="model">
          <span class="font-mono">{{ model }}</span>
        </label>
      </div>
      <p v-else class="mt-4 text-sm text-slate-500">{{ loadFailed('model access policy') ? '加载模型目录失败，请刷新重试。' : '正在加载模型目录…' }}</p>
    </section>

    <section v-if="context" class="grid gap-4 md:grid-cols-2">
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">代理站</p>
        <p class="mt-2 text-lg font-semibold">{{ context.agent.name }}</p>
        <p class="text-sm text-slate-500">{{ context.agent.domain || '域名待配置' }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">计费方式</p>
        <p class="mt-2 text-lg font-semibold">用户主站直扣</p>
        <p class="text-xs text-slate-500">每个用户使用自己的 Sub2API 余额，代理站不维护共享钱包。</p>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-end justify-between gap-4 border-b p-5 dark:border-slate-700">
        <div>
          <h2 class="font-semibold">代理站用户</h2>
        <p class="mt-1 text-sm text-slate-500">这里只显示已关联到此代理站的用户。停用操作默认只关闭本代理站访问；旧 runtime 兼容实例可能同时同步主站状态。</p>
        </div>
        <label class="w-full text-sm font-medium sm:w-72">
          搜索关联用户
          <input
            v-model="usersSearchInput"
            aria-label="搜索关联用户"
            class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950"
            maxlength="128"
            placeholder="邮箱、姓名或 Sub2API 用户编号"
            type="search"
            @input="scheduleUsersSearch"
          >
        </label>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800">
            <tr><th class="px-5 py-3">用户</th><th class="px-5 py-3">状态 / 访问权限</th><th class="px-5 py-3">Sub2API 实时余额</th></tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.main_user_id" class="border-t dark:border-slate-700">
              <td class="px-5 py-4"><div class="font-medium">{{ user.display_name || user.email || user.main_user_id }}</div><div class="text-xs text-slate-500">{{ user.main_user_id }}</div></td>
              <td class="px-5 py-4">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-full px-2 py-1 text-xs font-medium" :class="user.status === 'active' ? 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300' : 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300'">{{ statusLabel(user.status) }}</span>
                  <button
                    v-if="!isAgentOwner(user)"
                    class="rounded border px-3 py-1 text-xs disabled:opacity-50 dark:border-slate-600"
                    type="button"
                    :disabled="statusUpdatingUser !== ''"
                    @click="updateUserStatus(user)"
                  >{{ statusUpdatingUser === user.main_user_id ? '正在更新…' : user.status === 'active' ? '停用账号' : '启用账号' }}</button>
                  <span v-else class="text-xs text-slate-500">代理站主账户</span>
                </div>
              </td>
              <td class="px-5 py-4"><span v-if="!user.balance_error">{{ money(user.balance_cents) }}</span><span v-else class="text-xs text-amber-600">暂时无法读取</span></td>
            </tr>
            <tr v-if="!loading && users.length === 0"><td class="px-5 py-8 text-center text-slate-500" colspan="3">{{ loadFailed('mapped users') ? '加载关联用户失败，请刷新重试。' : '暂无关联用户。' }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <AgentPagination
      v-if="!loading && usersTotal > 0"
      :total="usersTotal"
      :page="usersPage"
      :page-size="usersPageSize"
      item-label="名用户"
      :page-size-options="[10, 25, 50, 100, 200]"
      @update:page="changeUsersPage"
      @update:page-size="changeUsersPageSize"
    />

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">待结算记录</h2>
        <p class="mt-1 text-sm text-slate-500">待结算记录不会被静默退款；请与主站用量账本核对。</p>
      </div>
      <div v-if="settlements.length === 0" class="p-5 text-sm text-slate-500">{{ loadFailed('pending settlements') ? '加载待结算记录失败，请刷新重试。' : '暂无待结算记录。' }}</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">请求编号</th><th class="px-5 py-3">预留金额</th><th class="px-5 py-3">状态</th><th class="px-5 py-3">说明</th></tr></thead>
          <tbody><tr v-for="item in settlements" :key="item.request_id" class="border-t dark:border-slate-700"><td class="max-w-xs truncate px-5 py-4 font-mono text-xs">{{ item.request_id }}</td><td class="px-5 py-4">{{ money(item.reserved_cents) }}</td><td class="px-5 py-4 text-amber-600">{{ statusLabel(item.status) }}</td><td class="max-w-md px-5 py-4 text-slate-500">{{ errorMessage({ message: item.error }, '等待主站用量记录') }}</td></tr></tbody>
        </table>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">审计记录</h2>
        <p class="mt-1 text-sm text-slate-500">此代理站最近涉及安全的变更记录。系统不会保存密钥或授权请求头。</p>
      </div>
      <div v-if="auditEvents.length === 0" class="p-5 text-sm text-slate-500">{{ loadFailed('audit events') ? '加载审计记录失败，请刷新重试。' : '暂无审计记录。' }}</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">时间</th><th class="px-5 py-3">操作</th><th class="px-5 py-3">操作者</th><th class="px-5 py-3">对象</th><th class="px-5 py-3">请求编号</th></tr></thead>
          <tbody><tr v-for="event in auditEvents" :key="event.id" class="border-t dark:border-slate-700"><td class="whitespace-nowrap px-5 py-4">{{ new Date(event.created_at).toLocaleString('zh-CN') }}</td><td class="px-5 py-4 font-medium">{{ operationLabel(event.operation) }}</td><td class="px-5 py-4">{{ statusLabel(event.actor_type) }}<span class="font-mono text-xs"> · {{ event.actor_id }}</span></td><td class="px-5 py-4">{{ targetLabel(event.target_type) }}<span v-if="event.target_id" class="font-mono text-xs"> · {{ event.target_id }}</span></td><td class="max-w-xs truncate px-5 py-4 font-mono text-xs">{{ event.request_id }}</td></tr></tbody>
        </table>
      </div>
    </section>

    <AgentConfirmDialog
      :open="statusConfirmation !== null"
      :title="statusConfirmation?.status === 'active' ? '停用关联账号' : '重新启用关联账号'"
      :message="statusConfirmationMessage"
      :confirm-label="statusConfirmation?.status === 'active' ? '确认停用' : '确认启用'"
      :destructive="statusConfirmation?.status === 'active'"
      @cancel="statusConfirmation = null"
      @confirm="confirmUserStatusUpdate"
    />
  </main>
</template>
