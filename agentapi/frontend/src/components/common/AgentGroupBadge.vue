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
  if (props.exclusive) return 'bg-purple-50 text-purple-700 dark:bg-purple-500/10 dark:text-purple-300'
  const subscription = isSubscription.value
  const classes: Record<string, [string, string]> = {
    anthropic: ['bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400', 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'],
    openai: ['bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400', 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'],
    gemini: ['bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400', 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'],
    antigravity: ['bg-fuchsia-50 text-fuchsia-700 dark:bg-fuchsia-900/20 dark:text-fuchsia-400', 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'],
    grok: ['bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200', 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'],
    kimi: ['bg-pink-50 text-pink-700 dark:bg-pink-900/20 dark:text-pink-400', 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-400'],
    zhipu: ['bg-indigo-50 text-indigo-700 dark:bg-indigo-900/20 dark:text-indigo-400', 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400'],
    deepseek: ['bg-teal-50 text-teal-700 dark:bg-teal-900/20 dark:text-teal-400', 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400'],
    minimax: ['bg-rose-50 text-rose-700 dark:bg-rose-900/20 dark:text-rose-400', 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'],
    composite: ['bg-cyan-50 text-cyan-800 dark:bg-cyan-900/20 dark:text-cyan-300', 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-300'],
  }
  const tone = classes[normalizedPlatform.value]
  if (tone) return tone[subscription ? 1 : 0]
  return subscription
    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
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
    <span v-if="hasPeakRate" class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300" :title="peakRateText">
      {{ peakRateText }}
    </span>
  </span>
</template>
