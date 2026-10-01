<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { agentAPI } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AgentInput from '@/components/common/AgentInput.vue'

const route = useRoute()
const email = computed(() => typeof route.query.email === 'string' ? route.query.email.trim() : '')
const token = computed(() => typeof route.query.token === 'string' ? route.query.token.trim() : '')
const invalid = computed(() => !email.value || !token.value)
const form = reactive({ password: '', confirm: '' })
const busy = ref(false)
const success = ref(false)
const error = ref('')
const showPassword = ref(false)
const showConfirm = ref(false)

async function submit(): Promise<void> {
  error.value = ''
  if (invalid.value) { error.value = '重置链接无效或不完整。'; return }
  if (form.password.length < 6) { error.value = '新密码至少需要 6 个字符。'; return }
  if (form.password !== form.confirm) { error.value = '两次输入的密码不一致。'; return }
  busy.value = true
  try {
    await agentAPI.auth.resetPassword(email.value, token.value, form.password)
    form.password = ''; form.confirm = ''; success.value = true
  } catch (reason) { error.value = errorMessage(reason, '重置密码失败，链接可能已失效。') }
  finally { busy.value = false }
}
</script>

<template>
  <AuthLayout>
    <main class="w-full"><section class="space-y-6">
      <div class="text-center"><h1 class="text-2xl font-bold text-gray-900 dark:text-white">重置密码</h1><p class="mt-2 text-sm text-gray-500 dark:text-dark-400">为 {{ email || '当前账号' }} 设置新密码</p></div>
      <div v-if="invalid" class="rounded-xl border border-amber-200 bg-amber-50 p-5 text-center dark:border-amber-500/30 dark:bg-amber-500/10"><Icon name="exclamationCircle" size="lg" class="mx-auto text-amber-600" /><h2 class="mt-3 font-semibold text-amber-800 dark:text-amber-200">重置链接无效</h2><p class="mt-2 text-sm text-amber-700 dark:text-amber-300">链接缺少必要参数，请重新申请。</p><RouterLink to="/forgot-password" class="btn btn-primary mt-4">重新申请链接</RouterLink></div>
      <div v-else-if="success" class="rounded-xl border border-emerald-200 bg-emerald-50 p-5 text-center dark:border-emerald-500/30 dark:bg-emerald-500/10"><Icon name="checkCircle" size="lg" class="mx-auto text-emerald-600" /><h2 class="mt-3 font-semibold text-emerald-800 dark:text-emerald-200">密码重置成功</h2><p class="mt-2 text-sm text-emerald-700 dark:text-emerald-300">现在可以使用新密码登录本站。</p><RouterLink to="/login" class="btn btn-primary mt-4">前往登录</RouterLink></div>
      <form v-else class="space-y-5" @submit.prevent="submit">
        <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">{{ error }}</div>
        <div><label for="reset-password" class="input-label">新密码</label><AgentInput id="reset-password" v-model="form.password" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" required :disabled="busy" placeholder="至少 6 个字符"><template #prefix><Icon name="lock" size="md" /></template><template #suffix><button type="button" :aria-label="showPassword ? '隐藏密码' : '显示密码'" class="flex text-gray-400" :disabled="busy" @click="showPassword = !showPassword"><Icon :name="showPassword ? 'eyeOff' : 'eye'" size="md" /></button></template></AgentInput></div>
        <div><label for="reset-confirm" class="input-label">确认新密码</label><AgentInput id="reset-confirm" v-model="form.confirm" :type="showConfirm ? 'text' : 'password'" autocomplete="new-password" required :disabled="busy" placeholder="再次输入新密码"><template #prefix><Icon name="lock" size="md" /></template><template #suffix><button type="button" :aria-label="showConfirm ? '隐藏确认密码' : '显示确认密码'" class="flex text-gray-400" :disabled="busy" @click="showConfirm = !showConfirm"><Icon :name="showConfirm ? 'eyeOff' : 'eye'" size="md" /></button></template></AgentInput></div>
        <button type="submit" class="btn btn-primary w-full" :disabled="busy"><span v-if="busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" /><Icon v-else name="checkCircle" size="md" class="mr-2" />{{ busy ? '正在重置…' : '重置密码' }}</button>
      </form>
    </section></main>
    <template #footer><p class="text-gray-500 dark:text-dark-400">返回 <RouterLink to="/login" class="font-medium text-primary-600 dark:text-primary-400">登录页面</RouterLink></p></template>
  </AuthLayout>
</template>
