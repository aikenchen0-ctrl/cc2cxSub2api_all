<template>
  <div class="relative w-full">
    <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
      <Icon name="search" size="md" class="text-gray-400" aria-hidden="true" />
    </div>
    <input
      v-bind="$attrs"
      :value="modelValue"
      type="search"
      class="input w-full pl-10"
      :class="{ 'pr-10': clearable && modelValue }"
      :placeholder="placeholder"
      :aria-label="ariaLabel || placeholder"
      autocomplete="off"
      @input="handleInput"
    />
    <button
      v-if="clearable && modelValue"
      type="button"
      class="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 transition hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:hover:text-dark-200"
      :aria-label="clearAriaLabel"
      @click="clearInput"
    >
      <Icon name="x" size="xs" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  ariaLabel?: string
  debounceMs?: number
  clearable?: boolean
  clearAriaLabel?: string
}>(), {
  placeholder: '搜索…',
  ariaLabel: '',
  debounceMs: 300,
  clearable: false,
  clearAriaLabel: '清空搜索',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  search: [value: string]
}>()

let searchTimer: ReturnType<typeof setTimeout> | undefined
let pendingValue: string | undefined

function clearSearchTimer(): void {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = undefined
  pendingValue = undefined
}

function scheduleSearch(value: string): void {
  clearSearchTimer()
  if (props.debounceMs <= 0) {
    emit('search', value)
    return
  }
  pendingValue = value
  searchTimer = setTimeout(() => {
    searchTimer = undefined
    pendingValue = undefined
    emit('search', value)
  }, props.debounceMs)
}

function handleInput(event: Event): void {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', value)
  scheduleSearch(value)
}

function clearInput(): void {
  clearSearchTimer()
  emit('update:modelValue', '')
  emit('search', '')
}

watch(() => props.modelValue, (value) => {
  if (pendingValue !== undefined && value !== pendingValue) clearSearchTimer()
})

onBeforeUnmount(clearSearchTimer)
</script>
