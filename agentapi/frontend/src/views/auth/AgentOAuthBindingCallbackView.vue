<template>
  <main class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-900">
    <section class="card mx-auto max-w-2xl p-6">
      <h1 class="text-lg font-semibold text-gray-900 dark:text-white">第三方账号绑定</h1>
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
        {{ error || '绑定已完成，正在返回个人资料页面…' }}
      </p>

      <div v-if="!error" role="status" class="mt-6 flex items-center justify-center py-10">
        <span class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></span>
      </div>

      <div v-else role="alert" class="mt-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/80">
        <p class="text-sm text-gray-700 dark:text-gray-300">{{ error }}</p>
        <button class="btn btn-primary mt-4" type="button" @click="goProfile">返回个人资料</button>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const error = ref('')
const supportedProviders = new Set(['linuxdo', 'oidc', 'wechat', 'dingtalk'])

function queryValue(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function fragmentParams(): URLSearchParams {
  const hash = window.location.hash.startsWith('#') ? window.location.hash.slice(1) : window.location.hash
  return new URLSearchParams(hash)
}

function normalizeRedirectPath(path: string): string {
  const value = path.trim()
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('://')) return '/profile'
  return value
}

function cleanCallbackURL(): void {
  if (!window.location.hash) return
  window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
}

function goProfile(): void {
  void router.replace('/profile')
}

onMounted(async () => {
  const fragment = fragmentParams()
  const read = (key: string) => fragment.get(key) || queryValue(key)
  const provider = read('provider').trim().toLowerCase()
  const status = read('status').trim().toLowerCase()
  const callbackError = read('error').trim()

  cleanCallbackURL()

  if (!supportedProviders.has(provider)) {
    error.value = '无法确认第三方账号类型，请返回个人资料页面重新发起绑定。'
    return
  }
  if (callbackError || status !== 'success') {
    error.value = '第三方账号绑定未完成，请返回个人资料页面后重试。'
    return
  }

  await router.replace({
    path: normalizeRedirectPath(read('redirect')),
    query: { oauth_binding: 'success', provider },
  })
})
</script>
