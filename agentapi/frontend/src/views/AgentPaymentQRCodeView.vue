<script setup lang="ts">
import QRCode from 'qrcode'
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
const qrCanvas = ref<HTMLCanvasElement | null>(null)
const remainingSeconds = ref(0)
const loading = ref(true)
const verifying = ref(false)
const cancelling = ref(false)
const terminal = ref(false)
const error = ref('')
let pollTimer: number | null = null
let countdownTimer: number | null = null

const payURL = computed(() => safePaymentURL(snapshot.value?.payURL))
const paymentLabel = computed(() => {
  const method = snapshot.value?.paymentType || ''
  if (method.startsWith('alipay')) return '支付宝'
  if (method.startsWith('wxpay')) return '微信支付'
  return method || '扫码支付'
})
const countdown = computed(() => {
  const minutes = Math.floor(remainingSeconds.value / 60).toString().padStart(2, '0')
  const seconds = (remainingSeconds.value % 60).toString().padStart(2, '0')
  return `${minutes}:${seconds}`
})

function cleanup(): void {
  if (pollTimer !== null) window.clearInterval(pollTimer)
  if (countdownTimer !== null) window.clearInterval(countdownTimer)
  pollTimer = null
  countdownTimer = null
}

async function renderQR(): Promise<void> {
  await nextTick()
  if (!qrCanvas.value || !snapshot.value?.qrCode) return
  await QRCode.toCanvas(qrCanvas.value, snapshot.value.qrCode, {
    width: 256,
    margin: 2,
    errorCorrectionLevel: 'M',
    color: { dark: '#111827', light: '#ffffff' },
  })
}

async function verify(showError = false): Promise<void> {
  if (!snapshot.value?.outTradeNo || verifying.value || terminal.value) return
  verifying.value = true
  try {
    const order = await agentAPI.payment.verifyOrder(snapshot.value.outTradeNo)
    if (['PAID', 'RECHARGING', 'COMPLETED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUNDED'].includes(order.status)) {
      cleanup()
      clearAgentPaymentRecovery(window.localStorage)
      await router.replace({ path: '/payment/result', query: { out_trade_no: order.out_trade_no } })
    } else if (['FAILED', 'EXPIRED', 'CANCELLED'].includes(order.status)) {
      cleanup()
      terminal.value = true
      error.value = `订单状态：${order.status}`
    }
  } catch (reason) {
    if (showError) error.value = errorMessage(reason, '暂时无法确认订单状态。')
  } finally {
    verifying.value = false
  }
}

async function cancelOrder(): Promise<void> {
  if (!snapshot.value || cancelling.value) return
  cancelling.value = true
  error.value = ''
  try {
    await agentAPI.orders.cancel(snapshot.value.orderId)
    cleanup()
    clearAgentPaymentRecovery(window.localStorage)
    await router.replace('/purchase')
  } catch (reason) {
    error.value = errorMessage(reason, '取消订单失败。')
  } finally {
    cancelling.value = false
  }
}

function startTimers(): void {
  if (!snapshot.value) return
  const expiration = Date.parse(snapshot.value.expiresAt)
  remainingSeconds.value = Number.isFinite(expiration)
    ? Math.max(0, Math.floor((expiration - Date.now()) / 1000))
    : 30 * 60
  if (remainingSeconds.value <= 0) {
    terminal.value = true
    error.value = '该订单已经过期。'
    return
  }
  countdownTimer = window.setInterval(() => {
    remainingSeconds.value = Math.max(0, remainingSeconds.value - 1)
    if (remainingSeconds.value === 0) {
      terminal.value = true
      error.value = '该订单已经过期。'
      cleanup()
    }
  }, 1000)
  pollTimer = window.setInterval(() => void verify(false), 3000)
}

onMounted(async () => {
  const orderId = Number(route.query.order_id) || undefined
  const outTradeNo = typeof route.query.out_trade_no === 'string' ? route.query.out_trade_no : undefined
  snapshot.value = readAgentPaymentRecovery(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY), { orderId, outTradeNo })
  if (!snapshot.value || (!snapshot.value.qrCode && !safePaymentURL(snapshot.value.payURL))) {
    error.value = '支付信息已失效，请返回购买页重新创建订单。'
    loading.value = false
    return
  }
  loading.value = false
  await renderQR()
  startTimers()
})

onBeforeUnmount(cleanup)
</script>

<template>
  <main class="mx-auto max-w-lg space-y-5 py-4">
    <header class="text-center">
      <p class="text-sm font-semibold text-primary-600 dark:text-primary-400">MAIN-SITE PAYMENT</p>
      <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">完成 {{ paymentLabel }}</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">支付状态由支付服务确认，页面会自动更新当前订单。</p>
    </header>

    <section v-if="loading" class="card flex min-h-72 items-center justify-center">
      <span class="h-9 w-9 animate-spin rounded-full border-4 border-primary-500/20 border-t-primary-500"></span>
    </section>

    <section v-else class="card p-6 text-center">
      <div v-if="error" role="alert" class="mb-5 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">{{ error }}</div>
      <template v-if="snapshot">
        <div v-if="snapshot.qrCode" class="mx-auto w-fit rounded-2xl bg-white p-5 shadow-sm ring-1 ring-gray-100">
          <canvas ref="qrCanvas" aria-label="支付二维码"></canvas>
        </div>
        <div v-else class="mx-auto flex h-40 w-40 items-center justify-center rounded-full bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
          <Icon name="creditCard" size="xl" />
        </div>
        <p class="mt-5 text-sm text-gray-500 dark:text-dark-400">{{ snapshot.qrCode ? `请使用${paymentLabel}扫码` : '请在支付窗口完成付款' }}</p>
        <p class="mt-2 text-3xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdown }}</p>
        <p class="mt-2 break-all font-mono text-xs text-gray-400">{{ snapshot.outTradeNo }}</p>
        <a v-if="payURL && !terminal" class="btn btn-primary mt-5 w-full" :href="payURL" target="_blank" rel="noopener noreferrer">打开支付窗口</a>
        <button class="btn btn-primary mt-3 w-full" type="button" :disabled="verifying || terminal" @click="verify(true)">
          <Icon name="refresh" size="sm" :class="['mr-2', verifying ? 'animate-spin' : '']" />{{ verifying ? '正在确认…' : '我已完成支付' }}
        </button>
        <button class="btn btn-secondary mt-3 w-full" type="button" :disabled="cancelling || terminal" @click="cancelOrder">{{ cancelling ? '正在取消…' : '取消订单' }}</button>
      </template>
      <RouterLink v-else class="btn btn-primary" to="/purchase">返回购买页</RouterLink>
    </section>
  </main>
</template>
