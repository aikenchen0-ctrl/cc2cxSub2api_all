<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'

interface DatePreset {
  label: string
  value: string
  getRange: () => { start: string; end: string }
}

const props = withDefaults(defineProps<{
  startDate: string
  endDate: string
  ariaLabel?: string
}>(), {
  ariaLabel: '选择日期范围',
})

const emit = defineEmits<{
  (event: 'update:startDate', value: string): void
  (event: 'update:endDate', value: string): void
  (event: 'change', value: { startDate: string; endDate: string; preset: string | null }): void
}>()

const isOpen = ref(false)
const container = ref<HTMLElement | null>(null)
const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const activePreset = ref<string | null>(null)

function localDate(value: Date): string {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const today = computed(() => localDate(new Date()))
const tomorrow = computed(() => {
  const value = new Date()
  value.setDate(value.getDate() + 1)
  return localDate(value)
})

const presets: DatePreset[] = [
  { label: '今天', value: 'today', getRange: () => ({ start: today.value, end: today.value }) },
  {
    label: '昨天',
    value: 'yesterday',
    getRange: () => {
      const value = new Date()
      value.setDate(value.getDate() - 1)
      const date = localDate(value)
      return { start: date, end: date }
    },
  },
  {
    label: '近 7 天',
    value: '7days',
    getRange: () => {
      const value = new Date()
      value.setDate(value.getDate() - 6)
      return { start: localDate(value), end: today.value }
    },
  },
  {
    label: '近 14 天',
    value: '14days',
    getRange: () => {
      const value = new Date()
      value.setDate(value.getDate() - 13)
      return { start: localDate(value), end: today.value }
    },
  },
  {
    label: '近 30 天',
    value: '30days',
    getRange: () => {
      const value = new Date()
      value.setDate(value.getDate() - 29)
      return { start: localDate(value), end: today.value }
    },
  },
  {
    label: '本月',
    value: 'thisMonth',
    getRange: () => {
      const value = new Date()
      return { start: localDate(new Date(value.getFullYear(), value.getMonth(), 1)), end: today.value }
    },
  },
  {
    label: '上月',
    value: 'lastMonth',
    getRange: () => {
      const value = new Date()
      return {
        start: localDate(new Date(value.getFullYear(), value.getMonth() - 1, 1)),
        end: localDate(new Date(value.getFullYear(), value.getMonth(), 0)),
      }
    },
  },
]

const invalidRange = computed(() => Boolean(
  localStartDate.value
  && localEndDate.value
  && localStartDate.value > localEndDate.value,
))

function readableDate(value: string): string {
  const parsed = new Date(`${value}T00:00:00`)
  return Number.isNaN(parsed.getTime())
    ? value
    : parsed.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

const displayValue = computed(() => {
  const preset = presets.find((item) => item.value === activePreset.value)
  if (preset) return preset.label
  if (localStartDate.value && localEndDate.value) {
    return localStartDate.value === localEndDate.value
      ? readableDate(localStartDate.value)
      : `${readableDate(localStartDate.value)} - ${readableDate(localEndDate.value)}`
  }
  if (localStartDate.value) return `从 ${readableDate(localStartDate.value)} 起`
  if (localEndDate.value) return `截至 ${readableDate(localEndDate.value)}`
  return '选择日期范围'
})

function detectPreset(): void {
  activePreset.value = null
  for (const preset of presets) {
    const range = preset.getRange()
    if (range.start === localStartDate.value && range.end === localEndDate.value) {
      activePreset.value = preset.value
      return
    }
  }
}

function selectPreset(preset: DatePreset): void {
  const range = preset.getRange()
  localStartDate.value = range.start
  localEndDate.value = range.end
  activePreset.value = preset.value
}

function apply(): void {
  if (invalidRange.value) return
  emit('update:startDate', localStartDate.value)
  emit('update:endDate', localEndDate.value)
  emit('change', {
    startDate: localStartDate.value,
    endDate: localEndDate.value,
    preset: activePreset.value,
  })
  isOpen.value = false
}

function clear(): void {
  localStartDate.value = ''
  localEndDate.value = ''
  activePreset.value = null
  apply()
}

function onDocumentClick(event: MouseEvent): void {
  if (container.value && !container.value.contains(event.target as Node)) isOpen.value = false
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') isOpen.value = false
}

watch(() => props.startDate, (value) => { localStartDate.value = value; detectPreset() })
watch(() => props.endDate, (value) => { localEndDate.value = value; detectPreset() })

onMounted(() => {
  detectPreset()
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div ref="container" class="relative">
    <button
      type="button"
      class="flex min-h-10 items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition hover:border-gray-300 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500"
      :class="{ 'border-primary-500 ring-2 ring-primary-500/30': isOpen }"
      :aria-label="ariaLabel"
      :aria-expanded="isOpen"
      aria-haspopup="dialog"
      @click="isOpen = !isOpen"
    >
      <Icon name="calendar" size="sm" class="text-gray-400" aria-hidden="true" />
      <span data-testid="date-range-value">{{ displayValue }}</span>
      <Icon name="chevronDown" size="sm" class="text-gray-400 transition-transform" :class="{ 'rotate-180': isOpen }" aria-hidden="true" />
    </button>

    <Transition name="agent-date-picker">
      <div
        v-if="isOpen"
        class="absolute left-0 z-[100] mt-2 min-w-[320px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
        role="dialog"
        :aria-label="`${ariaLabel}面板`"
      >
        <div class="grid grid-cols-2 gap-1 p-2">
          <button
            v-for="preset in presets"
            :key="preset.value"
            type="button"
            class="rounded-md px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700"
            :class="{ 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300': activePreset === preset.value }"
            :aria-pressed="activePreset === preset.value"
            @click="selectPreset(preset)"
          >
            {{ preset.label }}
          </button>
        </div>
        <div class="border-t border-gray-100 p-3 dark:border-dark-700">
          <div class="flex items-end gap-2">
            <label class="min-w-0 flex-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              开始日期
              <input v-model="localStartDate" type="date" class="input mt-1 px-2 py-1.5 text-sm" :max="localEndDate || tomorrow" aria-label="日期范围开始日期" @change="detectPreset" />
            </label>
            <Icon name="arrowRight" size="sm" class="mb-2 text-gray-400" aria-hidden="true" />
            <label class="min-w-0 flex-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              结束日期
              <input v-model="localEndDate" type="date" class="input mt-1 px-2 py-1.5 text-sm" :min="localStartDate" :max="tomorrow" aria-label="日期范围结束日期" @change="detectPreset" />
            </label>
          </div>
          <p v-if="invalidRange" class="mt-2 text-xs text-red-600 dark:text-red-300" role="alert">开始日期不能晚于结束日期。</p>
        </div>
        <div class="flex justify-between gap-2 px-3 pb-3">
          <button type="button" class="btn btn-ghost px-3 py-1.5 text-sm" @click="clear">清除</button>
          <button type="button" class="btn btn-primary px-4 py-1.5 text-sm" :disabled="invalidRange" @click="apply">应用</button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.agent-date-picker-enter-active,
.agent-date-picker-leave-active { transition: opacity 0.2s ease, transform 0.2s ease; }
.agent-date-picker-enter-from,
.agent-date-picker-leave-to { opacity: 0; transform: translateY(-8px); }

@media (prefers-reduced-motion: reduce) {
  .agent-date-picker-enter-active,
  .agent-date-picker-leave-active { transition: none; }
}
</style>
