<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { agentAPI, type AgentProfile } from '@/agent/api'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

const auth = useAgentSession()
const router = useRouter()
const profile = ref<AgentProfile | null>(null)
const username = ref('')
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const loading = ref(true)
const savingProfile = ref(false)
const changingPassword = ref(false)
const error = ref('')
const notice = ref('')

async function loadProfile(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    profile.value = await agentAPI.profile.get()
    username.value = profile.value.username
  } catch (err) {
    error.value = errorMessage(err, '加载个人资料失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function saveProfile(): Promise<void> {
  const nextUsername = username.value.trim()
  if (!nextUsername) {
    error.value = '请填写用户名。'
    return
  }
  savingProfile.value = true
  error.value = ''
  notice.value = ''
  try {
    profile.value = await agentAPI.profile.update(nextUsername)
    username.value = profile.value.username
    notice.value = '个人资料已更新。'
  } catch (err) {
    error.value = errorMessage(err, '更新个人资料失败，请稍后重试。')
  } finally {
    savingProfile.value = false
  }
}

async function changePassword(): Promise<void> {
  if (!oldPassword.value) {
    error.value = '请输入当前密码。'
    return
  }
  if (!newPassword.value) {
    error.value = '请输入新密码。'
    return
  }
  if (newPassword.value.length < 8) {
    error.value = '新密码至少需要 8 个字符。'
    return
  }
  if (!confirmPassword.value) {
    error.value = '请再次输入新密码。'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = '两次输入的新密码不一致。'
    return
  }
  changingPassword.value = true
  error.value = ''
  notice.value = ''
  try {
    await agentAPI.profile.changePassword(oldPassword.value, newPassword.value)
    // Sub2API invalidates access and refresh tokens after a password change;
    // AgentAPI also revokes its local HttpOnly session in the same response.
    auth.clear()
    await router.replace({ path: '/login', query: { passwordChanged: '1' } })
  } catch (err) {
    error.value = errorMessage(err, '修改密码失败，请稍后重试。')
  } finally {
    changingPassword.value = false
  }
}

async function logout(): Promise<void> {
  await auth.logout()
  await router.replace('/home')
}

onMounted(loadProfile)
</script>

<template>
  <main class="mx-auto max-w-4xl space-y-6 p-6">
    <header>
      <p class="text-sm text-slate-500">AgentAPI</p>
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">个人资料</h1>
      <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">在此代理站管理你的 Sub2API 账号。</p>
    </header>

    <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
      {{ error }}
    </p>
    <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200">
      {{ notice }}
    </p>
    <p v-if="loading" role="status" class="text-sm text-slate-500">正在加载个人资料…</p>

    <template v-else-if="profile">
      <section class="rounded-xl border bg-white p-6 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <div class="mb-5">
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">账号信息</h2>
          <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">账号身份和登录信息由 Sub2API 管理。</p>
        </div>
        <dl class="grid gap-4 text-sm sm:grid-cols-2">
          <div>
            <dt class="text-slate-500 dark:text-slate-400">邮箱</dt>
            <dd class="mt-1 font-medium text-slate-900 dark:text-white">{{ profile.email || '—' }}</dd>
          </div>
          <div>
            <dt class="text-slate-500 dark:text-slate-400">主站用户编号</dt>
            <dd class="mt-1 break-all font-mono text-slate-900 dark:text-white">{{ profile.id }}</dd>
          </div>
        </dl>

        <form class="mt-6 border-t pt-5 dark:border-slate-700" novalidate @submit.prevent="saveProfile">
          <label for="profile-username" class="block text-sm font-medium text-slate-700 dark:text-slate-200">用户名</label>
          <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">此操作会同步更新 Sub2API 主站账号名称。</p>
          <div class="mt-3 flex flex-col gap-3 sm:flex-row">
            <input
              id="profile-username"
              v-model="username"
              name="username"
              type="text"
              maxlength="128"
              autocomplete="nickname"
              required
              :disabled="!profile.can_edit || savingProfile"
              class="min-w-0 flex-1 rounded-lg border px-3 py-2 text-sm disabled:bg-slate-100 dark:border-slate-700 dark:bg-slate-950 dark:text-white dark:disabled:bg-slate-800"
            >
            <button
              type="submit"
              :disabled="!profile.can_edit || savingProfile || username.trim() === profile.username"
              class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
            >
              {{ savingProfile ? '正在保存…' : '保存资料' }}
            </button>
          </div>
        </form>
      </section>

      <section class="rounded-xl border bg-white p-6 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <div class="mb-5">
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">修改密码</h2>
          <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">当前密码由 Sub2API 验证。密码修改成功后，主站的其他登录会话将失效。</p>
        </div>

        <p v-if="!profile.can_edit" class="rounded-lg bg-slate-50 p-4 text-sm text-slate-600 dark:bg-slate-800 dark:text-slate-300">
          当前会话通过单点登录创建，仅用于身份识别。如需修改账号安全设置，请使用主站邮箱和密码登录。
        </p>
        <form v-else class="space-y-4" novalidate @submit.prevent="changePassword">
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-200">
              当前密码
              <input v-model="oldPassword" name="old_password" aria-label="当前密码" type="password" autocomplete="current-password" required class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950 dark:text-white">
            </label>
            <span class="hidden sm:block" />
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-200">
              新密码
              <input v-model="newPassword" name="new_password" aria-label="新密码" type="password" autocomplete="new-password" minlength="8" required class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950 dark:text-white">
              <span class="mt-1 block text-xs font-normal text-slate-500">至少 8 个字符。</span>
            </label>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-200">
              确认新密码
              <input v-model="confirmPassword" name="confirm_password" aria-label="确认新密码" type="password" autocomplete="new-password" minlength="8" required class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950 dark:text-white">
            </label>
          </div>
          <div class="flex justify-end pt-2">
            <button type="submit" :disabled="changingPassword" class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
              {{ changingPassword ? '正在修改…' : '修改密码' }}
            </button>
          </div>
        </form>
      </section>
    </template>

    <section class="rounded-xl border bg-white p-6 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="text-lg font-semibold text-slate-900 dark:text-white">当前会话</h2>
      <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">退出此浏览器中的 AgentAPI 会话。</p>
      <button class="mt-5 rounded-lg border border-red-200 px-4 py-2 text-sm text-red-600 hover:bg-red-50 dark:border-red-900 dark:hover:bg-red-950/40" type="button" @click="logout">退出登录</button>
    </section>
  </main>
</template>
