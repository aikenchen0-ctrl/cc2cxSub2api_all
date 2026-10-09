<script setup lang="ts">
import AgentSelect, { type AgentSelectOption } from './AgentSelect.vue'

export type SelectOption = AgentSelectOption

const props = defineProps<{
  modelValue: string | number | boolean | null | undefined
  options: AgentSelectOption[] | Array<Record<string, unknown>>
  disabled?: boolean
}>()
const emit = defineEmits<{
  'update:modelValue': [value: string | number | boolean | null | undefined]
  change: [value: unknown]
}>()
</script>

<template>
  <AgentSelect
    :model-value="props.modelValue"
    :options="props.options"
    :disabled="props.disabled"
    @update:model-value="emit('update:modelValue', $event)"
    @change="emit('change', $event)"
  />
</template>
