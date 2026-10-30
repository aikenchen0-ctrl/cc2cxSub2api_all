<script setup lang="ts">
import { computed } from 'vue'
import AgentPlatformIcon from './AgentPlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  groupId?: number | string
  name: string
  platform?: string
  subscriptionType?: string
  rateMultiplier?: number
  userRateMultiplier?: number | null
  peakRateEnabled?: boolean
  peakStart?: string
  peakEnd?: string
  peakRateMultiplier?: number
  showRate?: boolean
  alwaysShowRate?: boolean
  exclusive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  platform: '',
  subscriptionType: 'standard',
  userRateMultiplier: null,
  peakRateEnabled: false,
  peakStart: '',
  peakEnd: '',
  showRate: true,
  alwaysShowRate: false,
  exclusive: false,
})

const normalizedPlatform = computed(() => props.platform.trim().toLowerCase())
const isSubscription = computed(() => props.subscriptionType === 'subscription')
const hasCustomRate = computed(() => (
  props.userRateMultiplier != null
  && props.rateMultiplier != null
  && props.userRateMultiplier !== props.rateMultiplier
))
const hasPeakRate = computed(() => Boolean(
  props.showRate && props.peakRateEnabled && props.peakStart && props.peakEnd,
))

function formatRate(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return ''
  return `${Number(value.toFixed(4))}x`
}

const labelText = computed(() => {
  if (isSubscription.value && !props.alwaysShowRate) return '订阅'
  return formatRate(props.rateMultiplier)
})
const showLabel = computed(() => props.showRate && (
  isSubscription.value || props.rateMultiplier != null || hasCustomRate.value
))
const peakRateText = computed(() => {
  const multiplier = formatRate(props.peakRateMultiplier)
  return `高峰 ${props.peakStart}–${props.peakEnd}${multiplier ? ` · ${multiplier}` : ''}`
})

const badgeClass = computed(() => {
  if (props.exclusive) return 'bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
  const subscription = isSubscription.value
  const classes: Record<string, [string, string]> = {
    anthropic: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    openai: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    gemini: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    antigravity: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    grok: ['bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200', 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'],
    kimi: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    zhipu: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    deepseek: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    minimax: ['bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'],
    composite: ['bg-primary-50 text-primary-800 dark:bg-primary-900/20 dark:text-primary-300', 'bg-primary-100 text-primary-800 dark:bg-primary-900/30 dark:text-primary-300'],
  }
  const tone = classes[normalizedPlatform.value]
  if (tone) return tone[subscription ? 1 : 0]
  return subscription
    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
    : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200'
})

const labelClass = computed(() => {
  const base = 'rounded px-1.5 py-0.5 text-[10px] font-semibold'
  return isSubscription.value
    ? `${base} bg-black/10 dark:bg-white/10`
    : `${base} bg-black/10 dark:bg-white/10`
})
</script>

<template>
  <span
    :class="['inline-flex max-w-full items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium transition-colors', badgeClass]"
    :data-group-id="groupId"
    :data-group-name="name"
    :data-platform="normalizedPlatform || undefined"
    :data-exclusive="exclusive ? 'true' : undefined"
  >
    <Icon v-if="exclusive" name="shield" size="xs" class="shrink-0" />
    <AgentPlatformIcon v-else-if="platform" :platform="normalizedPlatform" size="sm" />
    <span class="truncate">{{ name }}</span>
    <span v-if="showLabel" :class="labelClass">
      <template v-if="hasCustomRate">
        <span class="mr-0.5 line-through opacity-50">{{ formatRate(rateMultiplier) }}</span>
        <span class="font-bold">{{ formatRate(userRateMultiplier) }}</span>
      </template>
      <template v-else>{{ labelText }}</template>
    </span>
    <span v-if="hasPeakRate" class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300" :title="peakRateText">
      {{ peakRateText }}
    </span>
  </span>
</template>
