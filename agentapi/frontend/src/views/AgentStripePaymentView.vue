<script setup lang="ts">
import type { Stripe, StripeElements, StripePaymentElement } from '@stripe/stripe-js'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { agentAPI } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import {
  AGENT_PAYMENT_RECOVERY_KEY,
  clearAgentPaymentRecovery,
  readAgentPaymentRecovery,
  safePaymentURL,
  type AgentPaymentRecoverySnapshot,
} from '@/agent/paymentFlow'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const router = useRouter()
const snapshot = ref<AgentPaymentRecoverySnapshot | null>(null)
const loading = ref(true)
const initError = ref('')
const paymentError = ref('')
const submitting = ref(false)
const ready = ref(false)
const success = ref(false)
const redirecting = ref(false)
const wechatQR = ref('')
let stripe: Stripe | null = null
let elements: StripeElements | null = null
let paymentElement: StripePaymentElement | null = null
let pollTimer: number | null = null

const method = computed(() => typeof route.query.method === 'string' ? route.query.method : '')
const showPaymentElement = computed(() => !method.value && !wechatQR.value && !redirecting.value && !success.value)
const formattedAmount = computed(() => {
  const value = snapshot.value?.payAmount || 0
  const currency = snapshot.value?.currency || 'CNY'
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(value) }
  catch { return `${value.toFixed(2)} ${currency}` }
})
const wechatQRSafe = computed(() => {
  if (wechatQR.value.startsWith('data:image/')) return wechatQR.value
  return safePaymentURL(wechatQR.value)
})

function resultURL(): string {
  const url = new URL('/payment/result', window.location.origin)
  if (snapshot.value?.outTradeNo) url.searchParams.set('out_trade_no', snapshot.value.outTradeNo)
  return url.toString()
}

async function finishWithoutRedirect(): Promise<void> {
  success.value = true
  clearAgentPaymentRecovery(window.localStorage)
  await router.replace({ path: '/payment/result', query: { out_trade_no: snapshot.value?.outTradeNo || undefined } })
}

async function verify(): Promise<void> {
  if (!snapshot.value?.outTradeNo) return
  try {
    const order = await agentAPI.payment.verifyOrder(snapshot.value.outTradeNo)
    if (['PAID', 'RECHARGING', 'COMPLETED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUNDED'].includes(order.status)) {
      if (pollTimer !== null) window.clearInterval(pollTimer)
      pollTimer = null
      await finishWithoutRedirect()
    }
  } catch {
    // Poll failures are transient; the explicit result page remains available.
  }
}

function startPolling(): void {
  if (pollTimer !== null) window.clearInterval(pollTimer)
  pollTimer = window.setInterval(() => void verify(), 3000)
}

async function confirmAlipay(instance: Stripe, clientSecret: string): Promise<void> {
  redirecting.value = true
  const { error } = await instance.confirmAlipayPayment(clientSecret, { return_url: resultURL() })
  if (error) {
    redirecting.value = false
    paymentError.value = error.message || '支付宝支付发起失败。'
  }
}

async function confirmWechat(instance: Stripe, clientSecret: string): Promise<void> {
  const stripeWechat = instance as Stripe & {
    confirmWechatPayPayment: (secret: string, options: Record<string, unknown>) => Promise<{
      paymentIntent?: { status: string; next_action?: { wechat_pay_display_qr_code?: { image_data_url?: string } } }
      error?: { message?: string }
    }>
  }
  const { paymentIntent, error } = await stripeWechat.confirmWechatPayPayment(clientSecret, {
    payment_method_options: { wechat_pay: { client: /Android|iPhone|iPad|Mobile/i.test(window.navigator.userAgent) ? 'mobile_web' : 'web' } },
  })
  if (error) {
    paymentError.value = error.message || '微信支付发起失败。'
    return
  }
  const qr = paymentIntent?.next_action?.wechat_pay_display_qr_code?.image_data_url || ''
  if (qr) {
    wechatQR.value = qr
    startPolling()
  } else if (paymentIntent?.status === 'succeeded') {
    await finishWithoutRedirect()
  } else {
    paymentError.value = '支付服务没有返回可用的微信支付二维码。'
  }
}

async function mountGenericElement(instance: Stripe, clientSecret: string): Promise<void> {
  await nextTick()
  const dark = document.documentElement.classList.contains('dark')
  elements = instance.elements({
    clientSecret,
    appearance: { theme: dark ? 'night' : 'stripe', variables: { borderRadius: '8px' } },
  })
  paymentElement = elements.create('payment', {
    layout: 'tabs',
    paymentMethodOrder: ['alipay', 'wechat_pay', 'card', 'link'],
  } as Record<string, unknown>)
  paymentElement.mount('#agent-stripe-payment-element')
  paymentElement.on('ready', () => { ready.value = true })
}

async function pay(): Promise<void> {
  if (!stripe || !elements || submitting.value) return
  submitting.value = true
  paymentError.value = ''
  try {
    const { error } = await stripe.confirmPayment({
      elements,
      confirmParams: { return_url: resultURL() },
      redirect: 'if_required',
    })
    if (error) paymentError.value = error.message || 'Stripe 支付失败。'
    else await finishWithoutRedirect()
  } catch (reason) {
    paymentError.value = errorMessage(reason, 'Stripe 支付失败。')
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  const orderId = Number(route.query.order_id) || undefined
  const outTradeNo = typeof route.query.out_trade_no === 'string' ? route.query.out_trade_no : undefined
  snapshot.value = readAgentPaymentRecovery(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY), { orderId, outTradeNo })
  if (!snapshot.value?.clientSecret) {
    initError.value = 'Stripe 支付信息已失效，请返回购买页重新创建订单。'
    loading.value = false
    return
  }
  try {
    const config = await agentAPI.payment.checkoutInfo()
    if (!config.stripe_publishable_key) throw new Error('本站未配置 Stripe 公钥。')
    const { loadStripe } = await import('@stripe/stripe-js/pure')
    stripe = await loadStripe(config.stripe_publishable_key)
    if (!stripe) throw new Error('Stripe 组件加载失败。')
    loading.value = false
    if (method.value === 'alipay') await confirmAlipay(stripe, snapshot.value.clientSecret)
    else if (method.value === 'wechat_pay') await confirmWechat(stripe, snapshot.value.clientSecret)
    else await mountGenericElement(stripe, snapshot.value.clientSecret)
  } catch (reason) {
    initError.value = errorMessage(reason, 'Stripe 组件加载失败。')
    loading.value = false
  }
})

onBeforeUnmount(() => {
  if (pollTimer !== null) window.clearInterval(pollTimer)
  paymentElement?.destroy()
})
</script>

<template>
  <main class="mx-auto max-w-lg space-y-5 py-4">
    <section v-if="loading" class="card flex min-h-72 items-center justify-center">
      <span class="h-9 w-9 animate-spin rounded-full border-4 border-indigo-500/20 border-t-indigo-500"></span>
    </section>
    <section v-else-if="initError" class="card p-8 text-center">
      <span class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-red-100 text-red-600 dark:bg-red-500/10 dark:text-red-300"><Icon name="exclamationCircle" size="xl" /></span>
      <h1 class="mt-4 text-xl font-bold text-gray-900 dark:text-white">Stripe 加载失败</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ initError }}</p>
      <RouterLink class="btn btn-primary mt-6" to="/purchase">返回购买页</RouterLink>
    </section>
    <template v-else>
      <section class="card overflow-hidden">
        <div class="bg-gradient-to-br from-[#635bff] to-[#4f46e5] px-6 py-6 text-center text-white">
          <p class="text-sm font-medium text-indigo-100">订单应付金额</p>
          <p class="mt-1 text-3xl font-bold">{{ formattedAmount }}</p>
          <p class="mt-2 break-all font-mono text-xs text-indigo-200">{{ snapshot?.outTradeNo }}</p>
        </div>
      </section>

      <section v-if="wechatQR" class="card p-6 text-center">
        <h1 class="text-lg font-bold text-gray-900 dark:text-white">使用微信扫描二维码</h1>
        <img v-if="wechatQRSafe" :src="wechatQRSafe" alt="微信支付二维码" class="mx-auto mt-5 h-64 w-64 rounded-xl bg-white p-3" />
        <p class="mt-4 text-sm text-gray-500 dark:text-dark-400">页面会自动确认支付结果。</p>
      </section>

      <section v-else-if="redirecting" class="card p-10 text-center">
        <span class="mx-auto block h-10 w-10 animate-spin rounded-full border-4 border-sky-500/20 border-t-sky-500"></span>
        <p class="mt-4 text-sm text-gray-500 dark:text-dark-400">正在打开支付宝支付页面…</p>
      </section>

      <section v-else-if="success" class="card p-8 text-center">
        <Icon name="checkCircle" size="xl" class="mx-auto text-emerald-500" />
        <h1 class="mt-3 text-xl font-bold text-gray-900 dark:text-white">支付已提交</h1>
      </section>

      <section v-else-if="showPaymentElement" class="card p-6">
        <h1 class="mb-5 text-lg font-bold text-gray-900 dark:text-white">选择 Stripe 支付方式</h1>
        <div id="agent-stripe-payment-element" class="min-h-48"></div>
        <p v-if="paymentError" role="alert" class="mt-4 text-sm text-red-600 dark:text-red-300">{{ paymentError }}</p>
        <button class="btn btn-primary mt-6 w-full py-3" type="button" :disabled="submitting || !ready" @click="pay">{{ submitting ? '正在提交…' : '立即支付' }}</button>
      </section>

      <section v-if="paymentError && !showPaymentElement" class="card p-4 text-center">
        <p role="alert" class="text-sm text-red-600 dark:text-red-300">{{ paymentError }}</p>
        <RouterLink class="btn btn-secondary mt-4" to="/purchase">返回购买页</RouterLink>
      </section>
    </template>
  </main>
</template>
