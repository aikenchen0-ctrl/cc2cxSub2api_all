<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI } from '@/agent/api'
import { useRoute, useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage, type AgentAPIError } from '@/agent/client'
import Icon from '@/components/icons/Icon.vue'
import AgentInput from '@/components/common/AgentInput.vue'

const router = useRouter()
const route = useRoute()
const session = useAgentSession()
const email = ref('')
const emailInput = ref<InstanceType<typeof AgentInput> | null>(null)
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const tempToken = ref('')
const totp = ref('')
const loginRequired = ref(false)
const availability = ref<'loading' | 'enabled' | 'disabled' | 'error'>('loading')
const emailVerifyEnabled = ref(false)
const emailError = ref('')
const passwordError = ref('')
const confirmPasswordError = ref('')
const totpError = ref('')
const showPassword = ref(false)
const showConfirmPassword = ref(false)
const affiliateCode = ref('')

async function loadAvailability(): Promise<void> {
  availability.value = 'loading'
  try {
    const settings = await agentAPI.getPublicSettings()
    availability.value = settings.registration_enabled === true ? 'enabled' : 'disabled'
    emailVerifyEnabled.value = settings.email_verify_enabled === true
  } catch {
    availability.value = 'error'
  }
}
onMounted(() => {
  const raw = Array.isArray(route.query.aff) ? route.query.aff[0] : route.query.aff
  affiliateCode.value = String(raw || '').trim().toUpperCase().slice(0, 32)
  void loadAvailability()
})

async function submit(): Promise<void> {
  if (session.busy || loginRequired.value) return
  if (!tempToken.value && availability.value !== 'enabled') return
  error.value = ''
  emailError.value = ''
  passwordError.value = ''
  confirmPasswordError.value = ''
  totpError.value = ''
  if (tempToken.value) {
    if (!/^\d{6,8}$/.test(totp.value.trim())) { totpError.value = '请输入 6 至 8 位验证器代码。'; return }
    try {
      await session.login2FA(tempToken.value, totp.value.trim())
      tempToken.value = ''
      await router.replace('/dashboard')
    } catch (err) { error.value = errorMessage(err, '验证失败，请重试。') }
    return
  }
  if (!email.value.trim()) { emailError.value = '请输入邮箱地址。'; return }
  if (emailInput.value?.hasTypeMismatch()) { emailError.value = '请输入有效的邮箱地址。'; return }
  if (!password.value) { passwordError.value = '请输入密码。'; return }
  if (password.value.length < 6) { passwordError.value = '密码至少需要 6 个字符。'; return }
  if (!confirmPassword.value) { confirmPasswordError.value = '请再次输入密码。'; return }
  if (password.value !== confirmPassword.value) { confirmPasswordError.value = '两次输入的密码不一致。'; return }
  try {
    if (emailVerifyEnabled.value) {
      sessionStorage.setItem('agent_register_data', JSON.stringify({
        email: email.value.trim(),
        password: password.value,
        username: username.value.trim() || undefined,
        affiliate_code: affiliateCode.value || undefined,
      }))
      password.value = ''
      confirmPassword.value = ''
      await router.push('/email-verify')
      return
    }
    const response = await session.register(email.value.trim(), password.value, username.value.trim() || undefined, affiliateCode.value || undefined)
    password.value = ''
    confirmPassword.value = ''
    if (response.requires_2fa) {
      if (!response.temp_token) { loginRequired.value = true; error.value = '账号已创建，请前往登录页面完成验证。'; return }
      tempToken.value = response.temp_token
      return
    }
    if (!session.isAuthenticated) { loginRequired.value = true; error.value = '账号已创建，请前往登录页面登录。'; return }
    await router.replace('/dashboard')
  } catch (err) {
    const code = (err as Partial<AgentAPIError> | null)?.code
    if (code === 'REGISTERED_LOGIN_REQUIRED' || code === 'USER_MAPPING_FAILED') {
      loginRequired.value = true
      password.value = ''
      confirmPassword.value = ''
      error.value = code === 'USER_MAPPING_FAILED'
        ? '主站账号已创建，本站关联尚未完成。请前往登录页面尝试恢复；若仍失败，请联系站长，不要重复注册。'
        : '账号已创建，自动登录暂未完成。请前往登录页面登录，不要重复注册。'
      return
    }
    error.value = errorMessage(err, '注册失败，请检查填写内容后重试。')
  }
}
</script>

<template>
  <main class="w-full">
    <section class="space-y-6">
      <div class="text-center">
        <div v-if="tempToken" class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="shield" size="lg" /></div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ tempToken ? '完成双重验证' : '创建账号' }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ tempToken ? '输入验证器动态代码以完成登录' : '注册主站正式账号并关联到当前代理站' }}</p>
      </div>

      <div v-if="error" role="alert" class="flex gap-2 rounded-xl border border-red-200 bg-red-50 p-3 text-sm leading-5 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"><Icon name="exclamationCircle" size="sm" class="mt-0.5 shrink-0" />{{ error }}</div>
      <div v-if="availability === 'loading'" role="status" class="flex items-center justify-center gap-2 rounded-xl bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-900/60"><span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600"></span>正在确认本站注册状态…</div>
      <div v-else-if="availability === 'disabled'" role="status" class="flex gap-2 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200"><Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />本站暂未开放注册，请联系站长。已有账号仍可前往登录。</div>
      <div v-else-if="availability === 'error'" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
        <p>无法确认本站注册状态，请稍后重试。</p>
        <button type="button" class="mt-2 font-medium text-primary-600 hover:underline" @click="loadAvailability">重新检查</button>
      </div>

      <div v-if="availability === 'enabled' && emailVerifyEnabled && !tempToken" class="flex gap-2 rounded-xl border border-blue-200 bg-blue-50 p-3 text-sm leading-5 text-blue-700 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-200">
        <Icon name="mail" size="sm" class="mt-0.5 shrink-0" />提交后将向该邮箱发送主站验证码，验证成功后才会创建账号。
      </div>

      <form v-if="!loginRequired && (availability === 'enabled' || tempToken)" class="space-y-5" novalidate @submit.prevent="submit">
        <template v-if="!tempToken">
          <div>
            <label for="agent-register-email" class="input-label">邮箱地址</label>
            <AgentInput id="agent-register-email" ref="emailInput" v-model="email" type="email" autocomplete="email" autofocus required :disabled="session.busy" placeholder="name@example.com" :error="emailError"><template #prefix><Icon name="mail" size="md" /></template></AgentInput>
          </div>
          <div>
            <label for="agent-register-name" class="input-label">显示名称 <span class="font-normal text-gray-400">（选填）</span></label>
            <AgentInput id="agent-register-name" v-model="username" type="text" maxlength="100" autocomplete="nickname" :disabled="session.busy" placeholder="如何称呼你"><template #prefix><Icon name="user" size="md" /></template></AgentInput>
          </div>
          <div v-if="affiliateCode" class="rounded-xl border border-primary-200 bg-primary-50 px-4 py-3 text-sm text-primary-800 dark:border-primary-900/40 dark:bg-primary-900/20 dark:text-primary-200">
            <p class="font-medium">已应用邀请码</p>
            <p class="mt-1 font-mono">{{ affiliateCode }}</p>
          </div>
          <div>
            <label for="agent-register-password" class="input-label">密码</label>
            <AgentInput id="agent-register-password" v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" required :disabled="session.busy" placeholder="至少 6 个字符" :error="passwordError"><template #prefix><Icon name="lock" size="md" /></template><template #suffix><button type="button" :aria-label="showPassword ? '隐藏密码' : '显示密码'" class="flex text-gray-400 hover:text-gray-600" :disabled="session.busy" @click="showPassword = !showPassword"><Icon :name="showPassword ? 'eyeOff' : 'eye'" size="md" /></button></template></AgentInput>
          </div>
          <div>
            <label for="agent-register-confirm" class="input-label">确认密码</label>
            <AgentInput id="agent-register-confirm" v-model="confirmPassword" :type="showConfirmPassword ? 'text' : 'password'" autocomplete="new-password" required :disabled="session.busy" placeholder="再次输入密码" :error="confirmPasswordError"><template #prefix><Icon name="lock" size="md" /></template><template #suffix><button type="button" :aria-label="showConfirmPassword ? '隐藏确认密码' : '显示确认密码'" class="flex text-gray-400 hover:text-gray-600" :disabled="session.busy" @click="showConfirmPassword = !showConfirmPassword"><Icon :name="showConfirmPassword ? 'eyeOff' : 'eye'" size="md" /></button></template></AgentInput>
          </div>
        </template>
        <div v-else>
          <label for="agent-register-totp" class="input-label">验证器代码</label>
          <AgentInput id="agent-register-totp" v-model="totp" class="text-center font-mono tracking-[0.35em]" inputmode="numeric" autocomplete="one-time-code" maxlength="8" required :disabled="session.busy" placeholder="000000" :error="totpError"><template #prefix><Icon name="lock" size="md" /></template></AgentInput>
        </div>
        <button class="btn btn-primary w-full" type="submit" :disabled="session.busy">
          <span v-if="session.busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
          <Icon v-else :name="tempToken ? 'shield' : 'userPlus'" size="md" class="mr-2" />
          {{ session.busy ? '正在处理…' : tempToken ? '验证并登录' : '注册账号' }}
        </button>
      </form>

      <div class="rounded-xl bg-gray-50 px-4 py-3 text-xs leading-5 text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">提交注册后，账号先由 Sub2API 主站创建，再由服务端记录本站归属。遇到“账号已创建”提示时请直接登录，不要重复注册。</div>
      <p class="text-center text-sm text-gray-500">已有账号？ <RouterLink class="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400" to="/login">登录</RouterLink></p>
    </section>
  </main>
</template>
