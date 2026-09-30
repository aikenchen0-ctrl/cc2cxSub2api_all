<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'
import Icon from '@/components/icons/Icon.vue'
import { agentPasskeyAPI } from '@/agent/passkeys'
import AgentInput from '@/components/common/AgentInput.vue'

const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const email = ref('')
const emailInput = ref<InstanceType<typeof AgentInput> | null>(null)
const password = ref('')
const totp = ref('')
const tempToken = ref('')
const error = ref('')
const emailError = ref('')
const passwordError = ref('')
const totpError = ref('')
const showPassword = ref(false)
const passkeyEnabled = ref(false)
const passwordChanged = route.query.passwordChanged === '1'
const mainSiteLoginURL = computed(() => {
  const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') && !route.query.redirect.startsWith('//')
    ? route.query.redirect
    : '/dashboard'
  return `/api/v1/auth/main-site/login?next=${encodeURIComponent(redirect)}`
})

onMounted(async () => {
  if (!agentPasskeyAPI.isSupported()) return
  try {
    const config = await agentPasskeyAPI.config()
    passkeyEnabled.value = config.enabled === true && config.supported_origin === true
  } catch {
    passkeyEnabled.value = false
  }
})

async function loginWithPasskey(): Promise<void> {
  if (session.busy) return
  error.value = ''
  try {
    await session.loginWithPasskey()
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') && !route.query.redirect.startsWith('//') ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (err) {
    if (!(err instanceof DOMException && err.name === 'NotAllowedError')) {
      error.value = errorMessage(err, 'Passkey 登录失败，请重试。')
    }
  }
}

async function submit(): Promise<void> {
  if (session.busy) return
  error.value = ''
  emailError.value = ''
  passwordError.value = ''
  totpError.value = ''
  if (tempToken.value) {
    const code = totp.value.trim()
    if (!code) { totpError.value = '请输入验证器代码。'; return }
    if (!/^\d{6,8}$/.test(code)) { totpError.value = '验证器代码应为 6 至 8 位数字。'; return }
  } else {
    if (!email.value.trim()) { emailError.value = '请输入邮箱地址。'; return }
    if (emailInput.value?.hasTypeMismatch()) { emailError.value = '请输入有效的邮箱地址。'; return }
    if (!password.value) { passwordError.value = '请输入密码。'; return }
  }
  try {
    if (tempToken.value) {
      await session.login2FA(tempToken.value, totp.value.trim())
    } else {
      const response = await session.login(email.value.trim(), password.value)
      if (response.requires_2fa) {
        password.value = ''
        if (!response.temp_token) { error.value = '登录验证信息不完整，请重新登录。'; return }
        tempToken.value = response.temp_token
        return
      }
      if (!session.isAuthenticated) { error.value = '登录响应缺少用户信息，请重试。'; return }
    }
    password.value = ''
    tempToken.value = ''
    totp.value = ''
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (err) {
    error.value = errorMessage(err, '登录失败，请检查账号信息后重试。')
  }
}
</script>

<template>
  <main class="w-full">
    <section class="space-y-6">
      <div class="text-center">
        <div v-if="tempToken" class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="shield" size="lg" /></div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ tempToken ? '双重验证' : '欢迎回来' }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ tempToken ? '输入验证器中显示的动态代码' : '登录当前代理站账户以继续' }}</p>
      </div>

      <div v-if="passwordChanged" role="status" class="flex gap-2 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-200"><Icon name="checkCircle" size="sm" class="mt-0.5 shrink-0" />密码已修改，请使用新密码登录。</div>
      <div v-if="error" role="alert" class="flex gap-2 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"><Icon name="exclamationCircle" size="sm" class="mt-0.5 shrink-0" />{{ error }}</div>

      <form class="space-y-5" novalidate @submit.prevent="submit">
        <template v-if="!tempToken">
          <div>
            <label for="agent-login-email" class="input-label">邮箱地址</label>
            <AgentInput id="agent-login-email" ref="emailInput" v-model="email" type="email" autocomplete="email" autofocus required :disabled="session.busy" placeholder="name@example.com" :error="emailError">
              <template #prefix><Icon name="mail" size="md" /></template>
            </AgentInput>
          </div>
          <div>
            <div class="mb-1.5 flex items-center justify-between">
              <label for="agent-login-password" class="input-label mb-0">密码</label>
              <RouterLink class="text-xs font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400" to="/forgot-password">忘记密码？</RouterLink>
            </div>
            <AgentInput id="agent-login-password" v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="current-password" required :disabled="session.busy" placeholder="请输入密码" :error="passwordError">
              <template #prefix><Icon name="lock" size="md" /></template>
              <template #suffix><button type="button" :aria-label="showPassword ? '隐藏密码' : '显示密码'" class="flex text-gray-400 transition hover:text-gray-600 dark:hover:text-dark-300" :disabled="session.busy" @click="showPassword = !showPassword"><Icon :name="showPassword ? 'eyeOff' : 'eye'" size="md" /></button></template>
            </AgentInput>
          </div>
        </template>
        <div v-else>
          <label for="agent-login-totp" class="input-label">验证器代码</label>
          <AgentInput id="agent-login-totp" v-model="totp" class="text-center font-mono tracking-[0.35em]" inputmode="numeric" autocomplete="one-time-code" minlength="6" maxlength="8" required :disabled="session.busy" placeholder="000000" :error="totpError">
            <template #prefix><Icon name="lock" size="md" /></template>
          </AgentInput>
        </div>
        <button class="btn btn-primary w-full" type="submit" :disabled="session.busy">
          <span v-if="session.busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
          <Icon v-else :name="tempToken ? 'shield' : 'login'" size="md" class="mr-2" />
          {{ session.busy ? '正在登录…' : tempToken ? '验证并登录' : '登录' }}
        </button>
      </form>

      <template v-if="!tempToken">
        <button v-if="passkeyEnabled" data-testid="passkey-login" type="button" class="btn btn-secondary w-full" :disabled="session.busy" @click="loginWithPasskey">
          <Icon name="key" size="md" class="mr-2" />
          使用 Passkey 登录
        </button>
        <div class="flex items-center gap-3" aria-hidden="true">
          <span class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></span>
          <span class="text-xs text-gray-400 dark:text-dark-500">或</span>
          <span class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></span>
        </div>
        <a data-testid="main-site-login" class="btn btn-secondary w-full" :href="mainSiteLoginURL">
          <Icon name="externalLink" size="md" class="mr-2" />
          使用主站账号登录
        </a>
      </template>

      <div class="rounded-xl bg-gray-50 px-4 py-3 text-xs leading-5 text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">浏览器仅保存本站 HttpOnly 会话；主站应用凭据与运行时控制凭据始终保留在服务端。</div>
      <p class="text-center text-sm text-gray-500">还没有账号？ <RouterLink class="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400" to="/register">立即注册</RouterLink></p>
    </section>
  </main>
</template>
