<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  agentAPI,
  type AgentIdentityBinding,
  type AgentIdentityProvider,
  type AgentProfile,
} from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import { useAgentSession } from '@/agent/session'

const props = defineProps<{ profile: AgentProfile }>()
const emit = defineEmits<{ updated: [profile: AgentProfile] }>()
const router = useRouter()
const auth = useAgentSession()
const loading = ref(true)
const error = ref('')
const notice = ref('')
const items = ref<AgentIdentityBinding[]>([])
const oauthBindingSupported = ref(false)
const sendingCode = ref(false)
const bindingEmail = ref(false)
const unbindingProvider = ref<Exclude<AgentIdentityProvider, 'email'> | null>(null)
const unbindTarget = ref<AgentIdentityBinding | null>(null)
const bindingProvider = ref<Exclude<AgentIdentityProvider, 'email'> | null>(null)
const emailForm = reactive({ email: props.profile.email || '', verifyCode: '', password: '' })

const providerLabels: Record<AgentIdentityProvider, string> = {
  email: '邮箱',
  linuxdo: 'LinuxDo',
  oidc: 'OIDC',
  wechat: '微信',
  dingtalk: '钉钉',
}

const emailBinding = computed(() => items.value.find((item) => item.provider === 'email'))

function providerInitial(provider: AgentIdentityProvider): string {
  return provider === 'email' ? 'E' : provider === 'linuxdo' ? 'L' : provider === 'oidc' ? 'O' : provider === 'wechat' ? 'W' : 'D'
}

function providerIconClass(provider: AgentIdentityProvider): string {
  if (provider === 'linuxdo') return 'bg-orange-100 text-orange-600 dark:bg-orange-900/20 dark:text-orange-300'
  if (provider === 'oidc') return 'bg-sky-100 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300'
  if (provider === 'wechat') return 'bg-green-100 text-green-600 dark:bg-green-900/20 dark:text-green-300'
  if (provider === 'dingtalk') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/20 dark:text-blue-300'
  return 'bg-primary-100 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
}

async function loadBindings(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const response = await agentAPI.profile.bindings.get()
    items.value = response.items
    oauthBindingSupported.value = response.oauth_binding_supported
  } catch (err) {
    error.value = errorMessage(err, '加载登录方式失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function validEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)
}

async function sendEmailCode(): Promise<void> {
  const email = emailForm.email.trim()
  if (!validEmail(email)) {
    error.value = '请输入有效的邮箱地址。'
    return
  }
  sendingCode.value = true
  error.value = ''
  notice.value = ''
  try {
    await agentAPI.profile.bindings.sendEmailCode(email)
    notice.value = `验证码已发送至 ${email}。`
  } catch (err) {
    error.value = errorMessage(err, '发送验证码失败，请稍后重试。')
  } finally {
    sendingCode.value = false
  }
}

async function submitEmailBinding(): Promise<void> {
  const email = emailForm.email.trim()
  if (!validEmail(email) || !/^\d{6}$/.test(emailForm.verifyCode) || !emailForm.password) {
    error.value = '请填写有效邮箱、六位验证码和当前密码。'
    return
  }
  bindingEmail.value = true
  error.value = ''
  notice.value = ''
  const replacingBoundEmail = emailBinding.value?.bound === true
  try {
    const response = await agentAPI.profile.bindings.bindEmail({
      email,
      verify_code: emailForm.verifyCode,
      password: emailForm.password,
    })
    items.value = response.items
    oauthBindingSupported.value = response.oauth_binding_supported
    emailForm.verifyCode = ''
    emailForm.password = ''
    notice.value = replacingBoundEmail ? '邮箱登录方式已更新。' : '邮箱登录方式已绑定。'
    if (response.profile) emit('updated', response.profile)
  } catch (err) {
    error.value = errorMessage(err, '更新邮箱登录方式失败，请稍后重试。')
  } finally {
    bindingEmail.value = false
  }
}

function requestUnbind(item: AgentIdentityBinding): void {
  if (item.provider === 'email' || !item.can_unbind) return
  unbindTarget.value = item
}

async function confirmUnbind(): Promise<void> {
  const item = unbindTarget.value
  if (!item || item.provider === 'email' || !item.can_unbind || unbindingProvider.value) return
  const provider = item.provider
  const label = providerLabels[provider]
  unbindingProvider.value = provider
  unbindTarget.value = null
  error.value = ''
  try {
    await agentAPI.profile.bindings.unbind(provider)
    auth.clear()
    await router.replace({ path: '/login', query: { identityChanged: '1' } })
  } catch (err) {
    error.value = errorMessage(err, `解除 ${label} 登录方式失败，请稍后重试。`)
  } finally {
    unbindingProvider.value = null
  }
}

async function startOAuthBinding(item: AgentIdentityBinding): Promise<void> {
  if (item.provider === 'email' || !item.can_bind || !oauthBindingSupported.value) return
  const provider = item.provider
  bindingProvider.value = provider
  error.value = ''
  notice.value = ''
  try {
    const start = await agentAPI.profile.bindings.start(provider)
    if (start.method !== 'GET' || !start.authorize_url) throw new Error('invalid binding start response')
    window.location.assign(start.authorize_url)
  } catch (err) {
    error.value = errorMessage(err, `启动 ${providerLabels[provider]} 绑定失败，请稍后重试。`)
    bindingProvider.value = null
  }
}

onMounted(loadBindings)
</script>

<template>
  <section data-testid="profile-bindings-card" class="card overflow-hidden">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">登录方式</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">管理与 Sub2API 主站账号关联的邮箱和第三方身份。</p>
    </div>

    <div class="px-6 pt-5">
      <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>
    </div>

    <div v-if="loading" class="flex items-center justify-center px-6 py-10" role="status">
      <span class="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-primary-500"></span>
      <span class="sr-only">正在加载登录方式</span>
    </div>
    <div v-else-if="items.length === 0" class="flex items-center justify-between gap-4 px-6 py-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">暂时无法读取登录方式。</p>
      <button type="button" class="btn btn-secondary" @click="loadBindings">重新加载</button>
    </div>
    <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
      <div v-for="item in items" :key="item.provider" :data-testid="`profile-binding-${item.provider}`" class="px-6 py-5">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div class="flex min-w-0 flex-1 items-start gap-4">
            <div :class="providerIconClass(item.provider)" class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-sm font-semibold" aria-hidden="true">
              {{ providerInitial(item.provider) }}
            </div>
            <div class="min-w-0 flex-1 space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="font-medium text-gray-900 dark:text-white">{{ providerLabels[item.provider] }}</h3>
                <AgentStatusBadge :data-testid="`profile-binding-${item.provider}-status`" :status="item.bound ? 'bound' : 'unbound'" :label="item.bound ? '已绑定' : '未绑定'" />
              </div>
              <p v-if="item.provider === 'email' && profile.email" class="break-all text-sm text-gray-600 dark:text-gray-300">{{ profile.email }}</p>
              <p v-if="item.display_name" class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ item.display_name }}</p>
              <p v-if="item.subject_hint" class="text-sm text-gray-500 dark:text-gray-400">{{ item.subject_hint }}</p>
              <p v-if="item.bound_count > 1" class="text-sm text-gray-500 dark:text-gray-400">已绑定 {{ item.bound_count }} 个身份</p>
              <p v-if="item.note" class="text-sm text-gray-500 dark:text-gray-400">{{ item.note }}</p>
              <p v-if="item.provider !== 'email' && !item.bound && item.can_bind && !oauthBindingSupported" data-testid="profile-binding-oauth-pending" class="text-sm text-amber-700 dark:text-amber-300">
                安全绑定入口接入中；当前不会跳转主站或暴露主站令牌。
              </p>
            </div>
          </div>
          <div v-if="item.provider !== 'email'" class="flex shrink-0 items-center gap-2">
            <button
              v-if="!item.bound && item.can_bind && oauthBindingSupported"
              :data-testid="`profile-binding-${item.provider}-bind`"
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="bindingProvider === item.provider"
              @click="startOAuthBinding(item)"
            >
              {{ bindingProvider === item.provider ? '正在跳转…' : '绑定' }}
            </button>
            <button
              v-if="item.can_unbind"
              :data-testid="`profile-binding-${item.provider}-unbind`"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="unbindingProvider === item.provider"
              @click="requestUnbind(item)"
            >
              {{ unbindingProvider === item.provider ? '正在解除…' : '解除绑定' }}
            </button>
          </div>
        </div>

        <div v-if="item.provider === 'email'" data-testid="profile-binding-email-form" class="mt-5 grid gap-3 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 dark:border-dark-700 dark:bg-dark-900/30 sm:grid-cols-[minmax(0,1.4fr)_auto]">
          <AgentInput v-model.trim="emailForm.email" data-testid="profile-binding-email-input" type="email" placeholder="邮箱地址" :disabled="sendingCode || bindingEmail" />
          <button data-testid="profile-binding-email-send-code" type="button" class="btn btn-secondary btn-sm" :disabled="sendingCode || bindingEmail" @click="sendEmailCode">{{ sendingCode ? '正在发送…' : '发送验证码' }}</button>
          <AgentInput v-model.trim="emailForm.verifyCode" data-testid="profile-binding-email-code-input" type="text" inputmode="numeric" maxlength="6" placeholder="六位验证码" :disabled="bindingEmail" />
          <AgentInput v-model="emailForm.password" data-testid="profile-binding-email-password-input" type="password" autocomplete="current-password" placeholder="当前密码" :disabled="bindingEmail" />
          <button data-testid="profile-binding-email-submit" type="button" class="btn btn-primary btn-sm sm:col-span-2" :disabled="bindingEmail" @click="submitEmailBinding">{{ bindingEmail ? '正在提交…' : emailBinding?.bound ? '确认更换邮箱' : '确认绑定邮箱' }}</button>
        </div>
      </div>
    </div>
    <AgentConfirmDialog
      :open="unbindTarget !== null"
      title="解除登录方式"
      :message="unbindTarget ? `确定解除 ${providerLabels[unbindTarget.provider]} 登录方式吗？解除后主站会撤销现有登录令牌，需要重新登录。` : ''"
      confirm-label="解除绑定"
      destructive
      @cancel="unbindTarget = null"
      @confirm="confirmUnbind"
    />
  </section>
</template>
