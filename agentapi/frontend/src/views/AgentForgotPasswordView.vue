<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type PasswordRecoveryConfig } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AgentCaptchaChallenge from '@/components/auth/AgentCaptchaChallenge.vue'
import AgentInput from '@/components/common/AgentInput.vue'

const settings = ref<PasswordRecoveryConfig | null>(null)
const email = ref('')
const emailInput = ref<InstanceType<typeof AgentInput> | null>(null)
const busy = ref(false)
const success = ref(false)
const error = ref('')
const captchaError = ref('')
const turnstileToken = ref('')
const tencentTicket = ref('')
const tencentRandstr = ref('')
const captcha = ref<InstanceType<typeof AgentCaptchaChallenge> | null>(null)
const actionCaptcha = computed(() => Boolean(settings.value?.tencent_captcha_enabled || settings.value?.aliyun_captcha_enabled))

onMounted(async () => {
  try { settings.value = await agentAPI.auth.passwordRecoveryConfig() }
  catch (reason) { error.value = errorMessage(reason, '无法读取密码找回配置。') }
})

function onVerify(token: string, randstr: string): void {
  captchaError.value = ''
  if (settings.value?.tencent_captcha_enabled) { tencentTicket.value = token; tencentRandstr.value = randstr }
  else turnstileToken.value = token
}

async function submit(): Promise<void> {
  if (busy.value || success.value) return
  error.value = ''
  captchaError.value = ''
  const normalizedEmail = email.value.trim()
  if (!normalizedEmail || emailInput.value?.hasTypeMismatch()) { error.value = '请输入有效的邮箱地址。'; return }
  if (!settings.value?.password_reset_enabled) { error.value = '当前站点尚未启用密码找回。'; return }
  busy.value = true
  try {
    if (actionCaptcha.value) {
      const proof = await captcha.value?.verifyAction()
      if (!proof) { captchaError.value = '请先完成人机验证。'; return }
      if (settings.value?.tencent_captcha_enabled) { tencentTicket.value = proof.token; tencentRandstr.value = proof.randstr }
      else turnstileToken.value = proof.token
    } else if (settings.value.turnstile_enabled && !turnstileToken.value) {
      captchaError.value = '请先完成人机验证。'
      return
    }
    await agentAPI.auth.forgotPassword({
      email: normalizedEmail,
      ...(turnstileToken.value ? { turnstile_token: turnstileToken.value } : {}),
      ...(tencentTicket.value ? { tencent_captcha_ticket: tencentTicket.value, tencent_captcha_randstr: tencentRandstr.value } : {}),
    })
    success.value = true
  } catch (reason) {
    error.value = errorMessage(reason, '发送重置邮件失败，请稍后重试。')
    captcha.value?.reset()
    turnstileToken.value = ''; tencentTicket.value = ''; tencentRandstr.value = ''
  } finally { busy.value = false }
}
</script>

<template>
  <AuthLayout>
    <main class="w-full">
      <section class="space-y-6">
        <div class="text-center">
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">找回密码</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">输入注册邮箱，我们会发送一次性重置链接</p>
        </div>
        <div v-if="success" role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 p-5 text-center dark:border-emerald-500/30 dark:bg-emerald-500/10">
          <Icon name="checkCircle" size="lg" class="mx-auto text-emerald-600" />
          <h2 class="mt-3 font-semibold text-emerald-800 dark:text-emerald-200">邮件已提交发送</h2>
          <p class="mt-2 text-sm text-emerald-700 dark:text-emerald-300">如果该邮箱已注册，稍后会收到密码重置邮件。</p>
        </div>
        <form v-else class="space-y-5" novalidate @submit.prevent="submit">
          <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">{{ error }}</div>
          <div>
            <label for="forgot-email" class="input-label">邮箱地址</label>
            <AgentInput id="forgot-email" ref="emailInput" v-model="email" type="email" autocomplete="email" required :disabled="busy" placeholder="name@example.com"><template #prefix><Icon name="mail" size="md" /></template></AgentInput>
          </div>
          <AgentCaptchaChallenge v-if="settings" ref="captcha"
            :turnstile-enabled="Boolean(settings.turnstile_enabled)" :turnstile-site-key="settings.turnstile_site_key || ''"
            :tencent-enabled="Boolean(settings.tencent_captcha_enabled)" :tencent-app-id="settings.tencent_captcha_app_id || ''" :tencent-region="settings.tencent_captcha_region || 'cn'"
            :aliyun-enabled="Boolean(settings.aliyun_captcha_enabled)" :aliyun-scene-id="settings.aliyun_captcha_scene_id || ''" :aliyun-prefix="settings.aliyun_captcha_prefix || ''" :aliyun-region="settings.aliyun_captcha_region || 'cn'"
            @verify="onVerify" @expire="turnstileToken = ''" @error="captchaError = '安全验证加载失败，请刷新后重试。'" />
          <p v-if="captchaError" class="text-sm text-red-600 dark:text-red-300">{{ captchaError }}</p>
          <button type="submit" class="btn btn-primary w-full" :disabled="busy || !settings">
            <span v-if="busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            <Icon v-else name="mail" size="md" class="mr-2" />{{ busy ? '正在发送…' : '发送重置邮件' }}
          </button>
        </form>
      </section>
    </main>
    <template #footer><p class="text-gray-500 dark:text-dark-400">想起密码了？ <RouterLink to="/login" class="font-medium text-primary-600 dark:text-primary-400">返回登录</RouterLink></p></template>
  </AuthLayout>
</template>
