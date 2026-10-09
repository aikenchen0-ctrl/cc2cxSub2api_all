<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type StationBillingSettings } from '@/agent/api'
import { useI18n } from '@/agent/componentI18n'
import { errorMessage } from '@/agent/client'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import ImageUpload from '@/components/common/ImageUpload.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import PaymentProviderList from '@/components/payment/PaymentProviderList.vue'
import type { ProviderInstance } from '@/types/payment'

const { t } = useI18n()
const agentId = ref('')
const loading = ref(false)
const saving = ref(false)
const loadFailed = ref(false)
const error = ref('')
const notice = ref('')
const form = ref(emptyForm())
const providers = ref<ProviderInstance[]>([])
const providersLoading = ref(false)
const providerSaving = ref(false)
const showProviderDialog = ref(false)
const showDeleteProviderDialog = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)
const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

const paymentGuideHref = computed(() => 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md')
const paymentMethodsHref = computed(() => 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT_CN.md#支持的支付方式')

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('payment.methods.easypay') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
  { value: 'airwallex', label: t('payment.methods.airwallex') },
])
const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
  { value: 'airwallex', label: t('admin.settings.payment.providerAirwallex') },
])
const enabledProviderKeyOptions = computed(() => {
  const enabled = form.value.payment_enabled_types
  return providerKeyOptions.value.filter(opt => enabled.includes(opt.value))
})
const hasAnyPaymentTypeEnabled = computed(() => form.value.payment_enabled_types.length > 0)
const loadBalanceOptions = computed(() => [
  { value: 'round-robin', label: t('admin.settings.payment.strategyRoundRobin') },
  { value: 'least-amount', label: t('admin.settings.payment.strategyLeastAmount') },
])
const cancelRateLimitUnitOptions = computed(() => [
  { value: 'minute', label: t('admin.settings.payment.cancelRateLimitUnitMinute') },
  { value: 'hour', label: t('admin.settings.payment.cancelRateLimitUnitHour') },
  { value: 'day', label: t('admin.settings.payment.cancelRateLimitUnitDay') },
])
const cancelRateLimitModeOptions = computed(() => [
  { value: 'rolling', label: t('admin.settings.payment.cancelRateLimitWindowModeRolling') },
  { value: 'fixed', label: t('admin.settings.payment.cancelRateLimitWindowModeFixed') },
])

type ProviderEnablementCandidate = Pick<ProviderInstance, 'id' | 'provider_key' | 'supported_types' | 'enabled' | 'name'>

function emptyForm() {
  return {
    payment_enabled: false,
    payment_balance_disabled: false,
    payment_product_name_prefix: '',
    payment_product_name_suffix: '',
    payment_min_amount: 1,
    payment_max_amount: 10000,
    payment_daily_limit: 0,
    payment_balance_recharge_multiplier: 1,
    payment_subscription_usd_to_cny_rate: 0,
    payment_recharge_fee_rate: 0,
    payment_order_timeout_minutes: 30,
    payment_max_pending_orders: 3,
    payment_load_balance_strategy: 'round-robin',
    payment_cancel_rate_limit_enabled: false,
    payment_cancel_rate_limit_window_mode: 'rolling',
    payment_cancel_rate_limit_window: 1,
    payment_cancel_rate_limit_unit: 'day',
    payment_cancel_rate_limit_max: 10,
    payment_alipay_force_qrcode: false,
    payment_alipay_mobile_precreate_deep_link: false,
    payment_enabled_types: [] as string[],
    payment_help_image_url: '',
    payment_help_text: '',
    payment_visible_method_alipay_source: '',
    payment_visible_method_alipay_enabled: true,
    payment_visible_method_wxpay_source: '',
    payment_visible_method_wxpay_enabled: true,
  }
}

function normalizeVisibleMethod(type: string): 'alipay' | 'wxpay' | '' {
  if (type === 'alipay' || type === 'alipay_direct') return 'alipay'
  if (type === 'wxpay' || type === 'wxpay_direct') return 'wxpay'
  return ''
}

function applySettings(settings: StationBillingSettings): void {
  form.value = {
    ...emptyForm(),
    payment_enabled: settings.enabled,
    payment_balance_disabled: settings.balance_disabled,
    payment_product_name_prefix: settings.product_name_prefix,
    payment_product_name_suffix: settings.product_name_suffix,
    payment_min_amount: settings.min_amount,
    payment_max_amount: settings.max_amount,
    payment_daily_limit: settings.daily_limit,
    payment_balance_recharge_multiplier: settings.balance_recharge_multiplier,
    payment_subscription_usd_to_cny_rate: settings.subscription_usd_to_cny_rate,
    payment_recharge_fee_rate: settings.recharge_fee_rate,
    payment_order_timeout_minutes: settings.order_timeout_minutes,
    payment_max_pending_orders: settings.max_pending_orders,
    payment_load_balance_strategy: settings.load_balance_strategy || 'round-robin',
    payment_cancel_rate_limit_enabled: settings.cancel_rate_limit_enabled,
    payment_cancel_rate_limit_window_mode: settings.cancel_rate_limit_window_mode || 'rolling',
    payment_cancel_rate_limit_window: settings.cancel_rate_limit_window,
    payment_cancel_rate_limit_unit: settings.cancel_rate_limit_unit || 'day',
    payment_cancel_rate_limit_max: settings.cancel_rate_limit_max,
    payment_alipay_force_qrcode: settings.alipay_force_qrcode,
    payment_alipay_mobile_precreate_deep_link: settings.alipay_mobile_precreate_deep_link,
    payment_enabled_types: settings.enabled_payment_types || [],
    payment_help_image_url: settings.help_image_url,
    payment_help_text: settings.help_text,
    payment_visible_method_alipay_source: settings.payment_visible_method_alipay_source || "",
    payment_visible_method_alipay_enabled: settings.payment_visible_method_alipay_enabled,
    payment_visible_method_wxpay_source: settings.payment_visible_method_wxpay_source || "",
    payment_visible_method_wxpay_enabled: settings.payment_visible_method_wxpay_enabled,
  }
}

function toPayload(): StationBillingSettings {
  const current = form.value
  return {
    enabled: current.payment_enabled,
    min_amount: Number(current.payment_min_amount) || 0,
    max_amount: Number(current.payment_max_amount) || 0,
    daily_limit: Number(current.payment_daily_limit) || 0,
    order_timeout_minutes: Number(current.payment_order_timeout_minutes),
    max_pending_orders: Number(current.payment_max_pending_orders),
    enabled_payment_types: current.payment_enabled_types,
    balance_disabled: current.payment_balance_disabled,
    balance_recharge_multiplier: Number(current.payment_balance_recharge_multiplier),
    subscription_usd_to_cny_rate: Number(current.payment_subscription_usd_to_cny_rate) || 0,
    recharge_fee_rate: Number(current.payment_recharge_fee_rate) || 0,
    load_balance_strategy: current.payment_load_balance_strategy,
    product_name_prefix: current.payment_product_name_prefix,
    product_name_suffix: current.payment_product_name_suffix,
    help_image_url: current.payment_help_image_url,
    help_text: current.payment_help_text,
    cancel_rate_limit_enabled: current.payment_cancel_rate_limit_enabled,
    cancel_rate_limit_max: Number(current.payment_cancel_rate_limit_max),
    cancel_rate_limit_window: Number(current.payment_cancel_rate_limit_window),
    cancel_rate_limit_unit: current.payment_cancel_rate_limit_unit,
    cancel_rate_limit_window_mode: current.payment_cancel_rate_limit_window_mode,
    alipay_force_qrcode: current.payment_alipay_force_qrcode,
    alipay_mobile_precreate_deep_link: current.payment_alipay_mobile_precreate_deep_link,
    payment_visible_method_alipay_source: current.payment_visible_method_alipay_source,
    payment_visible_method_wxpay_source: current.payment_visible_method_wxpay_source,
    payment_visible_method_alipay_enabled: current.payment_visible_method_alipay_enabled,
    payment_visible_method_wxpay_enabled: current.payment_visible_method_wxpay_enabled,
  }
}

function isPaymentTypeEnabled(type: string): boolean {
  return form.value.payment_enabled_types.includes(type)
}

function getProviderVisibleMethods(provider: ProviderEnablementCandidate): Array<'alipay' | 'wxpay'> {
  if (!provider.enabled) return []
  const supportedTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const methods = new Set<'alipay' | 'wxpay'>()
  const addMethod = (type: string) => {
    const method = normalizeVisibleMethod(type)
    if (method) methods.add(method)
  }
  if (provider.provider_key === 'alipay') {
    if (supportedTypes.length === 0) methods.add('alipay')
    else supportedTypes.forEach(type => { if (normalizeVisibleMethod(type) === 'alipay') methods.add('alipay') })
  } else if (provider.provider_key === 'wxpay') {
    if (supportedTypes.length === 0) methods.add('wxpay')
    else supportedTypes.forEach(type => { if (normalizeVisibleMethod(type) === 'wxpay') methods.add('wxpay') })
  } else if (provider.provider_key === 'easypay') {
    supportedTypes.forEach(addMethod)
  }
  return Array.from(methods)
}

function findProviderEnablementConflict(candidate: ProviderEnablementCandidate): { method: 'alipay' | 'wxpay'; conflicting: ProviderInstance } | null {
  const claimedMethods = getProviderVisibleMethods(candidate)
  if (claimedMethods.length === 0) return null
  for (const other of providers.value) {
    if (other.id === candidate.id || !other.enabled || other.provider_key === candidate.provider_key) continue
    const matchedMethod = claimedMethods.find(method => getProviderVisibleMethods(other).includes(method))
    if (matchedMethod) return { method: matchedMethod, conflicting: other }
  }
  return null
}

function showProviderEnablementConflict(conflict: { method: 'alipay' | 'wxpay'; conflicting: ProviderInstance }): void {
  window.alert(t('admin.settings.payment.enableConflict', {
    method: t(`payment.methods.${conflict.method}`),
    provider: conflict.conflicting.name,
  }))
}

async function loadProviders(): Promise<void> {
  providersLoading.value = true
  try {
    const page = await agentAPI.billing.providers()
    providers.value = (page.items || []).map(item => ({
      id: item.id || 0,
      provider_key: item.provider_key,
      name: item.name,
      config: item.config || {},
      supported_types: Array.isArray(item.supported_types) ? item.supported_types : [],
      enabled: item.enabled,
      payment_mode: item.payment_mode,
      refund_enabled: item.refund_enabled,
      allow_user_refund: item.allow_user_refund,
      limits: item.limits || '',
      sort_order: item.sort_order,
    }))
  } catch (cause) {
    loadFailed.value = true
    error.value = errorMessage(cause, '支付渠道加载失败')
  } finally {
    providersLoading.value = false
  }
}

async function saveSettings(): Promise<void> {
  if (loading.value || loadFailed.value || saving.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    applySettings(await agentAPI.billing.saveSettings(toPayload()))
    notice.value = '支付配置已保存'
  } catch (cause) {
    error.value = errorMessage(cause, '保存支付配置失败')
  } finally {
    saving.value = false
  }
}

function togglePaymentType(type: string): void {
  if (form.value.payment_enabled_types.includes(type)) {
    form.value.payment_enabled_types = form.value.payment_enabled_types.filter(item => item !== type)
    void disableProvidersByType(type)
  } else {
    form.value.payment_enabled_types = [...form.value.payment_enabled_types, type]
  }
}

async function disableProvidersByType(type: string): Promise<void> {
  const matching = providers.value.filter(item => item.provider_key === type && item.enabled)
  for (const provider of matching) {
    try {
      await agentAPI.billing.updateProvider(provider.id, { enabled: false })
      provider.enabled = false
    } catch (cause) {
      error.value = errorMessage(cause, '停用服务商失败')
    }
  }
}

function openCreateProvider(): void {
  error.value = ''
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance): void {
  error.value = ''
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

async function handleSaveProvider(payload: Partial<ProviderInstance>): Promise<void> {
  providerSaving.value = true
  try {
    const candidate: ProviderEnablementCandidate = {
      id: editingProvider.value?.id ?? 0,
      provider_key: payload.provider_key ?? editingProvider.value?.provider_key ?? '',
      supported_types: payload.supported_types ?? editingProvider.value?.supported_types ?? [],
      enabled: payload.enabled ?? editingProvider.value?.enabled ?? false,
      name: payload.name ?? editingProvider.value?.name ?? '',
    }
    const conflict = findProviderEnablementConflict(candidate)
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }
    if (editingProvider.value) await agentAPI.billing.updateProvider(editingProvider.value.id, payload)
    else await agentAPI.billing.createProvider(payload as ProviderInstance)
    showProviderDialog.value = false
    await loadProviders()
    await saveSettings()
  } catch (cause) {
    error.value = errorMessage(cause, '保存服务商失败')
  } finally {
    providerSaving.value = false
  }
}

async function handleToggleField(provider: ProviderInstance, field: 'enabled' | 'refund_enabled' | 'allow_user_refund'): Promise<void> {
  const newValue = !provider[field]
  if (field === 'enabled' && newValue) {
    const conflict = findProviderEnablementConflict({ ...provider, enabled: true })
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }
  }
  const next: Partial<ProviderInstance> = { [field]: newValue }
  if (field === 'refund_enabled' && !newValue) next.allow_user_refund = false
  try {
    await agentAPI.billing.updateProvider(provider.id, next)
    await loadProviders()
  } catch (cause) {
    error.value = errorMessage(cause, '更新服务商失败')
  }
}

async function handleToggleType(provider: ProviderInstance, type: string): Promise<void> {
  const currentTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const updated = currentTypes.includes(type) ? currentTypes.filter(item => item !== type) : [...currentTypes, type]
  const conflict = findProviderEnablementConflict({ ...provider, supported_types: updated })
  if (conflict) {
    showProviderEnablementConflict(conflict)
    return
  }
  try {
    await agentAPI.billing.updateProvider(provider.id, { supported_types: updated })
    await loadProviders()
  } catch (cause) {
    error.value = errorMessage(cause, '更新支付方式失败')
  }
}

function confirmDeleteProvider(provider: ProviderInstance): void {
  deletingProviderId.value = provider.id
  showDeleteProviderDialog.value = true
}

async function handleReorderProviders(updates: { id: number; sort_order: number }[]): Promise<void> {
  try {
    await Promise.all(updates.map(item => {
      const provider = providers.value.find(entry => entry.id === item.id)
      return provider ? agentAPI.billing.updateProvider(item.id, { sort_order: item.sort_order }) : Promise.resolve()
    }))
    await loadProviders()
  } catch (cause) {
    error.value = errorMessage(cause, '排序保存失败')
    await loadProviders()
  }
}

async function handleDeleteProvider(): Promise<void> {
  if (!deletingProviderId.value) return
  try {
    await agentAPI.billing.deleteProvider(deletingProviderId.value)
    showDeleteProviderDialog.value = false
    await loadProviders()
  } catch (cause) {
    error.value = errorMessage(cause, '删除服务商失败')
  }
}

async function load(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  error.value = ''
  try {
    const context = await agentAPI.getContext()
    agentId.value = context.agent.agent_id
    applySettings(await agentAPI.billing.settings())
    await loadProviders()
  } catch (cause) {
    loadFailed.value = true
    error.value = errorMessage(cause, '支付配置加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ error }}</p>
  <p v-if="notice" role="status" class="rounded-lg bg-emerald-50 p-4 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ notice }}</p>
  <button v-if="loadFailed" type="button" class="btn btn-secondary" @click="load">重新加载</button>
  <form class="space-y-6" novalidate @submit.prevent="saveSettings">
    <fieldset :disabled="loading || loadFailed || saving" class="contents">
        <div  class="space-y-6">
          <!-- Payment System Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.payment.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.payment.description") }}
                <a
                  :href="paymentGuideHref"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="ml-2 inline-flex items-center text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  <svg
                    class="mr-0.5 h-3.5 w-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                    />
                  </svg>
                  {{ t("admin.settings.payment.configGuide") }}
                </a>
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Enable toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">{{
                    t("admin.settings.payment.enabled")
                  }}</label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.payment.enabledHint") }}
                  </p>
                </div>
                <Toggle v-model="form.payment_enabled" />
              </div>
              <p v-if="!form.payment_enabled" role="status" class="rounded-lg bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
                当前未开放收款。可先填写以下设置并添加服务商；保存配置不会自动启用支付。
              </p>
              <div class="space-y-4" data-testid="payment-settings-fields">
                <!-- Row 1: Product name -->
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.productNamePrefix")
                    }}</label
                    ><input
                      v-model="form.payment_product_name_prefix"
                      type="text"
                      class="input"
                      placeholder="Sub2API"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.productNameSuffix")
                    }}</label
                    ><input
                      v-model="form.payment_product_name_suffix"
                      type="text"
                      class="input"
                      placeholder="CNY"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.preview")
                    }}</label>
                    <div
                      class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
                    >
                      {{
                        (form.payment_product_name_prefix || "Sub2API") +
                        " 100 " +
                        (form.payment_product_name_suffix || "CNY")
                      }}
                    </div>
                  </div>
                </div>
                <!-- Row 2: Balance toggle + amounts -->
                <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.minAmount")
                    }}</label
                    ><input
                      :value="form.payment_min_amount || ''"
                      @input="
                        form.payment_min_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.maxAmount")
                    }}</label
                    ><input
                      :value="form.payment_max_amount || ''"
                      @input="
                        form.payment_max_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.dailyLimit")
                    }}</label
                    ><input
                      :value="form.payment_daily_limit || ''"
                      @input="
                        form.payment_daily_limit =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.balanceRechargeMultiplier")
                    }}</label>
                    <input
                      :value="form.payment_balance_recharge_multiplier || ''"
                      @input="
                        form.payment_balance_recharge_multiplier =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 1
                      "
                      type="number"
                      step="0.01"
                      min="0.01"
                      class="input"
                    />
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{
                        t(
                          "admin.settings.payment.balanceRechargeMultiplierHint",
                        )
                      }}
                    </p>
                    <p
                      class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400"
                    >
                      {{
                        t("admin.settings.payment.balanceRechargePreview", {
                          usd: (
                            Number(form.payment_balance_recharge_multiplier) ||
                            1
                          ).toFixed(2),
                        })
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.subscriptionUsdToCnyRate")
                    }}</label>
                    <input
                      :value="form.payment_subscription_usd_to_cny_rate || ''"
                      @input="
                        form.payment_subscription_usd_to_cny_rate =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      :placeholder="
                        t(
                          'admin.settings.payment.subscriptionUsdToCnyRateDisabled',
                        )
                      "
                    />
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{
                        t("admin.settings.payment.subscriptionUsdToCnyRateHint")
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.rechargeFeeRate")
                    }}</label>
                    <div class="relative">
                      <input
                        :value="form.payment_recharge_fee_rate ?? ''"
                        @input="
                          form.payment_recharge_fee_rate = Math.min(
                            100,
                            Math.max(
                              0,
                              Math.round(
                                parseFloat(
                                  ($event.target as HTMLInputElement).value ||
                                    '0',
                                ) * 100,
                              ) / 100,
                            ),
                          )
                        "
                        type="number"
                        step="0.01"
                        min="0"
                        max="100"
                        class="input pr-8"
                      />
                      <span
                        class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400"
                        >%</span
                      >
                    </div>
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{ t("admin.settings.payment.rechargeFeeRateHint") }}
                    </p>
                    <p
                      v-if="(Number(form.payment_recharge_fee_rate) || 0) > 0"
                      class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400"
                    >
                      {{
                        t("admin.settings.payment.rechargeFeePreview", {
                          fee: (
                            Number(form.payment_recharge_fee_rate) || 0
                          ).toFixed(2),
                        })
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="input-label"
                      >{{ t("admin.settings.payment.orderTimeout") }}
                      <span class="text-red-500">*</span></label
                    ><input
                      v-model.number="form.payment_order_timeout_minutes"
                      type="number"
                      min="1"
                      class="input"
                      required
                    />
                    <p class="mt-0.5 text-xs text-gray-400">
                      {{ t("admin.settings.payment.orderTimeoutHint") }}
                    </p>
                  </div>
                </div>
                <!-- Row 3: Pending orders + load balance + cancel rate limit (all in one row) -->
                <div class="flex flex-wrap items-end gap-4">
                  <div class="w-28">
                    <label class="input-label">{{
                      t("admin.settings.payment.maxPendingOrders")
                    }}</label
                    ><input
                      v-model.number="form.payment_max_pending_orders"
                      type="number"
                      min="1"
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.loadBalanceStrategy")
                    }}</label>
                    <Select
                      v-model="form.payment_load_balance_strategy"
                      :options="loadBalanceOptions"
                      class="w-40"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.cancelRateLimit")
                    }}</label>
                    <div class="flex flex-wrap items-center gap-2">
                      <button
                        type="button"
                        :class="[
                          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                          form.payment_cancel_rate_limit_enabled
                            ? 'bg-primary-500'
                            : 'bg-gray-300 dark:bg-dark-600',
                        ]"
                        @click="
                          form.payment_cancel_rate_limit_enabled =
                            !form.payment_cancel_rate_limit_enabled
                        "
                      >
                        <span
                          :class="[
                            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                            form.payment_cancel_rate_limit_enabled
                              ? 'translate-x-5'
                              : 'translate-x-0',
                          ]"
                        />
                      </button>
                      <Select
                        v-model="form.payment_cancel_rate_limit_window_mode"
                        :options="cancelRateLimitModeOptions"
                        class="w-24"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitEvery")
                        }}</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_window"
                        type="number"
                        min="1"
                        required
                        class="input w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <Select
                        v-model="form.payment_cancel_rate_limit_unit"
                        :options="cancelRateLimitUnitOptions"
                        class="w-28"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitAllowMax")
                        }}</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_max"
                        type="number"
                        min="1"
                        required
                        class="input w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-gray-700 dark:text-gray-300'
                            : 'text-gray-400 dark:text-gray-600',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitTimes")
                        }}</span
                      >
                    </div>
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.alipayForceQRCode")
                    }}</label>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        :class="[
                          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                          form.payment_alipay_force_qrcode
                            ? 'bg-primary-500'
                            : 'bg-gray-300 dark:bg-dark-600',
                        ]"
                        @click="
                          form.payment_alipay_force_qrcode =
                            !form.payment_alipay_force_qrcode
                        "
                      >
                        <span
                          :class="[
                            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                            form.payment_alipay_force_qrcode
                              ? 'translate-x-5'
                              : 'translate-x-0',
                          ]"
                        />
                      </button>
                      <span class="text-sm text-gray-500 dark:text-gray-400">{{
                        t("admin.settings.payment.alipayForceQRCodeHint")
                      }}</span>
                    </div>
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.alipayMobilePrecreateDeepLink")
                    }}</label>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        :class="[
                          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                          form.payment_alipay_mobile_precreate_deep_link
                            ? 'bg-primary-500'
                            : 'bg-gray-300 dark:bg-dark-600',
                        ]"
                        @click="
                          form.payment_alipay_mobile_precreate_deep_link =
                            !form.payment_alipay_mobile_precreate_deep_link
                        "
                      >
                        <span
                          :class="[
                            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                            form.payment_alipay_mobile_precreate_deep_link
                              ? 'translate-x-5'
                              : 'translate-x-0',
                          ]"
                        />
                      </button>
                      <span class="text-sm text-gray-500 dark:text-gray-400">{{
                        t("admin.settings.payment.alipayMobilePrecreateDeepLinkHint")
                      }}</span>
                    </div>
                  </div>
                </div>
                <!-- Row 4: Enabled payment types (provider badges like sub2apipay) -->
                <div>
                  <label class="input-label">{{
                    t("admin.settings.payment.enabledPaymentTypes")
                  }}</label>
                  <div class="mt-1.5 flex flex-wrap gap-2">
                    <button
                      v-for="pt in allPaymentTypes"
                      :key="pt.value"
                      type="button"
                      @click="togglePaymentType(pt.value)"
                      :class="[
                        'rounded-lg border px-3 py-1.5 text-sm font-medium transition-all',
                        isPaymentTypeEnabled(pt.value)
                          ? 'border-primary-500 bg-primary-500 text-white shadow-sm'
                          : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500',
                      ]"
                    >
                      {{ pt.label }}
                    </button>
                  </div>
                  <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                    {{ t("admin.settings.payment.enabledPaymentTypesHint") }}
                    <a
                      :href="paymentMethodsHref"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="ml-1 text-primary-500 hover:text-primary-600 dark:text-primary-400 dark:hover:text-primary-300"
                    >
                      {{ t("admin.settings.payment.findProvider") }}
                      <svg
                        class="mb-0.5 ml-0.5 inline h-3 w-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                    </a>
                  </p>
                </div>
                <!-- Row 5: Help image + text -->
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.helpImage")
                    }}</label>
                    <ImageUpload
                      v-model="form.payment_help_image_url"
                      :upload-label="t('admin.settings.site.uploadImage')"
                      :remove-label="t('admin.settings.site.remove')"
                      :placeholder="
                        t('admin.settings.payment.helpImagePlaceholder')
                      "
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.helpText")
                    }}</label>
                    <textarea
                      v-model="form.payment_help_text"
                      rows="3"
                      class="input"
                      :placeholder="
                        t('admin.settings.payment.helpTextPlaceholder')
                      "
                    ></textarea>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Provider Management -->
          <PaymentProviderList
            :providers="providers"
            :loading="providersLoading"
            :can-create="hasAnyPaymentTypeEnabled"
            :enabled-payment-types="form.payment_enabled_types"
            :all-payment-types="allPaymentTypes"
            :redirect-label="t('admin.settings.payment.easypayRedirect')"
            @refresh="loadProviders"
            @create="openCreateProvider"
            @edit="openEditProvider"
            @delete="confirmDeleteProvider"
            @toggle-field="handleToggleField"
            @toggle-type="handleToggleType"
            @reorder="handleReorderProviders"
          />
        </div>
        <div class="flex justify-end">
          <button
            type="submit"
            :disabled="loading || saving || loadFailed"
            class="btn btn-primary"
          >
            <svg
              v-if="saving"
              class="h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              saving
                ? t("admin.settings.saving")
                : t("admin.settings.saveSettings")
            }}
          </button>
        </div>
    </fieldset>
  </form>
      <PaymentProviderDialog
        :agent-id="agentId"
        :server-error="error"
        ref="providerDialogRef"
        :show="showProviderDialog"
        :saving="providerSaving"
        :editing="editingProvider"
        :all-key-options="providerKeyOptions"
        :enabled-key-options="enabledProviderKeyOptions"
        :all-payment-types="allPaymentTypes"
        :redirect-label="t('admin.settings.payment.easypayRedirect')"
        @close="showProviderDialog = false"
        @save="handleSaveProvider"
      />
      <ConfirmDialog
        :show="showDeleteProviderDialog"
        :title="t('admin.settings.payment.deleteProvider')"
        :message="t('admin.settings.payment.deleteProviderConfirm')"
        :confirm-text="t('common.delete')"
        danger
        @confirm="handleDeleteProvider"
        @cancel="showDeleteProviderDialog = false"
      />

</template>
