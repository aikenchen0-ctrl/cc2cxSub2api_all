<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { agentAPI, type AgentProfile } from '@/agent/api'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import AgentProfileAvatarCard from '@/components/user/profile/AgentProfileAvatarCard.vue'
import AgentProfileTotpCard from '@/components/user/profile/AgentProfileTotpCard.vue'
import AgentProfileBalanceNotifyCard from '@/components/user/profile/AgentProfileBalanceNotifyCard.vue'
import AgentProfileIdentityBindingsCard from '@/components/user/profile/AgentProfileIdentityBindingsCard.vue'
import AgentProfilePasskeyCard from '@/components/user/profile/AgentProfilePasskeyCard.vue'
import { agentPasskeyAPI } from '@/agent/passkeys'
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
const passkeyEnabled = ref(false)
const displayName = computed(() => profile.value?.username || profile.value?.email || '用户')
const avatarInitial = computed(() => Array.from(displayName.value)[0]?.toUpperCase() || 'U')
const avatarUrl = computed(() => profile.value?.avatar_url?.trim() || '')

function handleAvatarUpdated(updated: AgentProfile): void {
  profile.value = updated
  username.value = updated.username
  if (auth.user) {
    auth.user = {
      ...auth.user,
      username: updated.username,
      avatar_url: updated.avatar_url,
    }
  }
}

function handleIdentityProfileUpdated(updated: AgentProfile): void {
  profile.value = updated
  username.value = updated.username
  if (auth.user) {
    auth.user = {
      ...auth.user,
      email: updated.email,
      username: updated.username,
      avatar_url: updated.avatar_url,
    }
  }
}

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

async function loadPasskeyConfig(): Promise<void> {
  try {
    const config = await agentPasskeyAPI.config()
    passkeyEnabled.value = config.enabled === true && config.supported_origin === true
  } catch {
    passkeyEnabled.value = false
  }
}

async function saveProfile(): Promise<void> {
  if (!profile.value?.can_edit || savingProfile.value) return
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
  if (!profile.value?.can_edit || changingPassword.value) return
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
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
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

onMounted(() => { void loadProfile(); void loadPasskeyConfig() })
</script>

<template>
  <main data-testid="profile-shell" class="mx-auto max-w-[950px] space-y-6">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">个人资料</h1>
    <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>
    <p v-if="loading" role="status" class="text-sm text-gray-500">正在加载个人资料…</p>
    <button v-else-if="!profile" type="button" class="btn btn-secondary" @click="loadProfile">重新加载</button>
    <template v-else>
      <section data-testid="profile-overview-hero" class="card overflow-hidden border border-primary-100/80 bg-gradient-to-br from-primary-50 via-white to-amber-50/70 dark:border-primary-900/40 dark:from-primary-950/40 dark:via-dark-900 dark:to-dark-950">
        <div class="px-6 py-6 md:px-8">
          <div class="flex flex-col gap-6 lg:flex-row lg:items-start">
            <div class="flex h-20 w-20 shrink-0 items-center justify-center overflow-hidden rounded-[1.75rem] bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20">
              <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="h-full w-full object-cover">
              <span v-else>{{ avatarInitial }}</span>
            </div>
            <div class="min-w-0 flex-1 space-y-5">
              <div class="space-y-3">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="truncate text-2xl font-semibold text-gray-900 dark:text-white">{{ displayName }}</h2>
                  <span :class="['badge', auth.isAgentAdmin ? 'badge-primary' : 'badge-gray']">{{ auth.isAgentAdmin ? '本站管理员' : '用户' }}</span>
                </div>
                <p class="truncate text-sm text-gray-600 dark:text-gray-300">{{ profile.email || '—' }}</p>
              </div>
              <dl class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700">
                  <dt class="text-xs font-medium text-gray-500">主站用户编号</dt>
                  <dd class="mt-1 break-all text-lg font-semibold text-gray-900 dark:text-white">{{ profile.id }}</dd>
                </div>
                <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700">
                  <dt class="text-xs font-medium text-gray-500">当前会话权限</dt>
                  <dd class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ profile.can_edit ? '可修改个人资料' : '单点登录 · 只读' }}</dd>
                </div>
              </dl>
            </div>
          </div>
        </div>
      </section>
      <section data-testid="profile-basics-panel" class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50">
        <div class="mb-5">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">基本资料</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">此操作会同步更新 Sub2API 主站账号名称。</p>
        </div>
        <div class="rounded-3xl border border-gray-100 bg-gray-50/80 p-5 dark:border-dark-700 dark:bg-dark-900/30">
          <AgentProfileAvatarCard v-if="profile.can_edit" :profile="profile" class="mb-5" @updated="handleAvatarUpdated" />
          <ProfileEditForm v-model="username" :initial-username="profile.username" :disabled="!profile.can_edit" :loading="savingProfile" embedded @submit="saveProfile" />
        </div>
      </section>
      <p v-if="!profile.can_edit" class="card p-6 text-sm text-gray-600 dark:text-gray-300">
        当前会话通过单点登录创建，仅用于身份识别。如需修改账号安全设置，请使用主站邮箱和密码登录。
      </p>
      <div v-else class="space-y-3">
        <p class="text-sm text-gray-500 dark:text-gray-400">当前密码由 Sub2API 验证。修改成功后需重新登录，主站其他登录会话也将失效。</p>
        <AgentProfileIdentityBindingsCard :profile="profile" @updated="handleIdentityProfileUpdated" />
        <ProfilePasswordForm v-model:old-password="oldPassword" v-model:new-password="newPassword" v-model:confirm-password="confirmPassword" :loading="changingPassword" @submit="changePassword" />
        <AgentProfileBalanceNotifyCard />
        <AgentProfileTotpCard />
        <AgentProfilePasskeyCard :enabled="passkeyEnabled" />
      </div>
    </template>
    <section class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"><h2 class="text-lg font-medium text-gray-900 dark:text-white">当前会话</h2></div>
      <div class="space-y-4 px-6 py-6">
        <p class="text-sm text-gray-500 dark:text-gray-400">退出此浏览器中的代理站会话。</p>
        <button class="btn btn-secondary text-red-600" type="button" @click="logout">退出登录</button>
      </div>
    </section>
  </main>
</template>
