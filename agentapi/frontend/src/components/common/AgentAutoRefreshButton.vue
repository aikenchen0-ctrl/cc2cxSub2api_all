<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  enabled: boolean
  intervalSeconds: number
  countdown: number
  intervals: readonly number[]
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:interval': [value: number]
}>()

const showDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

function selectEnabled(value: boolean): void {
  emit('update:enabled', value)
}

function selectInterval(value: number): void {
  emit('update:interval', value)
}

function handleClickOutside(event: MouseEvent): void {
  const target = event.target
  if (target instanceof Node && dropdownRef.value && !dropdownRef.value.contains(target)) {
    showDropdown.value = false
  }
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') showDropdown.value = false
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div ref="dropdownRef" class="relative">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
      aria-label="自动刷新设置"
      :aria-expanded="showDropdown"
      aria-haspopup="menu"
      @click="showDropdown = !showDropdown"
    >
      <Icon name="refresh" size="sm" :class="enabled ? 'animate-spin' : ''" />
      <span>{{ enabled ? `自动刷新：${countdown} 秒` : '自动刷新' }}</span>
    </button>

    <div
      v-if="showDropdown"
      class="absolute right-0 z-20 mt-1 w-44 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
      role="menu"
    >
      <div class="p-1.5">
        <button
          type="button"
          class="flex w-full items-center justify-between rounded-md px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
          role="menuitemcheckbox"
          :aria-checked="enabled"
          @click="selectEnabled(!enabled)"
        >
          <span>启用自动刷新</span>
          <Icon v-if="enabled" name="check" size="sm" class="text-primary-500" />
        </button>
        <div class="my-1 border-t border-gray-100 dark:border-dark-700" />
        <button
          v-for="seconds in intervals"
          :key="seconds"
          type="button"
          class="flex w-full items-center justify-between rounded-md px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
          role="menuitemradio"
          :aria-checked="intervalSeconds === seconds"
          @click="selectInterval(seconds)"
        >
          <span>{{ seconds }} 秒</span>
          <Icon v-if="intervalSeconds === seconds" name="check" size="sm" class="text-primary-500" />
        </button>
      </div>
    </div>
  </div>
</template>
