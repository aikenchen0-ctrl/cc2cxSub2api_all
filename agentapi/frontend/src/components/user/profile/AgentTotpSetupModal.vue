<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { agentAPI, type AgentTOTPSetupResponse } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentInput from '@/components/common/AgentInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'

const emit = defineEmits<{ close: []; success: [] }>()
const step = ref<0 | 1 | 2>(0)
const methodLoading = ref(true)
const method = ref<'email' | 'password'>('password')
const emailCode = ref('')
const password = ref('')
const totpCode = ref('')
const setup = ref<AgentTOTPSetupResponse | null>(null)
const qrCodeDataURL = ref('')
const loading = ref(false)
const sendingCode = ref(false)
const cooldown = ref(0)
const error = ref('')
const notice = ref('')
let cooldownTimer: ReturnType<typeof setInterval> | undefined

const canVerifyIdentity = computed(() => method.value === 'email' ? /^\d{6}$/.test(emailCode.value) : password.value.length > 0)
const stepHint = computed(() => step.value === 0
  ? (method.value === 'email' ? '先验证主站账号绑定邮箱。' : '先验证当前主站账号密码。')
  : step.value === 1 ? '扫描二维码或手动输入密钥。' : '输入身份验证器中的六位动态验证码。')

watch(() => setup.value?.qr_code_url, async (value) => {
  qrCodeDataURL.value = value ? await QRCode.toDataURL(value, { width: 200, margin: 2 }) : ''
})

async function loadMethod(): Promise<void> {
  try {
    method.value = (await agentAPI.profile.totp.getVerificationMethod()).method
  } catch (err) {
    error.value = errorMessage(err, '加载验证方式失败，请关闭后重试。')
  } finally {
    methodLoading.value = false
  }
}

async function sendCode(): Promise<void> {
  if (sendingCode.value || cooldown.value > 0) return
  sendingCode.value = true
  error.value = ''
  try {
    await agentAPI.profile.totp.sendVerifyCode()
    notice.value = '验证码已发送，请查看主站账号邮箱。'
    cooldown.value = 60
    cooldownTimer = setInterval(() => {
      cooldown.value -= 1
      if (cooldown.value <= 0 && cooldownTimer) {
        clearInterval(cooldownTimer)
        cooldownTimer = undefined
      }
    }, 1000)
  } catch (err) {
    error.value = errorMessage(err, '发送验证码失败，请稍后重试。')
  } finally {
    sendingCode.value = false
  }
}

async function initiateSetup(): Promise<void> {
  if (!canVerifyIdentity.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    setup.value = await agentAPI.profile.totp.initiateSetup(method.value === 'email'
      ? { email_code: emailCode.value }
      : { password: password.value })
    password.value = ''
    emailCode.value = ''
    step.value = 1
  } catch (err) {
    error.value = errorMessage(err, '验证失败，无法开始配置两步验证。')
  } finally {
    loading.value = false
  }
}

async function copySecret(): Promise<void> {
  if (!setup.value) return
  try {
    await navigator.clipboard.writeText(setup.value.secret)
    notice.value = '密钥已复制。'
  } catch {
    error.value = '复制失败，请手动选择密钥。'
  }
}

async function enable(): Promise<void> {
  if (!setup.value || !/^\d{6}$/.test(totpCode.value) || loading.value) return
  loading.value = true
  error.value = ''
  try {
    await agentAPI.profile.totp.enable(totpCode.value, setup.value.setup_token)
    emit('success')
  } catch (err) {
    totpCode.value = ''
    error.value = errorMessage(err, '动态验证码校验失败，请重试。')
  } finally {
    loading.value = false
  }
}

function close(): void { emit('close') }

onMounted(loadMethod)
onUnmounted(() => {
  if (cooldownTimer) clearInterval(cooldownTimer)
  password.value = ''
  setup.value = null
  qrCodeDataURL.value = ''
})
</script>

<template>
  <BaseDialog
    :show="true"
    title="启用两步验证"
    width="narrow"
    :close-on-escape="!loading"
    :close-on-click-outside="!loading"
    :show-close-button="!loading"
    @close="close"
  >
    <div data-testid="totp-setup-modal">
      <p class="text-center text-sm text-gray-500 dark:text-gray-400">{{ stepHint }}</p>
      <p v-if="error" role="alert" class="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="mt-4 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>

      <div v-if="step === 0" class="mt-6 space-y-5">
        <div v-if="methodLoading" role="status" class="py-8 text-center text-sm text-gray-500">正在加载验证方式…</div>
        <template v-else>
          <div v-if="method === 'email'">
            <label for="totp-email-code" class="input-label">邮箱验证码</label>
            <div class="flex gap-2">
              <div class="min-w-0 flex-1"><AgentInput id="totp-email-code" v-model="emailCode" maxlength="6" inputmode="numeric" autocomplete="one-time-code" placeholder="请输入 6 位验证码" /></div>
              <button type="button" class="btn btn-secondary whitespace-nowrap" :disabled="sendingCode || cooldown > 0" @click="sendCode">{{ cooldown > 0 ? `${cooldown}s` : sendingCode ? '发送中…' : '发送验证码' }}</button>
            </div>
          </div>
          <div v-else>
            <label for="totp-password" class="input-label">当前密码</label>
            <AgentInput id="totp-password" v-model="password" type="password" autocomplete="current-password" placeholder="请输入当前主站账号密码" />
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button type="button" class="btn btn-secondary" @click="close">取消</button>
            <button type="button" class="btn btn-primary" :disabled="!canVerifyIdentity || loading" @click="initiateSetup">{{ loading ? '正在验证…' : '下一步' }}</button>
          </div>
        </template>
      </div>

      <div v-else-if="step === 1" class="mt-6 space-y-5">
        <div v-if="qrCodeDataURL" class="flex justify-center"><div class="rounded-lg border border-gray-200 bg-white p-4"><img :src="qrCodeDataURL" alt="两步验证二维码" class="h-48 w-48" /></div></div>
        <div v-if="setup" class="text-center">
          <p class="mb-2 text-sm text-gray-500 dark:text-gray-400">无法扫码时手动输入密钥</p>
          <div class="flex items-center justify-center gap-2"><code class="break-all rounded bg-gray-100 px-3 py-2 font-mono text-sm dark:bg-dark-700">{{ setup.secret }}</code><button type="button" class="btn btn-secondary btn-sm" @click="copySecret">复制</button></div>
        </div>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="close">取消</button><button type="button" class="btn btn-primary" :disabled="!setup" @click="step = 2">下一步</button></div>
      </div>

      <form v-else class="mt-6 space-y-5" @submit.prevent="enable">
        <div><label for="totp-auth-code" class="input-label">动态验证码</label><AgentInput id="totp-auth-code" v-model="totpCode" class="text-center font-mono text-lg tracking-[0.35em]" maxlength="6" inputmode="numeric" autocomplete="one-time-code" placeholder="000000" /></div>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="step = 1">上一步</button><button type="submit" class="btn btn-primary" :disabled="loading || !/^\d{6}$/.test(totpCode)">{{ loading ? '正在校验…' : '确认启用' }}</button></div>
      </form>
    </div>
  </BaseDialog>
</template>
