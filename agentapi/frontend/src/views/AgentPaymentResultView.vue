<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { notifyAgentAccountFactsChanged } from '@/agent/accountFacts'
import { agentAPI, type AgentOrder } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import { AGENT_PAYMENT_RECOVERY_KEY, clearAgentPaymentRecovery, readAgentPaymentRecovery } from '@/agent/paymentFlow'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const loading = ref(true)
const verifying = ref(false)
const order = ref<AgentOrder | null>(null)
const error = ref('')

const outTradeNo = computed(() => {
  const query = typeof route.query.out_trade_no === 'string' ? route.query.out_trade_no.trim() : ''
  if (query) return query
  const state = readAgentPaymentRecovery(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY))
  if (state?.outTradeNo) return state.outTradeNo
  try {
    const stored = JSON.parse(window.localStorage.getItem(AGENT_PAYMENT_RECOVERY_KEY) || '{}') as { out_trade_no?: string }
    return stored.out_trade_no || ''
  } catch {
    return ''
  }
})
const success = computed(() => Boolean(order.value && ['PAID', 'RECHARGING', 'COMPLETED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUNDED'].includes(order.value.status)))

async function verify(): Promise<void> {
  if (!outTradeNo.value) {
    error.value = '缺少可验证的主站订单号。'
    loading.value = false
    return
  }
  verifying.value = true
  error.value = ''
  try {
    order.value = await agentAPI.payment.verifyOrder(outTradeNo.value)
    if (success.value) {
      clearAgentPaymentRecovery(window.localStorage)
      notifyAgentAccountFactsChanged({
        refreshBalance: order.value.order_type !== 'subscription',
        refreshSubscriptions: order.value.order_type === 'subscription',
      })
    }
  } catch (reason) {
    error.value = errorMessage(reason, '暂时无法确认主站订单状态，请稍后重试。')
  } finally {
    verifying.value = false
    loading.value = false
  }
}

onMounted(verify)
</script>

<template>
  <main class="mx-auto max-w-2xl">
    <section class="card overflow-hidden text-center">
      <div :class="['px-6 py-12', success ? 'bg-emerald-50 dark:bg-emerald-500/10' : 'bg-gray-50 dark:bg-dark-800']">
        <span :class="['mx-auto flex h-16 w-16 items-center justify-center rounded-full', success ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-300' : 'bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-300']">
          <span v-if="loading" class="h-8 w-8 animate-spin rounded-full border-4 border-current/25 border-t-current"></span>
          <Icon v-else :name="success ? 'checkCircle' : 'clock'" size="xl" />
        </span>
        <h1 class="mt-5 text-2xl font-bold text-gray-900 dark:text-white">{{ loading ? '正在确认支付结果' : success ? '支付已确认' : '订单仍在处理中' }}</h1>
        <p class="mx-auto mt-3 max-w-lg text-sm leading-6 text-gray-500 dark:text-dark-400">{{ success ? '主站已经确认该订单。余额或订阅权益以主站最终到账记录为准。' : '如果你已经完成支付，可以再次向主站查询订单状态。' }}</p>
      </div>
      <div class="space-y-5 p-6">
        <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">{{ error }}</div>
        <dl v-if="order" class="grid gap-3 rounded-xl border border-gray-200 p-4 text-left text-sm dark:border-dark-700 sm:grid-cols-2">
          <div><dt class="text-xs text-gray-400">主站订单号</dt><dd class="mt-1 break-all font-mono font-medium text-gray-800 dark:text-dark-100">{{ order.out_trade_no }}</dd></div>
          <div><dt class="text-xs text-gray-400">订单状态</dt><dd class="mt-1 font-semibold text-gray-800 dark:text-dark-100">{{ order.status }}</dd></div>
          <div><dt class="text-xs text-gray-400">支付金额</dt><dd class="mt-1 font-semibold text-gray-800 dark:text-dark-100">{{ order.pay_amount.toFixed(2) }} {{ order.currency || 'CNY' }}</dd></div>
          <div><dt class="text-xs text-gray-400">订单类型</dt><dd class="mt-1 font-semibold text-gray-800 dark:text-dark-100">{{ order.order_type === 'subscription' ? '订阅套餐' : '余额充值' }}</dd></div>
        </dl>
        <div class="flex flex-wrap justify-center gap-3">
          <button class="btn btn-primary" type="button" :disabled="verifying" @click="verify"><Icon name="refresh" size="sm" :class="['mr-2', verifying ? 'animate-spin' : '']" />重新查询主站</button>
          <RouterLink class="btn btn-secondary" to="/orders">查看订单</RouterLink>
          <RouterLink class="btn btn-secondary" to="/purchase">返回购买页</RouterLink>
        </div>
      </div>
    </section>
  </main>
</template>
