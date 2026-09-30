<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { errorMessage } from '@/agent/client'
import {
  AGENT_PAYMENT_RECOVERY_KEY,
  readAgentPaymentRecovery,
  type AgentPaymentRecoverySnapshot,
} from '@/agent/paymentFlow'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const loading = ref(true)
const error = ref('')
const snapshot = ref<AgentPaymentRecoverySnapshot | null>(null)

function successURL(state: AgentPaymentRecoverySnapshot): string {
  const url = new URL('/payment/result', window.location.origin)
  if (state.outTradeNo) url.searchParams.set('out_trade_no', state.outTradeNo)
  return url.toString()
}

onMounted(async () => {
  const orderId = Number(route.query.order_id) || undefined
  const outTradeNo = typeof route.query.out_trade_no === 'string' ? route.query.out_trade_no : undefined
  snapshot.value = readAgentPaymentRecovery(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY), { orderId, outTradeNo })
  if (!snapshot.value?.clientSecret || !snapshot.value.intentId || snapshot.value.paymentType !== 'airwallex') {
    error.value = 'Airwallex 支付信息已失效，请返回购买页重新创建订单。'
    loading.value = false
    return
  }
  try {
    const airwallex = await import('@airwallex/components-sdk')
    const initialized = await airwallex.init({
      env: snapshot.value.paymentEnv === 'prod' ? 'prod' : 'demo',
      enabledElements: ['payments'],
      locale: navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en',
    })
    if (!initialized.payments) throw new Error('Airwallex 支付组件不可用。')
    loading.value = false
    const redirect = initialized.payments.redirectToCheckout({
      intent_id: snapshot.value.intentId,
      client_secret: snapshot.value.clientSecret,
      currency: snapshot.value.currency || 'CNY',
      country_code: snapshot.value.countryCode || 'CN',
      successUrl: successURL(snapshot.value),
    })
    if (typeof redirect === 'string' && redirect) window.location.assign(redirect)
  } catch (reason) {
    error.value = errorMessage(reason, 'Airwallex 支付组件加载失败。')
    loading.value = false
  }
})
</script>

<template>
  <main class="mx-auto max-w-lg py-4">
    <section v-if="loading" class="card p-10 text-center">
      <span class="mx-auto block h-10 w-10 animate-spin rounded-full border-4 border-emerald-500/20 border-t-emerald-500"></span>
      <h1 class="mt-5 text-xl font-bold text-gray-900 dark:text-white">正在打开 Airwallex</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">订单由 Sub2API 主站创建，正在加载安全支付页面。</p>
    </section>
    <section v-else-if="error" class="card p-8 text-center">
      <span class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-red-100 text-red-600 dark:bg-red-500/10 dark:text-red-300"><Icon name="exclamationCircle" size="xl" /></span>
      <h1 class="mt-4 text-xl font-bold text-gray-900 dark:text-white">Airwallex 加载失败</h1>
      <p role="alert" class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ error }}</p>
      <RouterLink class="btn btn-primary mt-6" to="/purchase">返回购买页</RouterLink>
    </section>
    <section v-else class="card p-10 text-center">
      <span class="mx-auto block h-10 w-10 animate-spin rounded-full border-4 border-emerald-500/20 border-t-emerald-500"></span>
      <p class="mt-4 text-sm text-gray-500 dark:text-dark-400">支付窗口即将打开，请勿关闭当前页面。</p>
    </section>
  </main>
</template>
