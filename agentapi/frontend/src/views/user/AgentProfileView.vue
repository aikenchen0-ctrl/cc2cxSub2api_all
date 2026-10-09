<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { agentAPI, type AgentIdentityBinding, type AgentProfile, type AgentTOTPStatus } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import Icon from '@/components/icons/Icon.vue'

const router = useRouter()
const profile = ref<AgentProfile | null>(null)
const bindings = ref<AgentIdentityBinding[]>([])
const totp = ref<AgentTOTPStatus | null>(null)
const username = ref('')
const avatarURL = ref('')
const oldPassword = ref('')
const newPassword = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [nextProfile, nextBindings, nextTotp] = await Promise.all([
      agentAPI.profile.get(),
      agentAPI.profile.bindings.get().catch(() => null),
      agentAPI.profile.totp.getStatus().catch(() => null),
    ])
    profile.value = nextProfile
    username.value = nextProfile.username || ''
    avatarURL.value = nextProfile.avatar_url || ''
    bindings.value = nextBindings?.items || []
    totp.value = nextTotp
  } catch (cause) {
    profile.value = null
    error.value = errorMessage(cause, '个人资料加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function saveProfile(): Promise<void> {
  if (!profile.value?.can_edit || saving.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    profile.value = await agentAPI.profile.update(username.value.trim())
    if (avatarURL.value.trim() !== (profile.value.avatar_url || '')) {
      profile.value = await agentAPI.profile.avatar.update(avatarURL.value.trim())
    }
    notice.value = '个人资料已保存到主站账户。'
  } catch (cause) {
    error.value = errorMessage(cause, '保存个人资料失败。')
  } finally {
    saving.value = false
  }
}

async function changePassword(): Promise<void> {
  if (saving.value || newPassword.value.length < 6) return
  saving.value = true
  error.value = ''
  try {
    await agentAPI.profile.changePassword(oldPassword.value, newPassword.value)
    await router.push('/login')
  } catch (cause) {
    error.value = errorMessage(cause, '修改密码失败。')
    saving.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-3xl space-y-6">
    <header>
      <p class="text-sm font-medium text-primary-600 dark:text-primary-300">ACCOUNT</p>
      <h1 class="mt-1 text-2xl font-bold">个人资料</h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">资料保存在主站账户。这里只能修改当前登录用户，不能查看或修改其他用户。</p>
    </header>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ notice }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500" role="status">正在读取个人资料…</section>
    <template v-else-if="profile">
      <section class="card space-y-4 p-5">
        <h2 class="font-semibold">基本信息</h2>
        <p class="text-sm text-gray-500">邮箱 {{ profile.email || '未提供' }}</p>
        <label class="block text-sm font-medium" for="profile-username">显示名称</label>
        <input id="profile-username" v-model="username" class="input" maxlength="80" :disabled="!profile.can_edit">
        <label class="block text-sm font-medium" for="profile-avatar">头像地址</label>
        <input id="profile-avatar" v-model="avatarURL" class="input" maxlength="500" placeholder="https://" :disabled="!profile.can_edit">
        <button class="btn btn-primary" type="button" :disabled="saving || !profile.can_edit" @click="saveProfile">保存资料</button>
        <p v-if="!profile.can_edit" class="text-sm text-amber-700">当前会话不能修改主站资料，请使用密码登录后再试。</p>
      </section>
      <section class="card space-y-4 p-5">
        <h2 class="font-semibold">修改密码</h2>
        <input v-model="oldPassword" class="input" type="password" autocomplete="current-password" placeholder="当前密码" aria-label="当前密码">
        <input v-model="newPassword" class="input" type="password" autocomplete="new-password" placeholder="新密码，至少 6 位" aria-label="新密码">
        <button class="btn btn-secondary" type="button" :disabled="saving || newPassword.length < 6" @click="changePassword">更新密码并重新登录</button>
      </section>
      <section class="card space-y-3 p-5">
        <h2 class="font-semibold">登录方式</h2>
        <p v-if="!bindings.length" class="text-sm text-gray-500">暂未读取到可展示的绑定方式。</p>
        <ul class="space-y-2">
          <li v-for="item in bindings" :key="item.provider" class="flex items-center justify-between text-sm">
            <span>{{ item.display_name || item.provider }}</span>
            <span class="text-gray-500">{{ item.bound ? '已绑定' : '未绑定' }}</span>
          </li>
        </ul>
        <p v-if="totp" class="flex items-center gap-2 text-sm text-gray-500"><Icon name="shield" size="sm" />双重验证{{ totp.enabled ? '已开启' : '未开启' }}</p>
      </section>
    </template>
  </main>
</template>
