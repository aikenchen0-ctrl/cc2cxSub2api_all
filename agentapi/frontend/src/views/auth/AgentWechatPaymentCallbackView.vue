<template>
  <main class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-900">
    <section class="card mx-auto max-w-2xl p-6">
      <h1 class="text-lg font-semibold text-gray-900 dark:text-white">恢复微信支付</h1>
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
        {{ error || '正在验证微信授权结果并返回购买页面…' }}
      </p>

      <div v-if="!error" role="status" class="mt-6 flex items-center justify-center py-10">
        <span class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></span>
      </div>

      <div v-else role="alert" class="mt-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/80">
        <p class="text-sm text-gray-700 dark:text-gray-300">{{ error }}</p>
        <button class="btn btn-primary mt-4" type="button" @click="goBack">返回购买页</button>
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
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('://')) return '/purchase'
  if (value === '/payment') return '/purchase'
  if (value.startsWith('/payment?')) return `/purchase${value.slice('/payment'.length)}`
  return value
}

function append(query: Record<string, string>, key: string, value: string): void {
  if (value) query[key] = value
}

function goBack(): void {
  void router.replace('/purchase')
}

onMounted(async () => {
  const fragment = fragmentParams()
  const read = (key: string) => fragment.get(key) || queryValue(key)
  const callbackError = read('error') || read('err_msg') || read('errmsg')
  if (callbackError) {
    error.value = read('error_description') || read('message') || callbackError
    return
  }

  const resumeToken = read('wechat_resume_token')
  const openid = read('openid')
  if (!resumeToken && !openid) {
    error.value = '微信授权结果缺少支付恢复信息，请返回购买页重新发起支付。'
    return
  }

  const redirectURL = new URL(normalizeRedirectPath(read('redirect')), window.location.origin)
  const query: Record<string, string> = {
    ...Object.fromEntries(redirectURL.searchParams.entries()),
    wechat_resume: '1',
  }
  if (resumeToken) {
    query.wechat_resume_token = resumeToken
  } else {
    query.openid = openid
    append(query, 'state', read('state'))
    append(query, 'scope', read('scope'))
    append(query, 'payment_type', read('payment_type'))
    append(query, 'amount', read('amount'))
    append(query, 'order_type', read('order_type'))
    append(query, 'plan_id', read('plan_id'))
  }

  if (window.location.hash) {
    window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
  }
  await router.replace({ path: redirectURL.pathname, query })
})
</script>
