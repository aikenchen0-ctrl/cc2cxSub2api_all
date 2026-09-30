<script setup lang="ts">
import BaseDialog from '@/components/common/BaseDialog.vue'

withDefaults(defineProps<{
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

let messageIDSequence = 0
const messageID = `agent-confirm-message-${++messageIDSequence}`
</script>

<template>
  <BaseDialog
    :show="open"
    :title="title"
    width="narrow"
    dialog-role="alertdialog"
    :aria-describedby="messageID"
    @close="emit('cancel')"
  >
    <div class="space-y-4">
      <p :id="messageID" class="text-sm leading-6 text-gray-600 dark:text-gray-400">{{ message }}</p>
    </div>

    <template #footer>
      <div class="flex justify-end space-x-3">
        <button
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600 dark:focus:ring-offset-dark-800"
          type="button"
          @click="emit('cancel')"
        >{{ cancelLabel }}</button>
        <button
          data-testid="agent-confirm-action"
          class="rounded-md px-4 py-2 text-sm font-medium text-white focus:outline-none focus:ring-2 focus:ring-offset-2 dark:focus:ring-offset-dark-800"
          :class="destructive ? 'bg-red-600 hover:bg-red-700 focus:ring-red-500' : 'bg-primary-600 hover:bg-primary-700 focus:ring-primary-500'"
          type="button"
          @click="emit('confirm')"
        >{{ confirmLabel }}</button>
      </div>
    </template>
  </BaseDialog>
</template>
