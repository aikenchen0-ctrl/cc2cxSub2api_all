<script setup lang="ts">
import QRCode from 'qrcode'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  agentAPI,
  type AgentCheckoutInfo,
  type AgentCheckoutPlan,
  type AgentContextResponse,
  type AgentPaymentCreateResult,
} from '@/agent/api'
import { errorMessage } from '@/agent/client'
import {
  clearAgentPaymentRecovery,
  createAgentPaymentRecovery,
  AGENT_PAYMENT_RECOVERY_KEY,
  readAgentPaymentRecovery,
  writeAgentPaymentRecovery,
} from '@/agent/paymentFlow'
import {
  classifyWechatPaymentResult,
  invokeWechatJsapiPayment,
  isWechatBrowser,
} from '@/agent/wechatPayment'
import AgentInput from '@/components/common/AgentInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentSubscriptionPlanCard from '@/components/payment/AgentSubscriptionPlanCard.vue'
import Icon from '@/components/icons/Icon.vue'

type CheckoutTab = 'balance' | 'subscription'

const router = useRouter()
const route = useRoute()
const checkout = ref<AgentCheckoutInfo | null>(null)
const context = ref<AgentContextResponse | null>(null)
const loading = ref(true)
const submitting = ref(false)
const verifying = ref(false)
const error = ref('')
const activeTab = ref<CheckoutTab>('balance')
const amount = ref('50')
const selectedMethod = ref('')
const selectedPlan = ref<AgentCheckoutPlan | null>(null)
const payment = ref<AgentPaymentCreateResult | null>(null)
const qrCanvas = ref<HTMLCanvasElement | null>(null)
const pollTimer = ref<number | null>(null)
const wechatResumeToken = ref('')
const wechatOpenID = ref('')
const resumingWechat = ref(false)
const renewalGroup = ref<number | null>(null)
const renewalNotice = ref('')
const showRenewalModal = ref(false)
const renewalPlans = computed(() => (checkout.value?.plans || []).filter(plan => renewalGroup.value === null || plan.group_id === renewalGroup.value))
const presets = [10, 20, 50, 100, 200, 500]

const siteName = computed(() => context.value?.agent.site_name || 'AgentAPI')
const balance = computed(() => {
  if (!context.value?.user || context.value.balance_error) return ''
  return `$${(Math.max(0, context.value.user.balance_cents) / 100).toFixed(2)}`
})
const methods = computed(() => Object.entries(checkout.value?.methods || {})
  .filter(([, config]) => config.available !== false)
  .sort(([left], [right]) => methodOrder(left) - methodOrder(right)))
const selectedLimit = computed(() => checkout.value?.methods[selectedMethod.value])
const parsedAmount = computed(() => Number(amount.value))
const currency = computed(() => selectedLimit.value?.currency || 'CNY')
const feeRate = computed(() => checkout.value?.recharge_fee_rate || selectedLimit.value?.fee_rate || 0)
const currentBaseAmount = computed(() => activeTab.value === 'subscription'
  ? subscriptionPaymentAmount(selectedPlan.value)
  : parsedAmount.value)
const feeAmount = computed(() => currentBaseAmount.value > 0
  ? Math.ceil(currentBaseAmount.value * feeRate.value) / 100
  : 0)
const totalAmount = computed(() => currentBaseAmount.value + feeAmount.value)
const hasBalance = computed(() => Boolean(checkout.value && !checkout.value.balance_disabled))
const hasSubscriptions = computed(() => (checkout.value?.plans.length || 0) > 0)
const safePayURL = computed(() => safeExternalURL(payment.value?.pay_url))
const paymentPending = computed(() => Boolean(payment.value?.out_trade_no))

const amountError = computed(() => {
  if (activeTab.value !== 'balance') return ''
  const value = parsedAmount.value
  if (!Number.isFinite(value) || value <= 0) return '请输入有效的充值金额。'
  const limit = selectedLimit.value
  if (!limit) return '请选择支付方式。'
  const globalMin = checkout.value?.global_min || 0
  const globalMax = checkout.value?.global_max || 0
  if (globalMin > 0 && value < globalMin) return `主站最低充值金额为 ${formatMoney(globalMin, currency.value)}`
  if (globalMax > 0 && value > globalMax) return `主站最高充值金额为 ${formatMoney(globalMax, currency.value)}`
  if (limit.single_min > 0 && value < limit.single_min) return `单笔最低 ${formatMoney(limit.single_min, currency.value)}`
  if (limit.single_max > 0 && value > limit.single_max) return `单笔最高 ${formatMoney(limit.single_max, currency.value)}`
  if (limit.daily_limit > 0 && limit.daily_remaining >= 0 && value > limit.daily_remaining) return `今日该支付方式剩余额度为 ${formatMoney(limit.daily_remaining, currency.value)}`
  return ''
})

const canSubmit = computed(() => {
  if (loading.value || !selectedMethod.value || !selectedLimit.value || selectedLimit.value.available === false || submitting.value) return false
  if (activeTab.value === 'balance') return hasBalance.value && amountError.value === ''
  return selectedPlan.value !== null && renewalPlans.value.some(plan => plan.id === selectedPlan.value?.id) && currentBaseAmount.value > 0
})

function methodOrder(type: string): number {
  const order = ['alipay', 'alipay_direct', 'wxpay', 'wxpay_direct', 'stripe', 'airwallex', 'easypay']
  const index = order.indexOf(type)
  return index < 0 ? 999 : index
}

function methodLabel(type: string, displayName?: string): string {
  if (displayName) return displayName
  if (type.startsWith('alipay')) return '支付宝'
  if (type.startsWith('wxpay')) return '微信支付'
  if (type === 'stripe') return 'Stripe'
  if (type === 'airwallex') return 'Airwallex'
  return type || '请选择'
}

function methodTone(type: string): string {
  if (type.startsWith('alipay')) return 'border-sky-500 bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300'
  if (type.startsWith('wxpay')) return 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
  if (type === 'stripe') return 'border-indigo-500 bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
  return 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
}

function methodIcon(type: string): 'creditCard' | 'qrCode' {
  return type === 'stripe' || type === 'airwallex' ? 'creditCard' : 'qrCode'
}

function subscriptionPaymentAmount(plan: AgentCheckoutPlan | null): number {
  if (!plan) return 0
  const rate = checkout.value?.subscription_usd_to_cny_rate || 0
  return currency.value.toUpperCase() === 'CNY' && rate > 0
    ? Math.round(plan.price * rate * 100) / 100
    : plan.price
}

function formatMoney(value: number, unit = 'CNY'): string {
  const code = (unit || 'CNY').toUpperCase()
  try {
    return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: code }).format(value)
  } catch {
    return `${value.toFixed(2)} ${code}`
  }
}

function safeExternalURL(value?: string): string {
  if (!value) return ''
  try {
    const parsed = new URL(value, window.location.origin)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : ''
  } catch {
    return ''
  }
}

function saveRecovery(result: AgentPaymentCreateResult): void {
  writeAgentPaymentRecovery(window.localStorage, createAgentPaymentRecovery(
    result,
    activeTab.value,
    selectedMethod.value,
    Date.now(),
    selectedPlan.value?.id || 0,
  ))
}

function clearRecovery(): void {
  clearAgentPaymentRecovery(window.localStorage)
}

async function renderQRCode(): Promise<void> {
  await nextTick()
  if (!qrCanvas.value || !payment.value?.qr_code) return
  await QRCode.toCanvas(qrCanvas.value, payment.value.qr_code, {
    width: 220,
    margin: 1,
    color: { dark: '#111827', light: '#ffffff' },
  })
}

function stopPolling(): void {
  if (pollTimer.value !== null) {
    window.clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

async function verifyPayment(showError = false): Promise<void> {
  const outTradeNo = payment.value?.out_trade_no
  if (!outTradeNo || verifying.value) return
  verifying.value = true
  try {
    const order = await agentAPI.payment.verifyOrder(outTradeNo)
    if (['PAID', 'RECHARGING', 'COMPLETED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUNDED'].includes(order.status)) {
      stopPolling()
      clearRecovery()
      await router.push({ path: '/payment/result', query: { out_trade_no: outTradeNo } })
    } else if (['FAILED', 'EXPIRED', 'CANCELLED'].includes(order.status)) {
      stopPolling()
      error.value = `订单状态：${order.status}，请重新创建订单。`
    }
  } catch (reason) {
    if (showError) error.value = errorMessage(reason, '暂时无法确认支付状态，请稍后重试。')
  } finally {
    verifying.value = false
  }
}

function startPolling(): void {
  stopPolling()
  if (!payment.value?.out_trade_no) return
  pollTimer.value = window.setInterval(() => void verifyPayment(false), 3000)
}

async function submit(forceResume = false): Promise<void> {
  if (!canSubmit.value && !forceResume) return
  submitting.value = true
  error.value = ''
  try {
    const plan = selectedPlan.value
    const resumeToken = wechatResumeToken.value
    const openid = wechatOpenID.value
    const result = await agentAPI.payment.createOrder({
      amount: activeTab.value === 'balance' ? parsedAmount.value : (plan?.price || 0),
      payment_type: selectedMethod.value,
      order_type: activeTab.value,
      ...(activeTab.value === 'subscription' && plan ? { plan_id: plan.id } : {}),
      ...(resumeToken ? { wechat_resume_token: resumeToken } : {}),
      ...(!resumeToken && openid ? { openid } : {}),
      is_mobile: /Android|iPhone|iPad|Mobile/i.test(window.navigator.userAgent),
      is_wechat_browser: isWechatBrowser(),
    })
    wechatResumeToken.value = ''
    wechatOpenID.value = ''
    saveRecovery(result)
    const oauthURL = safeExternalURL(result.oauth?.authorize_url)
    if (result.result_type === 'oauth_required' && oauthURL) {
      window.location.assign(oauthURL)
      return
    }
    const jsapiPayload = result.jsapi || result.jsapi_payload
    if (result.result_type === 'jsapi_ready' && jsapiPayload) {
      payment.value = result
      startPolling()
      try {
        const outcome = classifyWechatPaymentResult(await invokeWechatJsapiPayment(jsapiPayload))
        if (outcome === 'success') {
          await router.push({
            path: '/payment/result',
            query: { order_id: String(result.order_id), out_trade_no: result.out_trade_no || undefined },
          })
        } else if (outcome === 'cancelled') {
          error.value = '微信支付已取消。订单仍保留，可重新发起支付或到订单记录中查看状态。'
        } else {
          error.value = '微信支付未完成。请返回重新选择支付方式，或到订单记录中确认订单状态。'
        }
      } catch (reason) {
        error.value = errorMessage(reason, '未检测到微信支付组件。请在微信内打开本站，或返回选择其他支付方式。')
      }
      return
    }
    if (result.client_secret && selectedMethod.value === 'airwallex' && result.intent_id) {
      await router.push({
        path: '/payment/airwallex',
        query: { order_id: String(result.order_id), out_trade_no: result.out_trade_no || undefined },
      })
      return
    }
    if (result.client_secret) {
      const method = selectedMethod.value.startsWith('wxpay')
        ? 'wechat_pay'
        : selectedMethod.value.startsWith('alipay') ? 'alipay' : undefined
      await router.push({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          method,
        },
      })
      return
    }
    if (result.qr_code || result.pay_url) {
      await router.push({
        path: '/payment/qrcode',
        query: { order_id: String(result.order_id), out_trade_no: result.out_trade_no || undefined },
      })
      return
    }
    payment.value = result
    await renderQRCode()
    startPolling()
    const payURL = safeExternalURL(result.pay_url)
    if (!result.qr_code && payURL) {
      const popup = window.open(payURL, 'agentapiPayment', 'width=520,height=720,noopener,noreferrer')
      if (!popup) window.location.assign(payURL)
    }
  } catch (reason) {
    error.value = errorMessage(reason, '创建主站支付订单失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}

function queryValue(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function positiveNumber(value: string): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function applyPurchaseNavigation(): void {
  if (!checkout.value || queryValue('wechat_resume') === '1' || submitting.value || payment.value) return
  if (queryValue('tab') !== 'subscription') return
  activeTab.value = 'subscription'
  showRenewalModal.value = false
  renewalNotice.value = ''
  renewalGroup.value = null
  if (!queryValue('group')) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === selectedPlan.value?.id) || checkout.value.plans[0] || null
    return
  }
  const group = Number(queryValue('group'))
  selectedPlan.value = null
  if (!Number.isSafeInteger(group) || group <= 0) {
    renewalGroup.value = -1
    renewalNotice.value = '续费分组无效，请重新选择套餐。'
    return
  }
  renewalGroup.value = group
  const plans = renewalPlans.value
  if (plans.length === 1) selectedPlan.value = plans[0]
  else if (plans.length > 1) showRenewalModal.value = true
  else renewalNotice.value = '该订阅分组当前没有可购买的续费套餐。你可以查看其他套餐，或稍后重试。'
}

function selectPlan(plan: AgentCheckoutPlan): void {
  if (submitting.value) return
  selectedPlan.value = renewalPlans.value.find(item => item.id === plan.id) || null
  showRenewalModal.value = false
}

async function showAllPlans(): Promise<void> {
  renewalGroup.value = null
  renewalNotice.value = ''
  showRenewalModal.value = false
  selectedPlan.value = null
  const query = { ...route.query }
  delete query.group
  delete query.tab
  await router.replace({ path: route.path, query })
}

async function resumeWechatPayment(): Promise<void> {
  if (queryValue('wechat_resume') !== '1' || resumingWechat.value || !checkout.value) return

  const resumeToken = queryValue('wechat_resume_token')
  const openid = queryValue('openid')
  if (!resumeToken && !openid) {
    error.value = '微信支付恢复信息无效，请重新发起支付。'
    return
  }

  const snapshot = readAgentPaymentRecovery(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY))
  const requestedMethod = queryValue('payment_type') || snapshot?.paymentType || ''
  const availableWechatMethods = methods.value.map(([type]) => type).filter(type => type.startsWith('wxpay'))
  const nextMethod = availableWechatMethods.includes(requestedMethod) ? requestedMethod : availableWechatMethods[0]
  if (!nextMethod) {
    error.value = '主站当前没有可用的微信支付方式，请选择其他支付方式。'
    return
  }

  const requestedOrderType = queryValue('order_type') || snapshot?.orderType || 'balance'
  activeTab.value = requestedOrderType === 'subscription' ? 'subscription' : 'balance'
  selectedMethod.value = nextMethod

  const restoredAmount = positiveNumber(queryValue('amount')) || snapshot?.amount || 0
  if (restoredAmount > 0) amount.value = String(restoredAmount)

  const restoredPlanID = Math.trunc(positiveNumber(queryValue('plan_id')) || snapshot?.planId || 0)
  if (activeTab.value === 'subscription') {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === restoredPlanID) || null
    if (!selectedPlan.value) {
      await router.replace('/purchase')
      error.value = '原订阅套餐已不可用，请重新选择套餐后支付。'
      return
    }
  }

  wechatResumeToken.value = resumeToken
  wechatOpenID.value = resumeToken ? '' : openid
  resumingWechat.value = true
  await router.replace('/purchase')
  try {
    await submit(true)
  } finally {
    resumingWechat.value = false
  }
}

function resetPayment(): void {
  stopPolling()
  payment.value = null
  error.value = ''
  clearRecovery()
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [checkoutInfo, agentContext] = await Promise.all([
      agentAPI.payment.checkoutInfo(),
      agentAPI.getContext(),
    ])
    checkout.value = checkoutInfo
    context.value = agentContext
    const firstMethod = Object.entries(checkoutInfo.methods)
      .filter(([, item]) => item.available !== false)
      .sort(([left], [right]) => methodOrder(left) - methodOrder(right))[0]?.[0]
    selectedMethod.value = firstMethod || ''
    if (checkoutInfo.balance_disabled && checkoutInfo.plans.length) activeTab.value = 'subscription'
    selectedPlan.value = checkoutInfo.plans.find(plan => plan.id === selectedPlan.value?.id) || checkoutInfo.plans[0] || null
    applyPurchaseNavigation()
    await resumeWechatPayment()
  } catch (reason) {
    checkout.value = null
    selectedPlan.value = null
    showRenewalModal.value = false
    error.value = errorMessage(reason, '加载主站支付配置失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

watch(() => payment.value?.qr_code, () => void renderQRCode())
watch(activeTab, () => { error.value = '' })
watch(() => [route.query.tab, route.query.group], () => {
  if (!loading.value) applyPurchaseNavigation()
})
onMounted(load)
onBeforeUnmount(stopPolling)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ siteName }}</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">购买与充值</h1>
        <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">页面沿用 Sub2API 主站的支付目录和订单流程。余额、套餐、支付订单与到账结果均由主站负责，本站只以当前登录用户身份安全中转。</p>
      </div>
      <div class="flex items-center gap-3">
        <div class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-2 text-right dark:border-emerald-500/20 dark:bg-emerald-500/10">
          <p class="text-[11px] text-emerald-700/70 dark:text-emerald-300/70">主站可用余额</p>
          <p class="text-lg font-bold text-emerald-700 dark:text-emerald-300">{{ balance || '暂不可用' }}</p>
        </div>
        <button class="btn btn-secondary" type="button" :disabled="loading" aria-label="刷新支付配置" @click="load">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </header>

    <div v-if="loading" role="status" class="card flex min-h-80 items-center justify-center">
      <div class="text-center"><span class="mx-auto block h-9 w-9 animate-spin rounded-full border-4 border-primary-500/20 border-t-primary-500"></span><p class="mt-3 text-sm text-gray-500">正在读取主站支付配置…</p></div>
    </div>

    <template v-else>
      <div v-if="error" role="alert" class="flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
        <Icon name="exclamationCircle" size="sm" class="mt-0.5 shrink-0" /><span>{{ error }}</span>
      </div>

      <section v-if="!checkout" class="card p-8 text-center text-sm text-gray-500">主站支付配置暂不可用。</section>

      <section v-else-if="payment" class="card overflow-hidden" data-testid="payment-status">
        <div class="border-b border-gray-100 px-6 py-5 dark:border-dark-700">
          <p class="text-xs font-semibold uppercase tracking-wider text-primary-600 dark:text-primary-400">等待支付</p>
          <h2 class="mt-1 text-xl font-bold text-gray-900 dark:text-white">请完成 {{ methodLabel(payment.payment_type || selectedMethod) }}</h2>
        </div>
        <div class="grid gap-8 p-6 md:grid-cols-[280px_1fr]">
          <div class="flex min-h-64 items-center justify-center rounded-2xl bg-gray-50 p-5 dark:bg-dark-800">
            <canvas v-if="payment.qr_code" ref="qrCanvas" aria-label="支付二维码" class="max-w-full rounded-lg bg-white p-2"></canvas>
            <div v-else class="text-center">
              <Icon name="creditCard" size="xl" class="mx-auto text-primary-500" />
              <p class="mt-3 text-sm text-gray-500">支付窗口已打开</p>
              <a v-if="safePayURL" class="btn btn-primary mt-4" :href="safePayURL" target="_blank" rel="noopener noreferrer">重新打开支付页</a>
            </div>
          </div>
          <div class="space-y-5">
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><p class="text-xs text-gray-400">应付金额</p><p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{{ formatMoney(payment.pay_amount, payment.currency || currency) }}</p></div>
              <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><p class="text-xs text-gray-400">订单号</p><p class="mt-1 break-all font-mono text-sm font-semibold text-gray-800 dark:text-dark-100">{{ payment.out_trade_no || payment.order_id }}</p></div>
            </div>
            <div class="rounded-xl bg-blue-50 p-4 text-sm leading-6 text-blue-800 dark:bg-blue-500/10 dark:text-blue-200">支付完成后页面会自动向主站确认订单。你也可以点击“我已完成支付”立即验单；到账结果只认主站订单状态。</div>
            <div class="flex flex-wrap gap-3">
              <button class="btn btn-primary" type="button" :disabled="verifying || !paymentPending" @click="verifyPayment(true)"><Icon name="refresh" size="sm" :class="['mr-2', verifying ? 'animate-spin' : '']" />{{ verifying ? '正在确认…' : '我已完成支付' }}</button>
              <button class="btn btn-secondary" type="button" @click="resetPayment">返回重新选择</button>
              <RouterLink class="btn btn-secondary" to="/orders">查看订单记录</RouterLink>
            </div>
          </div>
        </div>
      </section>

      <template v-else-if="checkout">
        <nav v-if="hasBalance && hasSubscriptions" class="flex w-fit rounded-xl bg-gray-100 p-1 dark:bg-dark-800" aria-label="购买类型">
          <button v-for="tab in ([['balance', '余额充值'], ['subscription', '订阅套餐']] as const)" :key="tab[0]" type="button" :class="['rounded-lg px-5 py-2 text-sm font-medium transition', activeTab === tab[0] ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-dark-300']" @click="activeTab = tab[0]">{{ tab[1] }}</button>
        </nav>

        <div class="grid gap-6 lg:grid-cols-[1fr_340px]">
          <section class="card p-6">
            <template v-if="activeTab === 'balance' && hasBalance">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">充值金额</h2>
              <div class="mt-4 grid grid-cols-3 gap-3 sm:grid-cols-6">
                <button v-for="preset in presets" :key="preset" type="button" :class="['rounded-xl border px-3 py-3 text-sm font-semibold transition', Number(amount) === preset ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300' : 'border-gray-200 text-gray-700 hover:border-primary-300 dark:border-dark-700 dark:text-dark-200']" @click="amount = String(preset)">¥{{ preset }}</button>
              </div>
              <label class="mt-5 block text-sm font-medium text-gray-700 dark:text-dark-200" for="recharge-amount">自定义金额</label>
              <div class="mt-2">
                <AgentInput id="recharge-amount" v-model="amount" class="text-lg font-semibold" type="number" min="0" step="0.01" inputmode="decimal">
                  <template #prefix><span>¥</span></template>
                </AgentInput>
              </div>
              <p v-if="amountError" class="mt-2 text-sm text-red-600 dark:text-red-300">{{ amountError }}</p>
              <p v-else class="mt-2 text-xs text-gray-400">充值倍率：{{ checkout.balance_recharge_multiplier || 1 }}×；最终余额由主站订单到账结果决定。</p>
            </template>

            <template v-else-if="activeTab === 'subscription'">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ renewalGroup !== null ? '选择续费套餐' : '选择订阅套餐' }}</h2>
                <button v-if="renewalGroup !== null" type="button" class="text-sm font-medium text-primary-600" :disabled="submitting" @click="showAllPlans">查看全部套餐</button>
              </div>
              <p v-if="renewalNotice" role="status" class="mt-4 rounded-xl bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-200">{{ renewalNotice }}</p>
              <p v-else-if="!renewalPlans.length" class="mt-4 text-sm text-gray-500">暂无可购买的订阅套餐。</p>
              <div class="mt-4 grid gap-4 md:grid-cols-2">
                <AgentSubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" :selected="selectedPlan?.id === plan.id" :disabled="submitting" @select="selectPlan" />
              </div>
            </template>

            <div class="mt-7 border-t border-gray-100 pt-6 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">支付方式</h2>
              <div v-if="methods.length" class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
                <button v-for="[type, item] in methods" :key="type" type="button" :class="['flex min-h-16 items-center gap-3 rounded-xl border px-4 py-3 text-left transition', selectedMethod === type ? methodTone(type) : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200']" @click="selectedMethod = type">
                  <Icon :name="methodIcon(type)" size="lg" class="shrink-0" /><span class="min-w-0"><span class="block truncate text-sm font-semibold">{{ methodLabel(type, item.display_name) }}</span><span v-if="item.fee_rate > 0" class="mt-1 block text-[11px] opacity-70">手续费 {{ item.fee_rate }}%</span></span>
                </button>
              </div>
              <p v-else class="mt-4 rounded-xl bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-200">主站当前没有可用支付方式。</p>
            </div>
          </section>

          <aside class="space-y-4">
            <section class="card p-5 lg:sticky lg:top-24">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">订单确认</h2>
              <dl class="mt-5 space-y-3 text-sm">
                <div class="flex justify-between gap-3"><dt class="text-gray-500">购买内容</dt><dd class="text-right font-medium text-gray-900 dark:text-white">{{ activeTab === 'balance' ? '余额充值' : (selectedPlan?.name || '请选择套餐') }}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-gray-500">支付方式</dt><dd class="font-medium text-gray-900 dark:text-white">{{ methodLabel(selectedMethod, selectedLimit?.display_name) }}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-gray-500">金额</dt><dd class="font-medium text-gray-900 dark:text-white">{{ formatMoney(currentBaseAmount || 0, currency) }}</dd></div>
                <div v-if="feeAmount > 0" class="flex justify-between gap-3"><dt class="text-gray-500">手续费（{{ feeRate }}%）</dt><dd class="font-medium text-gray-900 dark:text-white">{{ formatMoney(feeAmount, currency) }}</dd></div>
                <div class="border-t border-gray-100 pt-4 dark:border-dark-700"><div class="flex items-end justify-between"><dt class="font-medium text-gray-700 dark:text-dark-200">应付总额</dt><dd class="text-2xl font-bold text-primary-600 dark:text-primary-400">{{ formatMoney(totalAmount || 0, currency) }}</dd></div></div>
              </dl>
              <button class="btn btn-primary mt-6 w-full py-3 text-base" type="button" :disabled="!canSubmit" @click="submit()"><span v-if="submitting" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>{{ submitting ? '正在创建主站订单…' : '立即支付' }}</button>
              <p class="mt-3 text-center text-xs leading-5 text-gray-400">点击后由主站创建并计费订单，代理站不会维护第二套支付账本。</p>
            </section>
          </aside>
        </div>

        <section v-if="checkout.help_text || checkout.help_image_url" class="card p-5">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">支付帮助</h2>
          <p v-if="checkout.help_text" class="mt-2 whitespace-pre-line text-sm leading-6 text-gray-500 dark:text-dark-400">{{ checkout.help_text }}</p>
          <img v-if="safeExternalURL(checkout.help_image_url)" class="mt-4 max-h-80 rounded-xl object-contain" :src="safeExternalURL(checkout.help_image_url)" alt="支付帮助" />
        </section>
      </template>
    </template>
    <BaseDialog :show="showRenewalModal" title="选择续费套餐" @close="showRenewalModal = false">
      <div class="space-y-4">
        <AgentSubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" @select="selectPlan" />
      </div>
    </BaseDialog>
  </main>
</template>
