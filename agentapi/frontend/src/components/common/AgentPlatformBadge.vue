<template>
  <span
    class="inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-semibold"
    :class="badgeClass"
    :data-platform="normalizedPlatform"
  >
    <AgentPlatformIcon :platform="normalizedPlatform" size="xs" aria-hidden="true" />
    <span>{{ displayLabel }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AgentPlatformIcon from './AgentPlatformIcon.vue'

const props = withDefaults(defineProps<{
  platform?: string | null
  label?: string
}>(), {
  platform: '',
  label: '',
})

const normalizedPlatform = computed(() => (props.platform || '').trim().toLowerCase())

const displayLabel = computed(() => {
  if (props.label) return props.label
  switch (normalizedPlatform.value) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    case 'grok': return 'Grok'
    case 'kimi': return 'Kimi'
    case 'zhipu': return 'Zhipu GLM'
    case 'deepseek': return 'DeepSeek'
    case 'minimax': return 'MiniMax'
    case 'opencode_go': return 'OpenCode'
    case 'composite': return 'Composite'
    default: return props.platform || 'Unknown'
  }
})

const badgeClass = computed(() => {
  switch (normalizedPlatform.value) {
    case 'anthropic': return 'border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400'
    case 'openai': return 'border-green-500/30 bg-green-500/10 text-green-600 dark:text-green-400'
    case 'antigravity': return 'border-purple-500/30 bg-purple-500/10 text-purple-600 dark:text-purple-400'
    case 'gemini': return 'border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400'
    case 'grok': return 'border-zinc-800/30 bg-zinc-800/10 text-zinc-800 dark:border-zinc-500/30 dark:bg-zinc-500/10 dark:text-zinc-200'
    case 'kimi': return 'border-pink-500/30 bg-pink-500/10 text-pink-600 dark:text-pink-400'
    case 'zhipu': return 'border-indigo-500/30 bg-indigo-500/10 text-indigo-600 dark:text-indigo-400'
    case 'deepseek': return 'border-teal-500/30 bg-teal-500/10 text-teal-600 dark:text-teal-400'
    case 'minimax': return 'border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'
    case 'opencode_go': return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'composite': return 'border-cyan-500/30 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300'
    default: return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-400'
  }
})
</script>
