<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { agentAPI } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentInput from '@/components/common/AgentInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'

const emit = defineEmits<{ close: []; success: [] }>()
const methodLoading = ref(true)
const method = ref<'email' | 'password'>('password')
const emailCode = ref('')
const password = ref('')
const loading = ref(false)
const sendingCode = ref(false)
const cooldown = ref(0)
const error = ref('')
const notice = ref('')
let cooldownTimer: ReturnType<typeof setInterval> | undefined
const canSubmit = computed(() => method.value === 'email' ? /^\d{6}$/.test(emailCode.value) : password.value.length > 0)

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
    notice.value = '验证码已发送。'
    cooldown.value = 60
    cooldownTimer = setInterval(() => {
      cooldown.value -= 1
      if (cooldown.value <= 0 && cooldownTimer) { clearInterval(cooldownTimer); cooldownTimer = undefined }
    }, 1000)
  } catch (err) {
    error.value = errorMessage(err, '发送验证码失败，请稍后重试。')
  } finally {
    sendingCode.value = false
  }
}

async function disable(): Promise<void> {
  if (!canSubmit.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    await agentAPI.profile.totp.disable(method.value === 'email' ? { email_code: emailCode.value } : { password: password.value })
    emit('success')
  } catch (err) {
    error.value = errorMessage(err, '停用两步验证失败，请重试。')
  } finally {
    loading.value = false
  }
}

onMounted(loadMethod)
onUnmounted(() => { if (cooldownTimer) clearInterval(cooldownTimer); password.value = '' })
</script>

<template>
  <BaseDialog
    :show="true"
    title="停用两步验证"
    width="narrow"
    :close-on-escape="!loading"
    :close-on-click-outside="!loading"
    :show-close-button="!loading"
    @close="$emit('close')"
  >
    <div data-testid="totp-disable-dialog">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-red-100 text-xl text-red-600 dark:bg-red-900/30">!</div>
      <p class="mt-4 text-center text-sm text-gray-500 dark:text-gray-400">停用后，仅凭密码即可登录主站账号，账号安全性会降低。</p>
      <p v-if="error" role="alert" class="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="mt-4 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>
      <div v-if="methodLoading" role="status" class="py-8 text-center text-sm text-gray-500">正在加载验证方式…</div>
      <form v-else class="mt-6 space-y-5" @submit.prevent="disable">
        <div v-if="method === 'email'">
          <label for="totp-disable-email-code" class="input-label">邮箱验证码</label>
          <div class="flex gap-2"><div class="min-w-0 flex-1"><AgentInput id="totp-disable-email-code" v-model="emailCode" maxlength="6" inputmode="numeric" autocomplete="one-time-code" placeholder="请输入 6 位验证码" /></div><button type="button" class="btn btn-secondary whitespace-nowrap" :disabled="sendingCode || cooldown > 0" @click="sendCode">{{ cooldown > 0 ? `${cooldown}s` : sendingCode ? '发送中…' : '发送验证码' }}</button></div>
        </div>
        <div v-else><label for="totp-disable-password" class="input-label">当前密码</label><AgentInput id="totp-disable-password" v-model="password" type="password" autocomplete="current-password" placeholder="请输入当前主站账号密码" /></div>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="$emit('close')">取消</button><button type="submit" class="btn btn-danger" :disabled="loading || !canSubmit">{{ loading ? '正在停用…' : '确认停用' }}</button></div>
      </form>
    </div>
  </BaseDialog>
</template>
