<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { onAgentAccountFactsChanged, type AgentAccountFactsChangedDetail } from '@/agent/accountFacts'
import { agentAPI } from '@/agent/api'
import { siteContactInfo, siteDocURL, siteName } from '@/agent/branding'
import { agentOnboardingReplayAvailable, replayAgentOnboarding } from '@/agent/onboarding'
import { resolveAgentRoutePresentation } from '@/agent/routePresentation'
import { useAgentSession } from '@/agent/session'
import AgentAnnouncementBell from '@/components/common/AgentAnnouncementBell.vue'
import Icon from '@/components/icons/Icon.vue'

const emit = defineEmits<{ 'open-mobile': [] }>()
const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const dropdownOpen = ref(false)
const signingOut = ref(false)
const error = ref('')
const balanceCents = ref<number | null>(null)
const frozenBalanceCents = ref(0)
const dropdownRef = ref<HTMLElement | null>(null)
let stopAccountFactsListener: (() => void) | null = null
let balanceRequest = 0

const pagePresentation = computed(() => resolveAgentRoutePresentation(route.path, String(route.meta.title || '控制台')))
const pageTitle = computed(() => pagePresentation.value.title)
const pageDescription = computed(() => pagePresentation.value.description)
const displayName = computed(() => session.user?.display_name || session.user?.username || session.user?.email || siteName.value)
const userInitials = computed(() => displayName.value.trim().charAt(0).toUpperCase() || 'U')
const roleLabel = computed(() => session.isAgentAdmin ? '站点管理员' : '用户')
const totalBalanceCents = computed(() => balanceCents.value == null ? null : balanceCents.value + frozenBalanceCents.value)

function formatMoney(value: number | null): string { return value == null ? '余额暂不可用' : `$${(value / 100).toFixed(2)}` }
function closeDropdown(): void { dropdownOpen.value = false }
function outsideClick(event: MouseEvent): void { if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) closeDropdown() }
function replayOnboarding(): void { closeDropdown(); replayAgentOnboarding() }

async function refreshBalance(): Promise<void> {
  const request = ++balanceRequest
  try {
    const context = await agentAPI.getContext()
    if (request !== balanceRequest) return
    balanceCents.value = context.balance_error ? null : context.user?.balance_cents ?? null
    frozenBalanceCents.value = context.balance_error ? 0 : Math.max(0, context.user?.frozen_balance_cents ?? 0)
  } catch {
    if (request === balanceRequest) {
      balanceCents.value = null
      frozenBalanceCents.value = 0
    }
  }
}

function handleAccountFactsChanged(detail: AgentAccountFactsChangedDetail): void {
  if (detail.refreshBalance === false) return
  if (Number.isFinite(detail.balanceCents)) balanceCents.value = Number(detail.balanceCents)
  void refreshBalance()
}

async function logout(): Promise<void> {
  if (signingOut.value) return
  signingOut.value = true
  error.value = ''
  try { await session.logout(); await router.replace('/home') }
  catch { error.value = '退出失败，请重试。' }
  finally { signingOut.value = false; closeDropdown() }
}

onMounted(async () => {
  document.addEventListener('click', outsideClick)
  stopAccountFactsListener = onAgentAccountFactsChanged(handleAccountFactsChanged)
  await refreshBalance()
})
onBeforeUnmount(() => {
  balanceRequest += 1
  document.removeEventListener('click', outsideClick)
  stopAccountFactsListener?.()
  stopAccountFactsListener = null
})
</script>

<template>
  <header class="glass sticky top-0 z-30 border-b border-gray-200/50 dark:border-dark-700/50">
    <div class="flex h-16 items-center justify-between gap-2 px-2 sm:px-4 md:px-6">
      <div class="flex min-w-0 items-center gap-2 sm:gap-4"><button type="button" class="btn-ghost btn-icon lg:hidden" aria-label="打开菜单" aria-controls="agent-sidebar" @click.stop="emit('open-mobile')"><Icon name="menu" /></button><div class="min-w-0"><h1 class="truncate text-base font-semibold text-gray-900 dark:text-white sm:text-lg">{{ pageTitle }}</h1><p v-if="pageDescription" class="hidden max-w-[34rem] truncate text-xs text-gray-500 dark:text-dark-400 lg:block" data-testid="header-page-description">{{ pageDescription }}</p></div></div>
      <div class="flex min-w-0 items-center gap-1 sm:gap-3">
        <AgentAnnouncementBell />
        <a v-if="siteDocURL" :href="siteDocURL" target="_blank" rel="noopener noreferrer" class="hidden items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white sm:flex"><Icon name="book" size="sm" /><span class="hidden md:inline">文档</span></a>
        <div class="group relative hidden items-center gap-2 rounded-xl bg-primary-50 px-3 py-1.5 dark:bg-primary-900/20 sm:flex" data-testid="header-balance"><Icon name="creditCard" size="sm" class="text-primary-600 dark:text-primary-400" /><span class="text-sm font-semibold text-primary-700 dark:text-primary-300">{{ formatMoney(balanceCents) }}</span><span v-if="frozenBalanceCents > 0" class="rounded-full bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-200">冻结 {{ formatMoney(frozenBalanceCents) }}</span><div class="pointer-events-none absolute right-0 top-full mt-2 hidden w-60 rounded-lg border border-gray-200 bg-white p-3 text-xs shadow-lg group-hover:block dark:border-dark-700 dark:bg-dark-800" data-testid="header-balance-details"><div class="flex items-center justify-between"><span class="text-gray-500 dark:text-dark-400">可用余额</span><span class="font-medium text-gray-900 dark:text-white">{{ formatMoney(balanceCents) }}</span></div><div class="mt-2 flex items-center justify-between"><span class="text-gray-500 dark:text-dark-400">冻结余额</span><span class="font-medium text-amber-700 dark:text-amber-200">{{ formatMoney(frozenBalanceCents) }}</span></div><div class="mt-2 border-t border-gray-100 pt-2 dark:border-dark-700"><div class="flex items-center justify-between"><span class="text-gray-500 dark:text-dark-400">总余额</span><span class="font-semibold text-gray-900 dark:text-white">{{ formatMoney(totalBalanceCents) }}</span></div><p class="mt-2 leading-5 text-gray-500 dark:text-dark-400">余额与实际用量会自动同步更新。</p></div></div></div>
        <div ref="dropdownRef" class="relative">
          <button type="button" class="flex items-center gap-2 rounded-xl p-1.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-800" aria-label="用户菜单" data-tour="header-user-menu" @click.stop="dropdownOpen = !dropdownOpen"><div class="flex h-8 w-8 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 text-sm font-medium text-white shadow-sm"><img v-if="session.user?.avatar_url" :src="session.user.avatar_url" :alt="displayName" class="h-full w-full object-cover"><span v-else>{{ userInitials }}</span></div><div class="hidden text-left md:block"><div class="max-w-40 truncate text-sm font-medium text-gray-900 dark:text-white">{{ displayName }}</div><div class="text-xs text-gray-500 dark:text-dark-400">{{ roleLabel }}</div></div><Icon name="chevronDown" size="sm" class="hidden text-gray-400 md:block" /></button>
          <transition name="dropdown"><div v-if="dropdownOpen" class="dropdown right-0 mt-2 w-60"><div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700"><div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ displayName }}</div><div class="truncate text-xs text-gray-500 dark:text-dark-400">{{ session.user?.email }}</div></div><div class="border-b border-gray-100 px-4 py-2 sm:hidden dark:border-dark-700"><div class="text-xs text-gray-500">可用余额</div><div class="text-sm font-semibold text-primary-600">{{ formatMoney(balanceCents) }}</div><div v-if="frozenBalanceCents > 0" class="mt-1 text-xs text-amber-600 dark:text-amber-300">冻结余额 {{ formatMoney(frozenBalanceCents) }}</div></div><div class="py-1"><router-link to="/keys" class="dropdown-item" @click="closeDropdown"><Icon name="key" size="sm" />API 密钥</router-link><router-link v-if="session.isAgentAdmin" to="/admin/dashboard" class="dropdown-item" @click="closeDropdown"><Icon name="cog" size="sm" />站点管理</router-link></div><div v-if="agentOnboardingReplayAvailable" class="border-t border-gray-100 py-1 dark:border-dark-700"><button type="button" class="dropdown-item w-full" data-testid="replay-onboarding" @click="replayOnboarding"><Icon name="sparkles" size="sm" />重新查看新手引导</button></div><div v-if="siteContactInfo" class="border-t border-gray-100 px-4 py-2.5 dark:border-dark-700" data-testid="header-contact-info"><div class="flex items-start gap-2 text-xs text-gray-500 dark:text-gray-400"><Icon name="messageCircle" size="sm" class="mt-0.5 shrink-0" /><span>联系客服：</span><span class="min-w-0 break-words font-medium text-gray-700 dark:text-gray-300">{{ siteContactInfo }}</span></div></div><div class="border-t border-gray-100 py-1 dark:border-dark-700"><button type="button" class="dropdown-item w-full text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" :disabled="signingOut" aria-label="退出登录" @click="logout"><Icon name="login" size="sm" />{{ signingOut ? '正在退出…' : '退出登录' }}</button></div></div></transition>
        </div>
      </div>
    </div>
    <p v-if="error" role="alert" class="border-t border-red-100 bg-red-50 px-4 py-2 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">{{ error }}</p>
  </header>
</template>
