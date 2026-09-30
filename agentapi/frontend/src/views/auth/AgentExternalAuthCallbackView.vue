<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{ provider?: string }>(), {
  provider: '第三方账号',
})

const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const checking = ref(true)

function safeInternalPath(value: unknown): string {
  if (typeof value !== 'string') return '/dashboard'
  if (!value.startsWith('/') || value.startsWith('//') || value.includes('://') || /[\r\n]/.test(value)) return '/dashboard'
  if (value.startsWith('/auth/')) return '/dashboard'
  return value
}

const destination = computed(() => safeInternalPath(route.query.redirect ?? route.query.next))
const mainSiteLoginURL = computed(() => `/api/v1/auth/main-site/login?next=${encodeURIComponent(destination.value)}`)

onMounted(async () => {
  // OAuth code/state/token 由主站后端处理。代理站既不消费也不在页面中回显这些参数。
  // 清理地址栏可避免用户复制地址时意外带出一次性授权材料。
  if (typeof window !== 'undefined' && (window.location.search || window.location.hash)) {
    window.history.replaceState(window.history.state, '', route.path)
  }

  if (!session.initialized) await session.checkAuth()
  if (session.isAuthenticated) {
    await router.replace(destination.value)
    return
  }
  checking.value = false
})
</script>

<template>
  <main class="w-full" data-testid="external-auth-callback">
    <section class="space-y-6 text-center">
      <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
        <span v-if="checking" class="h-6 w-6 animate-spin rounded-full border-2 border-primary-200 border-t-primary-600 dark:border-primary-500/30 dark:border-t-primary-300" aria-hidden="true"></span>
        <Icon v-else name="shield" size="lg" />
      </div>

      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ checking ? `正在检查${props.provider}登录状态` : `${props.provider}登录由主站完成` }}
        </h1>
        <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
          {{ checking ? '正在确认当前代理站会话，请稍候。' : '代理站不会保存或交换第三方授权码。请通过主站完成授权，然后由一次性 SSO 票据建立本站会话。' }}
        </p>
      </div>

      <div v-if="!checking" role="status" class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-left text-sm leading-6 text-blue-800 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-200">
        <div class="flex gap-3">
          <Icon name="shield" size="md" class="mt-0.5 shrink-0" />
          <p>浏览器只保留代理站的 HttpOnly 会话；主站 OAuth 凭据、应用凭据、管理员 Key、SuperKey 与用户令牌都不会进入代理站前端。</p>
        </div>
      </div>

      <a v-if="!checking" data-testid="main-site-auth-restart" class="btn btn-primary w-full" :href="mainSiteLoginURL">
        <Icon name="externalLink" size="md" class="mr-2" />
        返回主站继续登录
      </a>
      <RouterLink v-if="!checking" class="btn btn-secondary w-full" to="/login">使用本站账号登录</RouterLink>
    </section>
  </main>
</template>
