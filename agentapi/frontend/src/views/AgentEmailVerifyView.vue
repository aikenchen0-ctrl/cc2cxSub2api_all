<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { agentAPI, type LoginResponse } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import { useAgentSession } from '@/agent/session'
import Icon from '@/components/icons/Icon.vue'
import AgentInput from '@/components/common/AgentInput.vue'

interface PendingRegistration {
  email: string
  password: string
  username?: string
  affiliate_code?: string
}

const STORAGE_KEY = 'agent_register_data'
const router = useRouter()
const session = useAgentSession()
const pending = ref<PendingRegistration | null>(null)
const verifyCode = ref('')
const error = ref('')
const codeSent = ref(false)
const sending = ref(false)
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const maskedEmail = computed(() => {
  const value = pending.value?.email || ''
  const [name, domain] = value.split('@')
  if (!name || !domain) return value
  return `${name.slice(0, Math.min(2, name.length))}${name.length > 2 ? '***' : ''}@${domain}`
})

function clearTimer(): void {
  if (timer) clearInterval(timer)
  timer = null
}

function startCountdown(seconds: number): void {
  clearTimer()
  countdown.value = Math.max(1, Math.min(3600, seconds || 60))
  timer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) clearTimer()
  }, 1000)
}

async function sendCode(): Promise<void> {
  if (!pending.value || sending.value) return
  sending.value = true
  error.value = ''
  try {
    const result = await agentAPI.auth.sendVerifyCode(pending.value.email)
    codeSent.value = true
    startCountdown(result.countdown)
  } catch (err) {
    error.value = errorMessage(err, '验证码发送失败，请稍后重试。')
  } finally {
    sending.value = false
  }
}

async function submit(): Promise<void> {
  if (!pending.value || session.busy) return
  error.value = ''
  const code = verifyCode.value.trim()
  if (!/^\d{4,8}$/.test(code)) {
    error.value = '请输入 4 至 8 位邮箱验证码。'
    return
  }
  try {
    const response: LoginResponse = await session.register(
      pending.value.email,
      pending.value.password,
      pending.value.username,
      pending.value.affiliate_code,
      code,
    )
    sessionStorage.removeItem(STORAGE_KEY)
    pending.value = null
    if (response.requires_2fa || !session.isAuthenticated) {
      await router.replace('/login')
      return
    }
    await router.replace('/dashboard')
  } catch (err) {
    error.value = errorMessage(err, '邮箱验证或注册失败，请检查验证码后重试。')
  }
}

function back(): void {
  sessionStorage.removeItem(STORAGE_KEY)
  pending.value = null
  void router.replace('/register')
}

onMounted(async () => {
  const raw = sessionStorage.getItem(STORAGE_KEY)
  if (!raw) return
  try {
    const parsed = JSON.parse(raw) as Partial<PendingRegistration>
    if (typeof parsed.email === 'string' && parsed.email.trim() && typeof parsed.password === 'string' && parsed.password) {
      pending.value = {
        email: parsed.email.trim(), password: parsed.password,
        ...(typeof parsed.username === 'string' && parsed.username.trim() ? { username: parsed.username.trim() } : {}),
        ...(typeof parsed.affiliate_code === 'string' && parsed.affiliate_code.trim() ? { affiliate_code: parsed.affiliate_code.trim() } : {}),
      }
      await sendCode()
    }
  } catch {
    sessionStorage.removeItem(STORAGE_KEY)
  }
})

onUnmounted(clearTimer)
</script>

<template>
  <main class="w-full">
    <section class="space-y-6">
      <div class="text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300"><Icon name="mail" size="lg" /></div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">验证邮箱</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">验证码将发送至 <span class="font-medium text-gray-700 dark:text-gray-200">{{ maskedEmail }}</span></p>
      </div>

      <div v-if="!pending" role="alert" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
        <p class="font-medium">注册会话已失效</p><p class="mt-1">请返回注册页面重新填写信息。</p>
      </div>
      <div v-if="error" role="alert" class="flex gap-2 rounded-xl border border-red-200 bg-red-50 p-3 text-sm leading-5 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"><Icon name="exclamationCircle" size="sm" class="mt-0.5 shrink-0" />{{ error }}</div>
      <div v-if="codeSent" role="status" class="flex gap-2 rounded-xl border border-green-200 bg-green-50 p-3 text-sm leading-5 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-300"><Icon name="checkCircle" size="sm" class="mt-0.5 shrink-0" />验证码已发送，请在 15 分钟内完成验证。</div>

      <form v-if="pending" class="space-y-5" @submit.prevent="submit">
        <div>
          <label for="agent-email-code" class="input-label text-center">邮箱验证码</label>
          <AgentInput id="agent-email-code" v-model="verifyCode" class="py-3 text-center font-mono text-xl tracking-[0.5em]" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="8" required :disabled="session.busy" placeholder="000000" hint="请输入邮件中的验证码" />
        </div>
        <button class="btn btn-primary w-full" type="submit" :disabled="session.busy || !verifyCode.trim()"><span v-if="session.busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span><Icon v-else name="checkCircle" size="md" class="mr-2" />{{ session.busy ? '正在验证…' : '验证并创建账号' }}</button>
        <div class="text-center">
          <button v-if="countdown > 0" type="button" disabled class="text-sm text-gray-400">{{ countdown }} 秒后可重新发送</button>
          <button v-else type="button" class="text-sm font-medium text-primary-600 hover:text-primary-500 disabled:opacity-50 dark:text-primary-400" :disabled="sending" @click="sendCode">{{ sending ? '正在发送…' : '重新发送验证码' }}</button>
        </div>
      </form>

      <button type="button" class="mx-auto flex items-center gap-2 text-sm text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-gray-200" @click="back"><Icon name="arrowLeft" size="sm" />返回注册</button>
    </section>
  </main>
</template>
