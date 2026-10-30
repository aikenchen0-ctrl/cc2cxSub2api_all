<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type StationBillingDashboard, type StationBillingSettings, type StationPaymentPlan, type StationPaymentProvider } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import AgentInput from '@/components/common/AgentInput.vue'

const tab = ref<'settings' | 'providers' | 'plans' | 'orders'>('settings')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const settings = ref<StationBillingSettings | null>(null)
const providers = ref<StationPaymentProvider[]>([])
const plans = ref<StationPaymentPlan[]>([])
const orders = ref<Array<Record<string, unknown>>>([])
const dashboard = ref<StationBillingDashboard | null>(null)
const providerForm = ref(emptyProvider())
const planForm = ref(emptyPlan())
const enabledTypes = ref('alipay,wxpay')

const providerKeys = ['easypay', 'alipay', 'wxpay', 'stripe', 'airwallex']
const configFields: Record<string, string[]> = {
  easypay: ['pid', 'pkey', 'apiBase', 'cidAlipay', 'cidWxpay'],
  alipay: ['appId', 'privateKey', 'publicKey'],
  wxpay: ['appId', 'mchId', 'privateKey', 'apiV3Key', 'certSerial', 'publicKey', 'publicKeyId'],
  stripe: ['secretKey', 'publishableKey', 'webhookSecret', 'currency'],
  airwallex: ['clientId', 'apiKey', 'webhookSecret', 'apiBase', 'countryCode', 'currency', 'accountId'],
}

function emptyProvider(): StationPaymentProvider {
  return { provider_key: 'easypay', name: '', config: {}, supported_types: ['alipay', 'wxpay'], enabled: true, payment_mode: 'popup', sort_order: 0, limits: '', refund_enabled: false, allow_user_refund: false }
}
function emptyPlan(): StationPaymentPlan {
  return { name: '', description: '', price: 0, currency: 'CNY', validity_days: 30, validity_unit: 'day', features: [], product_name: '', for_sale: true, sort_order: 0 }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [current, providerPage, planPage, orderPage, stats] = await Promise.all([
      agentAPI.billing.settings(),
      agentAPI.billing.providers(),
      agentAPI.billing.plans(),
      agentAPI.billing.orders(),
      agentAPI.billing.dashboard(),
    ])
    settings.value = current
    enabledTypes.value = (current.enabled_payment_types || []).join(',')
    providers.value = providerPage.items || []
    plans.value = planPage.items || []
    orders.value = (orderPage.items || []) as unknown as Array<Record<string, unknown>>
    dashboard.value = stats
  } catch (cause) {
    error.value = errorMessage(cause, '收费配置加载失败。')
  } finally {
    loading.value = false
  }
}

async function saveSettings(): Promise<void> {
  if (!settings.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    settings.value.enabled_payment_types = enabledTypes.value.split(',').map(item => item.trim()).filter(Boolean)
    settings.value = await agentAPI.billing.saveSettings(settings.value)
    notice.value = settings.value.enabled ? '本站收费已开通。用户按这里的渠道和套餐付款。' : '本站收费已关闭。'
  } catch (cause) {
    error.value = errorMessage(cause, '保存收费设置失败。')
  } finally {
    saving.value = false
  }
}

async function saveProvider(): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    if (providerForm.value.id) await agentAPI.billing.updateProvider(providerForm.value.id, providerForm.value)
    else await agentAPI.billing.createProvider(providerForm.value)
    providerForm.value = emptyProvider()
    notice.value = '支付渠道已保存。密钥只保存在本站服务端。'
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '保存支付渠道失败。')
  } finally {
    saving.value = false
  }
}

async function removeProvider(id: number): Promise<void> {
  saving.value = true
  try { await agentAPI.billing.deleteProvider(id); await load() }
  catch (cause) { error.value = errorMessage(cause, '删除支付渠道失败。') }
  finally { saving.value = false }
}

async function savePlan(): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    const payload = { ...planForm.value, features: String(planForm.value.features || '').split(',').map(item => item.trim()).filter(Boolean) }
    if (planForm.value.id) await agentAPI.billing.updatePlan(planForm.value.id, payload)
    else await agentAPI.billing.createPlan(payload)
    planForm.value = emptyPlan()
    notice.value = '套餐已保存，只对本站用户可见。'
    await load()
  } catch (cause) {
    error.value = errorMessage(cause, '保存套餐失败。')
  } finally {
    saving.value = false
  }
}

async function removePlan(id: number): Promise<void> {
  saving.value = true
  try { await agentAPI.billing.deletePlan(id); await load() }
  catch (cause) { error.value = errorMessage(cause, '删除套餐失败。') }
  finally { saving.value = false }
}

function editProvider(item: StationPaymentProvider): void {
  providerForm.value = { ...item, config: { ...(item.config || {}) }, supported_types: [...item.supported_types] }
}
function editPlan(item: StationPaymentPlan): void {
  planForm.value = { ...item, features: [...item.features] }
}

onMounted(() => void load())
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6 pb-12">
    <header>
      <p class="text-sm font-medium text-primary-600">本站收费</p>
      <h1 class="mt-1 text-2xl font-bold">收费配置</h1>
      <p class="mt-2 max-w-3xl text-sm text-gray-500">站长在这里配置本站的收费开关、支付渠道、套餐和订单。这些数据只属于当前站点，其他站长看不到。</p>
    </header>
    <nav class="flex flex-wrap gap-2" aria-label="收费配置分区">
      <button v-for="item in [['settings','收费设置'],['providers','支付渠道'],['plans','套餐'],['orders','订单']]" :key="item[0]" class="btn" :class="tab === item[0] ? 'btn-primary' : 'btn-secondary'" type="button" @click="tab = item[0] as typeof tab">{{ item[1] }}</button>
    </nav>
    <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ notice }}</p>
    <section v-if="loading" class="card p-8 text-center text-sm text-gray-500">正在读取本站收费配置…</section>

    <form v-else-if="tab === 'settings' && settings" class="card space-y-4 p-6" @submit.prevent="saveSettings">
      <label class="flex items-center justify-between gap-4 rounded-lg border p-4"><span><span class="block text-sm font-medium">开通本站收费</span><span class="mt-1 block text-xs text-gray-500">开通后，用户在本站按下面的渠道付款，额度记入本站账户。</span></span><input v-model="settings.enabled" type="checkbox" class="h-4 w-4"></label>
      <div class="grid gap-4 md:grid-cols-3">
        <label class="text-sm">最低金额<AgentInput v-model="settings.min_amount" class="mt-1" /></label>
        <label class="text-sm">最高金额<AgentInput v-model="settings.max_amount" class="mt-1" /></label>
        <label class="text-sm">每日限额，0 为不限<AgentInput v-model="settings.daily_limit" class="mt-1" /></label>
        <label class="text-sm">订单超时（分钟）<AgentInput v-model="settings.order_timeout_minutes" class="mt-1" /></label>
        <label class="text-sm">最多待支付订单<AgentInput v-model="settings.max_pending_orders" class="mt-1" /></label>
        <label class="text-sm">充值手续费率（%）<AgentInput v-model="settings.recharge_fee_rate" class="mt-1" /></label>
        <label class="text-sm">模型计费倍率<AgentInput v-model="settings.model_rate_multiplier" class="mt-1" /></label>
        <label class="text-sm">订阅美元兑人民币<AgentInput v-model="settings.subscription_usd_to_cny_rate" class="mt-1" /></label>
        <label class="text-sm">渠道分配<select v-model="settings.load_balance_strategy" class="input mt-1"><option value="round-robin">轮询</option><option value="weighted">按排序</option></select></label>
        <label class="text-sm">商品名前缀<AgentInput v-model="settings.product_name_prefix" class="mt-1" /></label>
        <label class="text-sm">商品名后缀<AgentInput v-model="settings.product_name_suffix" class="mt-1" /></label>
        <label class="text-sm">开放的支付方式<AgentInput v-model="enabledTypes" class="mt-1" placeholder="alipay,wxpay" /></label>
      </div>
      <label class="flex items-center gap-2 text-sm"><input v-model="settings.balance_disabled" type="checkbox">关闭余额充值，只卖套餐</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="settings.alipay_force_qrcode" type="checkbox">支付宝强制二维码</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="settings.alipay_mobile_precreate_deep_link" type="checkbox">支付宝手机唤起</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="settings.payment_visible_method_alipay_enabled" type="checkbox">显示支付宝</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="settings.payment_visible_method_wxpay_enabled" type="checkbox">显示微信支付</label>
      <label class="block text-sm">支付说明<textarea v-model="settings.help_text" class="input mt-1 min-h-24" maxlength="2000"></textarea></label>
      <label class="block text-sm">说明图片地址<AgentInput v-model="settings.help_image_url" class="mt-1" /></label>
      <div class="flex justify-end"><button class="btn btn-primary" type="submit" :disabled="saving">保存收费设置</button></div>
    </form>

    <section v-else-if="tab === 'providers'" class="space-y-4">
      <form class="card space-y-4 p-6" @submit.prevent="saveProvider">
        <h2 class="text-lg font-semibold">{{ providerForm.id ? '编辑渠道' : '新增渠道' }}</h2>
        <div class="grid gap-4 md:grid-cols-3">
          <label class="text-sm">渠道类型<select v-model="providerForm.provider_key" class="input mt-1"><option v-for="key in providerKeys" :key="key" :value="key">{{ key }}</option></select></label>
          <label class="text-sm">名称<AgentInput v-model="providerForm.name" class="mt-1" /></label>
          <label class="text-sm">模式<AgentInput v-model="providerForm.payment_mode" class="mt-1" placeholder="popup 或 qrcode" /></label>
        </div>
        <label class="text-sm">支持的支付方式<AgentInput :model-value="providerForm.supported_types.join(',')" class="mt-1" @update:model-value="providerForm.supported_types = String($event).split(',').map(item => item.trim()).filter(Boolean)" /></label>
        <div class="grid gap-4 md:grid-cols-2">
          <label v-for="field in configFields[providerForm.provider_key] || []" :key="field" class="text-sm">{{ field }}<AgentInput :model-value="providerForm.config?.[field] || ''" class="mt-1" :type="['pkey','privateKey','apiV3Key','secretKey','webhookSecret','apiKey','publicKey'].includes(field) ? 'password' : 'text'" :placeholder="providerForm.config_configured?.[field] ? '已配置，留空不修改' : ''" @update:model-value="providerForm.config = { ...(providerForm.config || {}), [field]: String($event) }" /></label>
        </div>
        <label class="block text-sm">单渠道限额 JSON<AgentInput v-model="providerForm.limits" class="mt-1 font-mono" placeholder='{"alipay":{"single_min":1,"single_max":5000,"currency":"CNY"}}' /></label>
        <label class="flex items-center gap-2 text-sm"><input v-model="providerForm.enabled" type="checkbox">启用</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="providerForm.refund_enabled" type="checkbox">允许退款</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="providerForm.allow_user_refund" type="checkbox">允许用户申请退款</label>
        <div class="flex justify-end gap-2"><button class="btn btn-secondary" type="button" @click="providerForm = emptyProvider()">清空</button><button class="btn btn-primary" type="submit" :disabled="saving">保存渠道</button></div>
      </form>
      <article v-for="item in providers" :key="item.id" class="card flex flex-wrap items-center justify-between gap-3 p-4">
        <div><p class="font-medium">{{ item.name }} · {{ item.provider_key }}</p><p class="text-xs text-gray-500">{{ item.supported_types.join(', ') }} · {{ item.enabled ? '启用' : '停用' }}</p><p v-if="item.webhook_path" class="mt-1 font-mono text-xs text-gray-400">{{ item.webhook_path }}</p></div>
        <div class="flex gap-2"><button class="btn btn-secondary" type="button" @click="editProvider(item)">编辑</button><button class="btn btn-secondary" type="button" @click="item.id && removeProvider(item.id)">删除</button></div>
      </article>
    </section>

    <section v-else-if="tab === 'plans'" class="space-y-4">
      <form class="card grid gap-4 p-6 md:grid-cols-3" @submit.prevent="savePlan">
        <label class="text-sm">名称<AgentInput v-model="planForm.name" class="mt-1" /></label>
        <label class="text-sm">价格<AgentInput v-model="planForm.price" class="mt-1" /></label>
        <label class="text-sm">币种<AgentInput v-model="planForm.currency" class="mt-1" /></label>
        <label class="text-sm">有效天数<AgentInput v-model="planForm.validity_days" class="mt-1" /></label>
        <label class="text-sm">单位<AgentInput v-model="planForm.validity_unit" class="mt-1" /></label>
        <label class="text-sm">商品名<AgentInput v-model="planForm.product_name" class="mt-1" /></label>
        <label class="text-sm md:col-span-3">说明<AgentInput v-model="planForm.description" class="mt-1" /></label>
        <label class="text-sm md:col-span-3">特性，逗号分隔<AgentInput :model-value="planForm.features.join(',')" class="mt-1" @update:model-value="planForm.features = String($event).split(',').map(item => item.trim()).filter(Boolean)" /></label>
        <label class="flex items-center gap-2 text-sm"><input v-model="planForm.for_sale" type="checkbox">在售</label>
        <div class="md:col-span-3 flex justify-end gap-2"><button class="btn btn-secondary" type="button" @click="planForm = emptyPlan()">清空</button><button class="btn btn-primary" type="submit" :disabled="saving">保存套餐</button></div>
      </form>
      <article v-for="item in plans" :key="item.id" class="card flex items-center justify-between p-4">
        <div><p class="font-medium">{{ item.name }} · {{ item.price }} {{ item.currency }}</p><p class="text-xs text-gray-500">{{ item.for_sale ? '在售' : '下架' }} · {{ item.validity_days }} {{ item.validity_unit }}</p></div>
        <div class="flex gap-2"><button class="btn btn-secondary" type="button" @click="editPlan(item)">编辑</button><button class="btn btn-secondary" type="button" @click="item.id && removePlan(item.id)">删除</button></div>
      </article>
    </section>

    <section v-else class="space-y-4">
      <div v-if="dashboard" class="grid gap-4 sm:grid-cols-4">
        <div class="card p-4"><p class="text-xs text-gray-500">已完成</p><p class="text-2xl font-bold">{{ dashboard.completed_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">待支付</p><p class="text-2xl font-bold">{{ dashboard.pending_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">退款</p><p class="text-2xl font-bold">{{ dashboard.refunded_count }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500">实收</p><p class="text-2xl font-bold">{{ dashboard.amount_available ? dashboard.completed_pay_amount : '待确认' }}</p></div>
      </div>
      <article v-for="item in orders" :key="String(item.id)" class="card flex flex-wrap items-center justify-between gap-3 p-4">
        <div><p class="font-mono text-sm">{{ item.out_trade_no }}</p><p class="text-sm">{{ item.pay_amount }} {{ item.currency }} · {{ item.status }} · {{ item.payment_type }}</p></div>
        <div class="flex gap-2">
          <button v-if="item.status === 'PENDING'" class="btn btn-secondary" type="button" @click="agentAPI.billing.cancelOrder(Number(item.id)).then(load)">取消</button>
          <button v-if="item.status === 'COMPLETED' || item.status === 'REFUND_REQUESTED'" class="btn btn-secondary" type="button" @click="agentAPI.billing.refundOrder(Number(item.id)).then(load)">标记退款</button>
        </div>
      </article>
      <p v-if="orders.length === 0" class="card p-8 text-center text-sm text-gray-500">本站还没有订单。</p>
    </section>
  </main>
</template>
