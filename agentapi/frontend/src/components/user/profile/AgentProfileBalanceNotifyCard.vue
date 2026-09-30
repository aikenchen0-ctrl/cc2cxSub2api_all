<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { agentAPI, type AgentBalanceNotifySettings, type AgentNotifyEmailEntry } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentToggle from '@/components/common/AgentToggle.vue'
import AgentInput from '@/components/common/AgentInput.vue'

const loading = ref(true)
const settings = ref<AgentBalanceNotifySettings | null>(null)
const notifyEnabled = ref(false)
const threshold = ref<number | null>(null)
const emails = ref<AgentNotifyEmailEntry[]>([])
const newEmail = ref('')
const verifyingEmail = ref('')
const verifyCode = ref('')
const countdown = ref(0)
const busy = ref('')
const error = ref('')
const notice = ref('')
let countdownTimer: ReturnType<typeof setInterval> | undefined

const featureEnabled = computed(() => settings.value?.feature_enabled === true)
const canAddEmail = computed(() => emails.value.length < 3)

function mergeProfile(result: AgentBalanceNotifySettings): void {
  notifyEnabled.value = result.enabled
  threshold.value = result.threshold
  emails.value = [...result.extra_emails]
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.profile.balanceNotify.get()
    settings.value = result
    mergeProfile(result)
  } catch (err) {
    error.value = errorMessage(err, '加载余额通知设置失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function toggleFeature(next: boolean): Promise<void> {
  if (busy.value) return
  const previous = !next
  busy.value = 'feature'
  error.value = ''
  notice.value = ''
  try {
    mergeProfile(await agentAPI.profile.balanceNotify.update({ enabled: notifyEnabled.value }))
    notice.value = notifyEnabled.value ? '余额通知已启用。' : '余额通知已关闭。'
  } catch (err) {
    notifyEnabled.value = previous
    error.value = errorMessage(err, '更新余额通知开关失败。')
  } finally {
    busy.value = ''
  }
}

async function saveThreshold(): Promise<void> {
  if (busy.value) return
  const value = Number(threshold.value || 0)
  if (!Number.isFinite(value) || value < 0) {
    error.value = '通知阈值必须是非负金额。'
    return
  }
  busy.value = 'threshold'
  error.value = ''
  notice.value = ''
  try {
    mergeProfile(await agentAPI.profile.balanceNotify.update({ threshold: value }))
    notice.value = '通知阈值已保存。'
  } catch (err) {
    error.value = errorMessage(err, '保存通知阈值失败。')
  } finally {
    busy.value = ''
  }
}

function startCountdown(): void {
  if (countdownTimer) clearInterval(countdownTimer)
  countdown.value = 60
  countdownTimer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0 && countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = undefined
    }
  }, 1000)
}

async function sendCode(email: string): Promise<void> {
  const normalized = email.trim().toLowerCase()
  if (!normalized || busy.value) return
  busy.value = 'send-code'
  error.value = ''
  notice.value = ''
  try {
    await agentAPI.profile.balanceNotify.sendCode(normalized)
    verifyingEmail.value = normalized
    verifyCode.value = ''
    startCountdown()
    notice.value = `验证码已发送至 ${normalized}。`
  } catch (err) {
    error.value = errorMessage(err, '发送通知邮箱验证码失败。')
  } finally {
    busy.value = ''
  }
}

async function addEmail(): Promise<void> {
  const value = newEmail.value.trim().toLowerCase()
  if (!value) return
  if (emails.value.some(entry => entry.email.toLowerCase() === value)) {
    error.value = '该邮箱已经在通知列表中。'
    return
  }
  await sendCode(value)
  if (verifyingEmail.value === value) newEmail.value = ''
}

async function verifyEmail(): Promise<void> {
  if (!/^\d{6}$/.test(verifyCode.value) || !verifyingEmail.value || busy.value) return
  busy.value = 'verify'
  error.value = ''
  try {
    await agentAPI.profile.balanceNotify.verify(verifyingEmail.value, verifyCode.value)
    verifyingEmail.value = ''
    verifyCode.value = ''
    if (countdownTimer) clearInterval(countdownTimer)
    countdownTimer = undefined
    countdown.value = 0
    notice.value = '通知邮箱验证成功。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '验证码无效或已过期。')
  } finally {
    busy.value = ''
  }
}

async function toggleEmail(entry: AgentNotifyEmailEntry): Promise<void> {
  if (busy.value) return
  busy.value = `toggle:${entry.email}`
  error.value = ''
  try {
    mergeProfile(await agentAPI.profile.balanceNotify.toggle(entry.email, !entry.disabled))
  } catch (err) {
    error.value = errorMessage(err, '更新通知邮箱状态失败。')
  } finally {
    busy.value = ''
  }
}

async function removeEmail(email: string): Promise<void> {
  if (busy.value) return
  busy.value = `remove:${email}`
  error.value = ''
  try {
    await agentAPI.profile.balanceNotify.remove(email)
    notice.value = '通知邮箱已移除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '移除通知邮箱失败。')
  } finally {
    busy.value = ''
  }
}

onMounted(load)
onUnmounted(() => { if (countdownTimer) clearInterval(countdownTimer) })
</script>

<template>
  <section v-if="loading || error || featureEnabled" data-testid="profile-balance-notify-card" class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">余额不足通知</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">主站余额低于设定金额时，通过已验证邮箱发送提醒。</p>
    </div>
    <div class="space-y-6 px-6 py-6">
      <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200">{{ notice }}</p>
      <div v-if="loading" role="status" class="py-6 text-center text-sm text-gray-500">正在加载余额通知设置…</div>
      <div v-else-if="!settings" class="flex items-center justify-between gap-4"><span class="text-sm text-gray-500">暂时无法读取设置。</span><button type="button" class="btn btn-secondary" @click="load">重新加载</button></div>
      <template v-else-if="featureEnabled">
        <div class="flex items-center justify-between">
          <label for="balance-notify-enabled" class="input-label mb-0">启用余额不足通知</label>
          <label class="relative inline-flex cursor-pointer items-center">
            <AgentToggle id="balance-notify-enabled" v-model="notifyEnabled" aria-label="启用余额不足通知" :disabled="Boolean(busy)" @change="toggleFeature" />
          </label>
        </div>

        <template v-if="notifyEnabled">
          <div>
            <label for="balance-notify-threshold" class="input-label">通知阈值 <span class="ml-2 text-xs font-normal text-gray-400">留空或填 0 使用主站默认值</span></label>
            <div class="flex items-center gap-2"><div class="min-w-0 flex-1"><AgentInput id="balance-notify-threshold" v-model.number="threshold" type="number" min="0" step="0.01" :placeholder="settings.system_default_threshold > 0 ? `主站默认 $${settings.system_default_threshold}` : '请输入通知阈值'"><template #prefix><span>$</span></template></AgentInput></div><button type="button" class="btn btn-primary btn-sm whitespace-nowrap" :disabled="Boolean(busy)" @click="saveThreshold">{{ busy === 'threshold' ? '保存中…' : '保存' }}</button></div>
          </div>

          <div>
            <label class="input-label">额外通知邮箱</label>
            <p class="mb-3 text-xs text-amber-600 dark:text-amber-400">邮箱必须完成验证码确认后才会收到通知，最多 3 个。</p>
            <div v-if="emails.length" class="mb-3 space-y-2">
              <div v-for="entry in emails" :key="entry.email" class="flex flex-col gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700 sm:flex-row sm:items-center sm:justify-between">
                <div class="flex min-w-0 items-center gap-2"><input type="checkbox" :checked="!entry.disabled" :disabled="Boolean(busy) || !entry.verified" :aria-label="`启用 ${entry.email}`" @change="toggleEmail(entry)" /><span class="truncate text-sm text-gray-700 dark:text-gray-300">{{ entry.email }}</span><span :class="entry.verified ? 'text-emerald-600' : 'text-amber-500'" class="text-xs">{{ entry.verified ? '已验证' : '未验证' }}</span></div>
                <div class="flex items-center gap-3"><button v-if="!entry.verified" type="button" class="text-xs text-primary-600" :disabled="Boolean(busy)" @click="sendCode(entry.email)">验证</button><button type="button" class="text-xs text-red-500" :disabled="Boolean(busy)" @click="removeEmail(entry.email)">移除</button></div>
              </div>
            </div>

            <div v-if="verifyingEmail" data-testid="balance-notify-verification" class="mb-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-900/10">
              <p class="mb-2 text-sm text-gray-700 dark:text-gray-300">验证 {{ verifyingEmail }}</p>
              <div class="flex flex-wrap items-center gap-2"><div class="w-32"><AgentInput v-model="verifyCode" maxlength="6" inputmode="numeric" autocomplete="one-time-code" placeholder="6 位验证码" /></div><button type="button" class="btn btn-primary btn-sm" :disabled="busy === 'verify' || !/^\d{6}$/.test(verifyCode)" @click="verifyEmail">{{ busy === 'verify' ? '验证中…' : '验证' }}</button><span v-if="countdown > 0" class="text-xs text-gray-400">{{ countdown }}s</span><button v-else type="button" class="text-xs text-gray-500" :disabled="Boolean(busy)" @click="sendCode(verifyingEmail)">重新发送</button><button type="button" class="text-xs text-gray-400" @click="verifyingEmail = ''">取消</button></div>
            </div>

            <div v-if="canAddEmail" class="flex gap-2"><div class="min-w-0 flex-1"><AgentInput v-model="newEmail" type="email" placeholder="添加通知邮箱" @keyup.enter="addEmail" /></div><button type="button" class="btn btn-secondary whitespace-nowrap" :disabled="!newEmail || Boolean(busy)" @click="addEmail">添加并验证</button></div>
            <p v-else class="text-xs text-gray-400">已达到最多 3 个通知邮箱。</p>
          </div>
        </template>
      </template>
    </div>
  </section>
</template>
