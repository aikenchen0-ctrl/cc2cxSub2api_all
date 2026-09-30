<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { agentAPI, type AgentAdminCheckoutPlan, type AgentPlanPolicyInput } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentGroupBadge from '@/components/common/AgentGroupBadge.vue'
import AgentTextArea from '@/components/common/AgentTextArea.vue'
import AgentToggle from '@/components/common/AgentToggle.vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'

const plans = ref<AgentAdminCheckoutPlan[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const success = ref('')
const editing = ref<AgentAdminCheckoutPlan | null>(null)
const featuresText = ref('')
const form = reactive<AgentPlanPolicyInput>({
  enabled: true,
  sort_order: 0,
  display_name: '',
  description: '',
  features: [],
})

const columns = [
  { key: 'id', label: 'ID' },
  { key: 'name', label: '套餐名称' },
  { key: 'group_name', label: '主站分组' },
  { key: 'price', label: '价格' },
  { key: 'validity_days', label: '有效期' },
  { key: 'enabled', label: '本站展示' },
  { key: 'sort_order', label: '排序' },
  { key: 'actions', label: '操作' },
]

const enabledCount = computed(() => plans.value.filter(plan => plan.enabled).length)
const customizedCount = computed(() => plans.value.filter(plan => plan.customized).length)

function money(value: number, currency = 'USD') {
  try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: currency || 'USD' }).format(value) }
  catch { return `${currency || 'USD'} ${Number(value || 0).toFixed(2)}` }
}

function validity(plan: AgentAdminCheckoutPlan) {
  const units: Record<string, string> = { day: '天', days: '天', month: '个月', months: '个月', year: '年', years: '年' }
  return `${plan.validity_days} ${units[plan.validity_unit] || plan.validity_unit || '天'}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.adminPaymentPlans.list()
    plans.value = result.items || []
  } catch (cause) {
    error.value = errorMessage(cause, '套餐列表加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function openEdit(plan: AgentAdminCheckoutPlan) {
  editing.value = plan
  form.enabled = plan.enabled
  form.sort_order = plan.sort_order
  form.display_name = plan.customized && plan.name !== plan.source_name ? plan.name : ''
  form.description = plan.customized && plan.description !== plan.source_description ? plan.description : ''
  const features = plan.customized && JSON.stringify(plan.features || []) !== JSON.stringify(plan.source_features || [])
    ? plan.features
    : []
  featuresText.value = (features || []).join('\n')
}

function closeEdit() {
  if (!saving.value) editing.value = null
}

async function save() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    const payload: AgentPlanPolicyInput = {
      enabled: form.enabled,
      sort_order: Number(form.sort_order) || 0,
      display_name: form.display_name.trim(),
      description: form.description.trim(),
      features: featuresText.value.split('\n').map(item => item.trim()).filter(Boolean),
    }
    const updated = await agentAPI.adminPaymentPlans.update(editing.value.id, payload)
    const index = plans.value.findIndex(plan => plan.id === updated.id)
    if (index >= 0) plans.value.splice(index, 1, updated)
    plans.value.sort((left, right) => left.sort_order - right.sort_order || left.id - right.id)
    success.value = `套餐“${updated.name}”的本站展示策略已保存。`
    editing.value = null
  } catch (cause) {
    error.value = errorMessage(cause, '套餐策略保存失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}

async function toggle(plan: AgentAdminCheckoutPlan) {
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    const updated = await agentAPI.adminPaymentPlans.update(plan.id, {
      enabled: !plan.enabled,
      sort_order: plan.sort_order,
      display_name: plan.customized && plan.name !== plan.source_name ? plan.name : '',
      description: plan.customized && plan.description !== plan.source_description ? plan.description : '',
      features: plan.customized && JSON.stringify(plan.features || []) !== JSON.stringify(plan.source_features || []) ? plan.features : [],
    })
    const index = plans.value.findIndex(item => item.id === updated.id)
    if (index >= 0) plans.value.splice(index, 1, updated)
    success.value = updated.enabled ? '套餐已在本站恢复展示。' : '套餐已从本站购买页面隐藏。'
  } catch (cause) {
    error.value = errorMessage(cause, '套餐展示状态更新失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}

onMounted(() => { void load() })
</script>

<template>
  <section class="space-y-5" aria-label="本站套餐管理">
    <div class="grid gap-4 sm:grid-cols-3">
      <div class="card p-5"><p class="text-xs text-gray-500 dark:text-gray-400">主站可售套餐</p><p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ plans.length }}</p></div>
      <div class="card p-5"><p class="text-xs text-gray-500 dark:text-gray-400">本站展示</p><p class="mt-2 text-2xl font-semibold text-emerald-600">{{ enabledCount }}</p></div>
      <div class="card p-5"><p class="text-xs text-gray-500 dark:text-gray-400">本站已自定义</p><p class="mt-2 text-2xl font-semibold text-primary-600">{{ customizedCount }}</p></div>
    </div>

    <div class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-200">
      套餐价格、币种、有效期、订阅分组和真实销售状态由 Sub2API 主站统一管理。这里仅控制当前代理站的展示、排序与文案，不会修改主站或其他代理站配置。
    </div>
    <p v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">{{ error }}</p>
    <p v-if="success" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700" role="status">{{ success }}</p>

    <div class="card overflow-hidden">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
        <div><h2 class="font-semibold text-gray-900 dark:text-white">订阅套餐</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">数据实时读取主站，本站覆盖项按代理站隔离保存。</p></div>
        <button class="btn btn-secondary" type="button" :disabled="loading" aria-label="刷新套餐列表" @click="load"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />刷新</button>
      </div>
      <DataTable :columns="columns" :data="plans" :loading="loading" row-key="id">
        <template #empty><div class="py-10 text-center text-gray-500">主站当前没有可供本站展示的套餐</div></template>
        <template #cell-id="{ value }"><span class="font-mono text-xs">#{{ value }}</span></template>
        <template #cell-name="{ row }"><div class="min-w-52"><p class="font-medium text-gray-900 dark:text-white">{{ row.name }}</p><p v-if="row.customized" class="mt-0.5 text-xs text-primary-600">本站自定义文案</p><p v-else class="mt-0.5 text-xs text-gray-400">继承主站文案</p></div></template>
        <template #cell-group_name="{ row }"><AgentGroupBadge :group-id="row.group_id" :name="row.group_name || `分组 #${row.group_id}`" :platform="row.group_platform" subscription-type="subscription" :rate-multiplier="row.rate_multiplier" :peak-rate-enabled="row.peak_rate_enabled" :peak-start="row.peak_start" :peak-end="row.peak_end" :peak-rate-multiplier="row.peak_rate_multiplier" /></template>
        <template #cell-price="{ row }"><div><p class="font-medium">{{ money(row.price, row.currency) }}</p><p v-if="row.original_price" class="text-xs text-gray-400 line-through">{{ money(row.original_price, row.currency) }}</p></div></template>
        <template #cell-validity_days="{ row }"><span class="text-sm">{{ validity(row) }}</span></template>
        <template #cell-enabled="{ row }"><button type="button" class="relative inline-flex h-6 w-11 rounded-full transition-colors disabled:opacity-50" :class="row.enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'" :aria-label="row.enabled ? '隐藏套餐' : '展示套餐'" :disabled="saving" @click="toggle(row)"><span class="pointer-events-none mt-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform" :class="row.enabled ? 'translate-x-5' : 'translate-x-0.5'" /></button></template>
        <template #cell-sort_order="{ value }"><span class="font-mono text-sm">{{ value }}</span></template>
        <template #cell-actions="{ row }"><button class="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-950/20" type="button" @click="openEdit(row)"><Icon name="edit" size="sm" />编辑</button></template>
      </DataTable>
    </div>
  </section>

  <BaseDialog :show="editing !== null" title="编辑本站套餐展示" width="wide" @close="closeEdit">
    <div v-if="editing" class="space-y-5">
      <div class="grid gap-3 rounded-xl border border-gray-200 bg-gray-50 p-4 text-sm dark:border-dark-600 dark:bg-dark-700/40 sm:grid-cols-3">
        <div><p class="text-xs text-gray-500">主站套餐</p><p class="mt-1 font-medium">{{ editing.source_name }}</p></div>
        <div><p class="text-xs text-gray-500">主站价格</p><p class="mt-1 font-medium">{{ money(editing.price, editing.currency) }}</p></div>
        <div><p class="text-xs text-gray-500">主站有效期</p><p class="mt-1 font-medium">{{ validity(editing) }}</p></div>
      </div>
      <label class="flex items-center justify-between gap-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600"><span><span class="block font-medium text-gray-900 dark:text-white">在本站购买页展示</span><span class="mt-1 block text-xs text-gray-500">关闭后，购买页隐藏该套餐，手工提交该套餐 ID 也会被本站拒绝。</span></span><AgentToggle v-model="form.enabled" aria-label="在本站购买页展示" /></label>
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="input-label" for="plan-sort">本站排序</label><AgentInput id="plan-sort" v-model.number="form.sort_order" class="mt-2" min="0" max="100000" type="number" /></div>
        <div><label class="input-label" for="plan-name">本站显示名称</label><AgentInput id="plan-name" v-model="form.display_name" class="mt-2" maxlength="120" :placeholder="editing.source_name" /><p class="mt-1 text-xs text-gray-400">留空即继承主站名称。</p></div>
      </div>
      <div><label class="input-label" for="plan-description">本站套餐说明</label><AgentTextArea id="plan-description" v-model="form.description" class="mt-2 min-h-24" maxlength="2000" :placeholder="editing.source_description || '留空继承主站说明'" /></div>
      <div><label class="input-label" for="plan-features">本站权益说明</label><AgentTextArea id="plan-features" v-model="featuresText" class="mt-2 min-h-32 font-mono text-sm" placeholder="每行一项；留空继承主站权益" /><p class="mt-1 text-xs text-gray-400">最多 20 项，每项最多 200 个字符。</p></div>
    </div>
    <template #footer><button class="btn btn-secondary" type="button" :disabled="saving" @click="closeEdit">取消</button><button class="btn btn-primary" type="button" :disabled="saving" data-testid="save-plan-policy" @click="save"><Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="{ 'animate-spin': saving }" />{{ saving ? '正在保存…' : '保存本站策略' }}</button></template>
  </BaseDialog>
</template>
