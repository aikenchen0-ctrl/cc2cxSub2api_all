<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}>(), {
  confirmLabel: '确认',
  cancelLabel: '取消',
  destructive: false,
})

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const panel = ref<HTMLElement | null>(null)
const cancelButton = ref<HTMLButtonElement | null>(null)
const confirmButton = ref<HTMLButtonElement | null>(null)
let previouslyFocused: HTMLElement | null = null

watch(() => props.open, async (open) => {
  if (typeof document === 'undefined') return
  if (open) {
    previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
    await nextTick()
    confirmButton.value?.focus()
    return
  }

  const target = previouslyFocused
  previouslyFocused = null
  await nextTick()
  if (target?.isConnected) target.focus()
})

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('cancel')
    return
  }
  if (event.key !== 'Tab') return

  const active = document.activeElement
  if (event.shiftKey && active === cancelButton.value) {
    event.preventDefault()
    confirmButton.value?.focus()
  } else if (!event.shiftKey && active === confirmButton.value) {
    event.preventDefault()
    cancelButton.value?.focus()
  } else if (!panel.value?.contains(active)) {
    event.preventDefault()
    confirmButton.value?.focus()
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      @click.self="emit('cancel')"
      @keydown="handleKeydown"
    >
      <section
        ref="panel"
        class="w-full max-w-md rounded-xl border border-slate-200 bg-white p-6 shadow-xl dark:border-slate-700 dark:bg-slate-900"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="agent-confirm-title"
        aria-describedby="agent-confirm-message"
        tabindex="-1"
      >
        <h2 id="agent-confirm-title" class="text-lg font-semibold text-slate-900 dark:text-white">{{ title }}</h2>
        <p id="agent-confirm-message" class="mt-3 text-sm leading-6 text-slate-600 dark:text-slate-300">{{ message }}</p>
        <div class="mt-6 flex justify-end gap-3">
          <button
            ref="cancelButton"
            class="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-200 dark:hover:bg-slate-800"
            type="button"
            @click="emit('cancel')"
          >{{ cancelLabel }}</button>
          <button
            ref="confirmButton"
            data-testid="agent-confirm-action"
            class="rounded-lg px-4 py-2 text-sm font-medium text-white"
            :class="destructive ? 'bg-red-600 hover:bg-red-700' : 'bg-blue-600 hover:bg-blue-700'"
            type="button"
            @click="emit('confirm')"
          >{{ confirmLabel }}</button>
        </div>
      </section>
    </div>
  </Teleport>
</template>
