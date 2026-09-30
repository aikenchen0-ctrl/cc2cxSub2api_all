<template>
  <div class="w-full">
    <label v-if="label" :for="id" class="input-label mb-1.5 block">
      {{ label }}
      <span v-if="required" class="text-red-500">*</span>
    </label>
    <textarea
      :id="id"
      ref="textAreaRef"
      v-bind="$attrs"
      :value="modelValue ?? ''"
      :disabled="disabled"
      :required="required"
      :placeholder="placeholder"
      :readonly="readonly"
      :rows="rows"
      :class="[
        'input w-full min-h-[80px] resize-y transition-all duration-200',
        error && 'input-error ring-2 ring-red-500/20',
        disabled && 'cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900',
      ]"
      @input="onInput"
      @change="$emit('change', ($event.target as HTMLTextAreaElement).value)"
      @blur="$emit('blur', $event)"
      @focus="$emit('focus', $event)"
    ></textarea>
    <p v-if="error" class="input-error-text mt-1.5">{{ error }}</p>
    <p v-else-if="hint" class="input-hint mt-1.5">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{
  modelValue: string | null | undefined
  label?: string
  placeholder?: string
  disabled?: boolean
  required?: boolean
  readonly?: boolean
  error?: string
  hint?: string
  id?: string
  rows?: number | string
}>(), {
  placeholder: '',
  disabled: false,
  required: false,
  readonly: false,
  rows: 3,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'change', value: string): void
  (event: 'blur', value: FocusEvent): void
  (event: 'focus', value: FocusEvent): void
}>()

const textAreaRef = ref<HTMLTextAreaElement | null>(null)

function onInput(event: Event): void {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

defineExpose({
  focus: () => textAreaRef.value?.focus(),
  select: () => textAreaRef.value?.select(),
})
</script>
