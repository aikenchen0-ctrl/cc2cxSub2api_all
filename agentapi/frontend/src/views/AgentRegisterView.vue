<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

const router = useRouter()
const session = useAgentSession()
const email = ref('')
const emailInput = ref<HTMLInputElement | null>(null)
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')

async function submit(): Promise<void> {
  error.value = ''
  if (!email.value.trim()) { error.value = '请输入邮箱地址。'; return }
  if (emailInput.value?.validity.typeMismatch) { error.value = '请输入有效的邮箱地址。'; return }
  if (!password.value) { error.value = '请输入密码。'; return }
  if (password.value.length < 6) { error.value = '密码至少需要 6 个字符。'; return }
  if (!confirmPassword.value) { error.value = '请再次输入密码。'; return }
  if (password.value !== confirmPassword.value) { error.value = '两次输入的密码不一致。'; return }
  try {
    await session.register(email.value.trim(), password.value, username.value.trim() || undefined)
    await router.replace('/dashboard')
  } catch (err) {
    error.value = errorMessage(err, '注册失败，请检查填写内容后重试。')
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-[calc(100vh-5rem)] max-w-md items-center px-4 py-10">
    <section class="w-full rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
      <h1 class="text-2xl font-semibold">注册账号</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">账号将在 Sub2API 主站创建，并关联到当前 AgentAPI 实例。</p>
      <p v-if="error" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <form class="mt-6 space-y-4" novalidate @submit.prevent="submit">
        <label class="block text-sm font-medium">邮箱<input ref="emailInput" v-model="email" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="email" autocomplete="email" required></label>
        <label class="block text-sm font-medium">显示名称 <span class="font-normal text-gray-400">（选填）</span><input v-model="username" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="text" maxlength="100" autocomplete="nickname"></label>
        <label class="block text-sm font-medium">密码<input v-model="password" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="new-password" required></label>
        <label class="block text-sm font-medium">确认密码<input v-model="confirmPassword" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="new-password" required></label>
        <button class="w-full rounded-lg bg-primary-600 px-4 py-2.5 font-medium text-white disabled:opacity-50" type="submit" :disabled="session.busy">{{ session.busy ? '正在创建…' : '注册账号' }}</button>
      </form>
      <p class="mt-6 text-center text-sm text-gray-500">已有账号？<RouterLink class="font-medium text-primary-600" to="/login">登录</RouterLink></p>
    </section>
  </main>
</template>
