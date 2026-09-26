<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5">
      <div class="satellite-tabs-shell">
        <nav
          class="satellite-tabs-scroll"
          role="tablist"
          :aria-label="t('admin.satelliteBilling.title')"
          :aria-describedby="'satellite-billing-keyboard-hint'"
        >
          <div class="satellite-tabs">
            <button
              v-for="app in satelliteApps"
              :id="`satellite-billing-tab-${app.slug}`"
              :key="app.slug"
              type="button"
              role="tab"
              :aria-selected="activeAppSlug === app.slug"
              :aria-controls="`satellite-billing-panel-${app.slug}`"
              :tabindex="activeAppSlug === app.slug ? 0 : -1"
              :class="[
                'satellite-tab',
                activeAppSlug === app.slug && 'satellite-tab-active',
              ]"
              @click="selectApp(app.slug)"
              @keydown="handleTabKeydown($event, app.slug)"
            >
              <span class="satellite-tab-icon">
                <Icon :name="app.icon" size="sm" />
              </span>
              <span class="satellite-tab-label">{{ app.label }}</span>
            </button>
          </div>
        </nav>
      </div>
      <p id="satellite-billing-keyboard-hint" class="sr-only">
        {{ t('admin.satelliteBilling.keyboardHint') }}
      </p>

      <div v-if="loadError" class="billing-error" role="alert">
        <span>{{ loadError }}</span>
        <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="loadConfigs">
          {{ t('admin.satelliteBilling.retry') }}
        </button>
      </div>

      <section
        v-if="activeApp && activeConfig"
        :id="`satellite-billing-panel-${activeApp.slug}`"
        class="space-y-5"
        role="tabpanel"
        :aria-labelledby="`satellite-billing-tab-${activeApp.slug}`"
        tabindex="0"
      >
        <div class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-5 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between sm:px-6">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ activeApp.label }}
                </h2>
                <code class="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ activeApp.slug }}/
                </code>
              </div>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.satelliteBilling.appConfigDescription') }}
              </p>
              <p
                v-if="activeApp.slug === 'livart'"
                class="mt-1.5 flex items-start gap-1.5 text-xs text-primary-700 dark:text-primary-300"
              >
                <Icon name="infoCircle" size="xs" class="mt-0.5 shrink-0" />
                <span>{{ t('admin.satelliteBilling.livartSharedHint') }}</span>
              </p>
            </div>
            <span class="draft-status shrink-0" :class="isDirty ? 'draft-status-dirty' : 'draft-status-saved'">
              <span class="draft-status-dot"></span>
              {{ loading ? t('admin.satelliteBilling.loading') : isDirty ? t('admin.satelliteBilling.unsavedChanges') : t('admin.satelliteBilling.saved') }}
            </span>
          </div>

          <div class="space-y-4 p-5 sm:p-6">
            <div class="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.satelliteBilling.modeTitle') }}
              </h3>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.satelliteBilling.modeHint') }}
              </p>
            </div>

            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4" role="radiogroup" :aria-label="t('admin.satelliteBilling.modeTitle')">
              <button
                v-for="mode in billingModes"
                :key="mode.key"
                type="button"
                role="radio"
                :aria-checked="activeConfig.mode === mode.key"
                :class="[
                  'billing-mode-card',
                  activeConfig.mode === mode.key && 'billing-mode-card-active',
                ]"
                @click="selectBillingMode(mode.key)"
              >
                <div class="flex items-start justify-between gap-3">
                  <span class="billing-mode-icon" :class="`billing-mode-icon-${mode.key}`">
                    <Icon :name="mode.icon" size="sm" />
                  </span>
                  <Icon
                    v-if="activeConfig.mode === mode.key"
                    name="checkCircle"
                    size="sm"
                    class="shrink-0 text-primary-600 dark:text-primary-300"
                  />
                </div>
                <span class="mt-3 block text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t(`admin.satelliteBilling.modes.${mode.key}.title`) }}
                </span>
                <span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t(`admin.satelliteBilling.modes.${mode.key}.description`) }}
                </span>
              </button>
            </div>
          </div>
        </div>

        <section v-if="activeConfig.mode === 'model'" class="model-pricing-summary">
          <span class="model-pricing-icon">
            <Icon name="creditCard" size="md" />
          </span>
          <div>
            <h3 class="font-semibold text-gray-900 dark:text-white">
              {{ t('admin.satelliteBilling.modes.model.title') }}
            </h3>
            <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">
              {{ t('admin.satelliteBilling.modes.model.summary') }}
            </p>
          </div>
        </section>

        <section v-else class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between sm:px-6">
            <div>
              <h3 class="font-semibold text-gray-900 dark:text-white">
                {{ t(`admin.satelliteBilling.modes.${activeConfig.mode}.rulesTitle`) }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t(`admin.satelliteBilling.modes.${activeConfig.mode}.rulesHint`) }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm w-fit" @click="addRule">
              <Icon name="plus" size="xs" />
              {{ t('admin.satelliteBilling.addRule') }}
            </button>
          </div>

          <div v-if="activeConfig.rules.length" class="space-y-3 p-5 sm:p-6">
            <article
              v-for="(rule, index) in activeConfig.rules"
              :key="rule.id"
              class="billing-rule-row"
            >
              <div class="billing-rule-index">{{ index + 1 }}</div>
              <div class="grid min-w-0 flex-1 grid-cols-1 gap-3 md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_minmax(9rem,0.8fr)] md:items-end">
                <label class="block min-w-0">
                  <span class="input-label">{{ t('admin.satelliteBilling.ruleTarget') }}</span>
                  <input
                    v-model="rule.target"
                    type="text"
                    class="input"
                    :placeholder="t('admin.satelliteBilling.ruleTargetPlaceholder')"
                  />
                </label>
                <label class="block min-w-0">
                  <span class="input-label">{{ t('admin.satelliteBilling.unit') }}</span>
                  <select v-model="rule.unit" class="input">
                    <option v-for="unit in activeUnits" :key="unit.value" :value="unit.value">
                      {{ unit.label }}
                    </option>
                  </select>
                </label>
                <label class="block min-w-0">
                  <span class="input-label">
                    {{ activeConfig.mode === 'points' ? t('admin.satelliteBilling.pointsAmount') : t('admin.satelliteBilling.amount') }}
                  </span>
                  <input
                    v-model="rule.amount"
                    type="number"
                    min="0"
                    :step="activeConfig.mode === 'points' ? 1 : 'any'"
                    class="input"
                    placeholder="0"
                  />
                </label>
              </div>
              <button
                type="button"
                class="billing-rule-delete"
                :aria-label="t('admin.satelliteBilling.removeRule')"
                :title="t('admin.satelliteBilling.removeRule')"
                @click="removeRule(rule.id)"
              >
                <Icon name="trash" size="sm" />
              </button>
            </article>
          </div>

          <div v-else class="billing-rules-empty">
            <span class="billing-rules-empty-icon"><Icon name="document" size="md" /></span>
            <p class="font-medium text-gray-800 dark:text-gray-200">
              {{ t('admin.satelliteBilling.emptyRulesTitle') }}
            </p>
            <p class="mt-1 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">
              {{ t('admin.satelliteBilling.emptyRulesDescription') }}
            </p>
            <button type="button" class="btn btn-primary btn-sm mt-4" @click="addRule">
              <Icon name="plus" size="xs" />
              {{ t('admin.satelliteBilling.addRule') }}
            </button>
          </div>
        </section>

        <p v-if="ruleValidationError" class="text-sm text-red-600 dark:text-red-300" role="alert">
          {{ ruleValidationError }}
        </p>
        <footer class="flex flex-col gap-3 border-t border-gray-200 pt-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <p v-if="saveError" class="max-w-2xl text-xs leading-5 text-red-600 dark:text-red-300" role="alert">
            {{ saveError }}
          </p>
          <p v-else class="max-w-2xl text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.satelliteBilling.footerHint') }}
          </p>
          <button type="button" class="btn btn-primary shrink-0" :disabled="saveDisabled" @click="saveActiveConfig">
            <Icon name="check" size="sm" />
            {{ saving ? t('admin.satelliteBilling.saving') : t('admin.satelliteBilling.save') }}
          </button>
        </footer>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getSatelliteBillingConfigs,
  updateSatelliteBillingConfig,
  type SatelliteBillingAppSlug,
  type SatelliteBillingConfig,
  type SatelliteBillingUnit,
} from '@/api/admin/settings'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

type SatelliteSlug = SatelliteBillingAppSlug
type BillingMode = 'model' | 'custom' | 'request' | 'points'
type BillingUnit = SatelliteBillingUnit
interface BillingRule {
  id: number
  target: string
  unit: BillingUnit
  amount: string
}

interface AppBillingDraft {
  mode: BillingMode
  rules: BillingRule[]
}

const { t } = useI18n()

const satelliteApps = computed(() => [
  { slug: 'aicut' as const, label: t('nav.aiCut'), icon: 'edit' as const },
  { slug: 'aiexcel' as const, label: t('nav.aiExcel'), icon: 'calculator' as const },
  { slug: 'ai3d' as const, label: t('nav.ai3d'), icon: 'cube' as const },
  { slug: 'aihuoke' as const, label: t('nav.aihuoke'), icon: 'users' as const },
  { slug: 'canvas' as const, label: t('nav.infiniteCanvas'), icon: 'grid' as const },
  { slug: 'ju' as const, label: t('nav.smartShortDrama'), icon: 'play' as const },
  { slug: 'livart' as const, label: t('nav.superCanvas'), icon: 'sparkles' as const },
  { slug: 'ppt' as const, label: t('nav.eternalPpt'), icon: 'document' as const },
  { slug: 'qrcode' as const, label: t('nav.artQr'), icon: 'qrCode' as const },
  { slug: 'screen2code' as const, label: t('nav.screen2code'), icon: 'terminal' as const },
  { slug: 'yibiao' as const, label: t('nav.yibiao'), icon: 'chartBar' as const },
])

const billingModes: { key: BillingMode; icon: 'creditCard' | 'calculator' | 'clock' | 'sparkles' }[] = [
  { key: 'model', icon: 'creditCard' },
  { key: 'custom', icon: 'calculator' },
  { key: 'request', icon: 'clock' },
  { key: 'points', icon: 'sparkles' },
]

const activeAppSlug = ref<SatelliteSlug>('aicut')
const activeApp = computed(() => satelliteApps.value.find((app) => app.slug === activeAppSlug.value))
const satelliteSlugs = satelliteApps.value.map((app) => app.slug)
const appDrafts = reactive<Record<SatelliteSlug, AppBillingDraft>>(
  Object.fromEntries(satelliteSlugs.map((slug) => [slug, { mode: 'model', rules: [] }])) as unknown as Record<SatelliteSlug, AppBillingDraft>,
)
const savedSnapshots = reactive<Partial<Record<SatelliteSlug, string>>>({})
const activeConfig = computed(() => appDrafts[activeAppSlug.value])
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
let nextRuleId = 1

function toPayload(config: AppBillingDraft): SatelliteBillingConfig {
  return {
    mode: config.mode,
    rules: config.rules.map((rule) => ({
      target: rule.target.trim(),
      unit: rule.unit,
      amount: Number(rule.amount),
    })),
  }
}

const isDirty = computed(() => {
  const saved = savedSnapshots[activeAppSlug.value]
  return saved !== undefined && JSON.stringify(toPayload(activeConfig.value)) !== saved
})
const ruleValidationError = computed(() => {
  if (activeConfig.value.mode === 'model') return ''
  const seen = new Set<string>()
  for (const rule of activeConfig.value.rules) {
    const target = rule.target.trim()
    const amountText = rule.amount.trim()
    const amount = Number(amountText)
    if (!target) return t('admin.satelliteBilling.validation.targetRequired')
    if (!amountText || !Number.isFinite(amount) || amount < 0) return t('admin.satelliteBilling.validation.amountInvalid')
    if (activeConfig.value.mode === 'points' && !Number.isInteger(amount)) return t('admin.satelliteBilling.validation.pointsWhole')
    const key = `${target.toLowerCase()}\u0000${rule.unit}`
    if (seen.has(key)) return t('admin.satelliteBilling.validation.duplicateRule')
    seen.add(key)
  }
  return ''
})
const hasValidRules = computed(() => !ruleValidationError.value)
const saveDisabled = computed(() => loading.value || saving.value || Boolean(loadError.value) || !isDirty.value || !hasValidRules.value)

async function loadConfigs(): Promise<void> {
  loading.value = true
  loadError.value = ''
  saveError.value = ''
  try {
    const configs = await getSatelliteBillingConfigs()
    for (const slug of satelliteSlugs) {
      const config = configs[slug] ?? { mode: 'model', rules: [] }
      const draft: AppBillingDraft = {
        mode: config.mode,
        rules: config.rules.map((rule) => ({
          id: nextRuleId++,
          target: rule.target,
          unit: rule.unit,
          amount: String(rule.amount),
        })),
      }
      appDrafts[slug] = draft
      savedSnapshots[slug] = JSON.stringify(toPayload(draft))
    }
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('admin.satelliteBilling.loadFailed')
  } finally {
    loading.value = false
  }
}

async function saveActiveConfig(): Promise<void> {
  if (saveDisabled.value) return
  saving.value = true
  saveError.value = ''
  try {
    const saved = await updateSatelliteBillingConfig(activeAppSlug.value, toPayload(activeConfig.value))
    const draft: AppBillingDraft = {
      mode: saved.mode,
      rules: saved.rules.map((rule) => ({
        id: nextRuleId++,
        target: rule.target,
        unit: rule.unit,
        amount: String(rule.amount),
      })),
    }
    appDrafts[activeAppSlug.value] = draft
    savedSnapshots[activeAppSlug.value] = JSON.stringify(toPayload(draft))
  } catch (error) {
    saveError.value = error instanceof Error ? error.message : t('admin.satelliteBilling.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(loadConfigs)

const activeUnits = computed(() => {
  const keys: BillingUnit[] = activeConfig.value.mode === 'custom'
    ? ['millionTokens', 'image', 'videoSecond', 'request']
    : ['request', 'image', 'videoSecond']
  return keys.map((value) => ({ value, label: t(`admin.satelliteBilling.units.${value}`) }))
})

function selectApp(slug: SatelliteSlug): void {
  activeAppSlug.value = slug
}

function focusAppTab(slug: SatelliteSlug): void {
  window.requestAnimationFrame(() => {
    document.getElementById(`satellite-billing-tab-${slug}`)?.focus()
  })
}

function handleTabKeydown(event: KeyboardEvent, slug: SatelliteSlug): void {
  const keys = ['ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown', 'Home', 'End']
  if (!keys.includes(event.key)) return
  event.preventDefault()

  const currentIndex = satelliteApps.value.findIndex((app) => app.slug === slug)
  let nextIndex = currentIndex
  if (event.key === 'Home') nextIndex = 0
  else if (event.key === 'End') nextIndex = satelliteApps.value.length - 1
  else nextIndex = (currentIndex + (event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1) + satelliteApps.value.length) % satelliteApps.value.length

  const nextApp = satelliteApps.value[nextIndex]
  if (!nextApp) return
  selectApp(nextApp.slug)
  focusAppTab(nextApp.slug)
}

function selectBillingMode(mode: BillingMode): void {
  const config = activeConfig.value
  config.mode = mode
  if (mode === 'model') {
    config.rules = config.rules.filter((rule) => {
      const amount = Number(rule.amount)
      return rule.target.trim().length > 0 && rule.amount.trim().length > 0 && Number.isFinite(amount) && amount >= 0
    })
  }
  const allowedUnits = mode === 'custom'
    ? ['millionTokens', 'image', 'videoSecond', 'request']
    : ['request', 'image', 'videoSecond']
  for (const rule of config.rules) {
    if (!allowedUnits.includes(rule.unit)) rule.unit = allowedUnits[0] as BillingUnit
  }
}

function addRule(): void {
  const defaultUnit: BillingUnit = activeConfig.value.mode === 'custom' ? 'millionTokens' : 'request'
  activeConfig.value.rules.push({ id: nextRuleId++, target: '', unit: defaultUnit, amount: '' })
}

function removeRule(id: number): void {
  activeConfig.value.rules = activeConfig.value.rules.filter((rule) => rule.id !== id)
}
</script>

<style scoped>
.model-pricing-icon {
  @apply flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-white text-sky-700 shadow-sm dark:bg-dark-800 dark:text-sky-300;
}

.satellite-tabs-shell {
  @apply sticky z-20 -mx-1 rounded-2xl border border-white/80 bg-white/90 p-1.5 backdrop-blur-xl;
  top: 4.75rem;
  box-shadow: 0 12px 28px rgb(15 23 42 / 0.07), 0 1px 0 rgb(255 255 255 / 0.9) inset;
}

.satellite-tabs-scroll {
  @apply overflow-x-auto overscroll-x-contain;
  scrollbar-width: thin;
  scrollbar-color: rgb(148 163 184 / 0.7) transparent;
}

.satellite-tabs-scroll::-webkit-scrollbar { height: 7px; }
.satellite-tabs-scroll::-webkit-scrollbar-track { @apply rounded-full bg-gray-100 dark:bg-dark-800; }
.satellite-tabs-scroll::-webkit-scrollbar-thumb { @apply rounded-full bg-gray-300 dark:bg-dark-600; }
.satellite-tabs-scroll::-webkit-scrollbar-thumb:hover { @apply bg-gray-400 dark:bg-dark-500; }
.satellite-tabs { @apply flex w-max min-w-full items-center gap-1; }

.satellite-tab {
  @apply relative isolate flex h-10 min-w-[8.5rem] shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-transparent px-3 text-sm font-medium text-gray-600 outline-none transition-colors duration-200 dark:text-gray-300;
}

.satellite-tab::before {
  @apply absolute inset-0 -z-10 rounded-xl opacity-0 transition-opacity duration-200;
  content: '';
  background: linear-gradient(135deg, rgb(248 250 252 / 0.95), rgb(241 245 249 / 0.8));
}

.satellite-tab:hover::before,
.satellite-tab:focus-visible::before { opacity: 1; }
.satellite-tab:focus-visible { @apply ring-2 ring-primary-500/40 ring-offset-2 ring-offset-white dark:ring-offset-dark-900; }

.satellite-tab-active {
  @apply border-primary-200/80 bg-white text-primary-700 shadow-sm dark:border-primary-400/30 dark:bg-dark-700/95 dark:text-primary-200;
  box-shadow: 0 8px 18px rgb(15 23 42 / 0.08), 0 1px 0 rgb(255 255 255 / 0.92) inset;
}

.satellite-tab-active::after {
  position: absolute;
  right: 0.75rem;
  bottom: 0.25rem;
  left: 0.75rem;
  height: 2px;
  border-radius: 9999px;
  content: '';
  background: linear-gradient(90deg, #14b8a6, #0ea5e9);
}

.satellite-tab-icon {
  @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors dark:text-gray-400;
}
.satellite-tab-active .satellite-tab-icon { @apply bg-primary-50 text-primary-600 dark:bg-primary-400/10 dark:text-primary-300; }
.satellite-tab-label { @apply min-w-0 overflow-hidden text-ellipsis whitespace-nowrap leading-none; }

.draft-status {
  @apply inline-flex items-center gap-2 rounded-full border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs font-medium text-amber-800 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-300;
}
.draft-status-dirty { @apply border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-300; }
.draft-status-saved { @apply border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900/70 dark:bg-emerald-950/30 dark:text-emerald-300; }
.draft-status-dot { @apply h-1.5 w-1.5 rounded-full bg-amber-500; }
.draft-status-saved .draft-status-dot { @apply bg-emerald-500; }

.billing-error { @apply flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-300; }

.billing-mode-card {
  @apply flex min-h-[142px] flex-col rounded-xl border border-gray-200 bg-white p-4 text-left transition duration-150 hover:border-primary-300 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-500/60;
}
.billing-mode-card-active {
  @apply border-primary-400 bg-primary-50/50 shadow-sm dark:border-primary-500/70 dark:bg-primary-950/20;
}
.billing-mode-icon { @apply flex h-9 w-9 items-center justify-center rounded-xl; }
.billing-mode-icon-model { @apply bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200; }
.billing-mode-icon-custom { @apply bg-violet-50 text-violet-700 dark:bg-violet-950/50 dark:text-violet-300; }
.billing-mode-icon-request { @apply bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300; }
.billing-mode-icon-points { @apply bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300; }

.model-pricing-summary {
  @apply flex items-start gap-3 rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800 sm:p-6;
}

.billing-rule-row {
  @apply flex items-start gap-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-700 dark:bg-dark-800/70 sm:gap-4 sm:p-4;
}
.billing-rule-index { @apply mt-7 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-white text-xs font-semibold text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300; }
.billing-rule-delete { @apply mt-7 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-gray-400 transition hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/40 dark:hover:bg-red-950/40 dark:hover:text-red-300; }

.billing-rules-empty { @apply flex flex-col items-center px-5 py-10 text-center; }
.billing-rules-empty-icon { @apply mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300; }
</style>

<style>
.dark .satellite-tabs-shell {
  border-color: rgb(51 65 85 / 0.65);
  background: rgb(15 23 42 / 0.86);
  box-shadow: 0 16px 36px rgb(0 0 0 / 0.28), 0 1px 0 rgb(255 255 255 / 0.06) inset;
}

.dark .satellite-tab::before {
  background: linear-gradient(135deg, rgb(30 41 59 / 0.9), rgb(51 65 85 / 0.62));
}

.dark .satellite-tab-active {
  box-shadow: 0 12px 26px rgb(0 0 0 / 0.22), 0 1px 0 rgb(255 255 255 / 0.08) inset;
}

.dark .satellite-tabs-scroll { scrollbar-color: rgb(71 85 105 / 0.9) transparent; }
</style>
