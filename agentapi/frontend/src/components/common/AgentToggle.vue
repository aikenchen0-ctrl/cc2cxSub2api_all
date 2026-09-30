<template>
  <button
    :id="id"
    v-bind="$attrs"
    type="button"
    role="switch"
    :disabled="disabled"
    :aria-checked="modelValue"
    :class="[
      'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus:ring-offset-dark-800',
      modelValue ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600',
    ]"
    @click="toggle"
  >
    <span
      :class="[
        'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
        modelValue ? 'translate-x-5' : 'translate-x-0',
      ]"
    ></span>
  </button>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  modelValue: boolean
  disabled?: boolean
  id?: string
}>(), {
  disabled: false,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
  (event: 'change', value: boolean): void
}>()

function toggle(): void {
  if (props.disabled) return
  const value = !props.modelValue
  emit('update:modelValue', value)
  emit('change', value)
}
</script>
