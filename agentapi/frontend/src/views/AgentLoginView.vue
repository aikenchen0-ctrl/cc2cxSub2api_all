<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const email = ref('')
const emailInput = ref<HTMLInputElement | null>(null)
const password = ref('')
const totp = ref('')
const tempToken = ref('')
const error = ref('')
const passwordChanged = route.query.passwordChanged === '1'

async function submit(): Promise<void> {
  error.value = ''
  if (tempToken.value) {
    const code = totp.value.trim()
    if (!code) { error.value = '请输入验证器代码。'; return }
    if (code.length < 6 || code.length > 8) { error.value = '验证器代码应为 6 至 8 位。'; return }
  } else {
    if (!email.value.trim()) { error.value = '请输入邮箱地址。'; return }
    if (emailInput.value?.validity.typeMismatch) { error.value = '请输入有效的邮箱地址。'; return }
    if (!password.value) { error.value = '请输入密码。'; return }
  }
  try {
    if (tempToken.value) {
      await session.login2FA(tempToken.value, totp.value.trim())
    } else {
      const response = await session.login(email.value.trim(), password.value)
      if (response.requires_2fa) {
        tempToken.value = response.temp_token || ''
        return
      }
    }
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (err) {
    error.value = errorMessage(err, '登录失败，请检查账号信息后重试。')
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-[calc(100vh-5rem)] max-w-md items-center px-4 py-10">
    <section class="w-full rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
      <h1 class="text-2xl font-semibold">{{ tempToken ? '双重验证' : '登录' }}</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">浏览器仅保存 AgentAPI 会话信息；主站凭据仅保存在服务端。</p>
      <p v-if="passwordChanged" role="status" class="mt-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200">密码已修改，请使用新密码登录。</p>
      <p v-if="error" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <form class="mt-6 space-y-4" novalidate @submit.prevent="submit">
        <template v-if="!tempToken">
          <label class="block text-sm font-medium">邮箱<input ref="emailInput" v-model="email" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="email" autocomplete="email" required></label>
          <label class="block text-sm font-medium">密码<input v-model="password" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="current-password" required></label>
        </template>
        <label v-else class="block text-sm font-medium">验证器代码<input v-model="totp" class="mt-1 w-full rounded-lg border px-3 py-2 tracking-[0.4em] dark:border-dark-600 dark:bg-dark-950" inputmode="numeric" autocomplete="one-time-code" minlength="6" maxlength="8" required></label>
        <button class="w-full rounded-lg bg-primary-600 px-4 py-2.5 font-medium text-white disabled:opacity-50" type="submit" :disabled="session.busy">{{ session.busy ? '正在登录…' : tempToken ? '验证' : '登录' }}</button>
      </form>
      <p class="mt-6 text-center text-sm text-gray-500">还没有账号？<RouterLink class="font-medium text-primary-600" to="/register">立即注册</RouterLink></p>
    </section>
  </main>
</template>
