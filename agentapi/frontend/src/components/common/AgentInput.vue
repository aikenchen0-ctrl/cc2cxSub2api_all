<template>
  <div class="w-full">
    <label v-if="label" :for="id" class="input-label mb-1.5 block">
      {{ label }}
      <span v-if="required" class="text-red-500">*</span>
    </label>
    <div class="relative">
      <div
        v-if="$slots.prefix"
        class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5 text-gray-400 dark:text-dark-400"
      >
        <slot name="prefix"></slot>
      </div>
      <input
        :id="id"
        ref="inputRef"
        v-bind="$attrs"
        :type="type"
        :value="modelValue ?? ''"
        :disabled="disabled"
        :required="required"
        :placeholder="placeholder"
        :autocomplete="autocomplete"
        :readonly="readonly"
        :class="[
          'input w-full transition-all duration-200',
          $slots.prefix && 'pl-11',
          $slots.suffix && 'pr-11',
          error && 'input-error ring-2 ring-red-500/20',
          disabled && 'cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900',
        ]"
        @input="onInput"
        @change="$emit('change', normalize(($event.target as HTMLInputElement).value))"
        @blur="$emit('blur', $event)"
        @focus="$emit('focus', $event)"
        @keyup.enter="$emit('enter', $event)"
      />
      <div
        v-if="$slots.suffix"
        class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 dark:text-dark-400"
      >
        <slot name="suffix"></slot>
      </div>
    </div>
    <p v-if="error" class="input-error-text mt-1.5">{{ error }}</p>
    <p v-else-if="hint" class="input-hint mt-1.5">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  modelValue: string | number | null | undefined
  modelModifiers?: { trim?: boolean; number?: boolean }
  type?: string
  label?: string
  placeholder?: string
  disabled?: boolean
  required?: boolean
  readonly?: boolean
  error?: string
  hint?: string
  id?: string
  autocomplete?: string
}>(), {
  modelModifiers: () => ({}),
  type: 'text',
  placeholder: '',
  disabled: false,
  required: false,
  readonly: false,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: string | number): void
  (event: 'change', value: string | number): void
  (event: 'blur', value: FocusEvent): void
  (event: 'focus', value: FocusEvent): void
  (event: 'enter', value: KeyboardEvent): void
}>()

const inputRef = ref<HTMLInputElement | null>(null)

function normalize(rawValue: string): string | number {
  const value = props.modelModifiers?.trim ? rawValue.trim() : rawValue
  if (!props.modelModifiers?.number || value === '') return value
  const parsed = Number(value)
  return Number.isNaN(parsed) ? value : parsed
}

function onInput(event: Event): void {
  emit('update:modelValue', normalize((event.target as HTMLInputElement).value))
}

defineExpose({
  focus: () => inputRef.value?.focus(),
  select: () => inputRef.value?.select(),
  hasTypeMismatch: () => Boolean(inputRef.value?.validity.typeMismatch),
})
</script>
