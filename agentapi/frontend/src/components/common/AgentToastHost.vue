<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { useAgentToast, type AgentToastType } from '@/composables/useAgentToast'

const { toasts, removeToast } = useAgentToast()

const iconName: Record<AgentToastType, 'checkCircle' | 'xCircle' | 'exclamationTriangle' | 'infoCircle'> = {
  success: 'checkCircle',
  error: 'xCircle',
  warning: 'exclamationTriangle',
  info: 'infoCircle',
}

const iconClass: Record<AgentToastType, string> = {
  success: 'text-emerald-500',
  error: 'text-red-500',
  warning: 'text-amber-500',
  info: 'text-blue-500',
}

const borderClass: Record<AgentToastType, string> = {
  success: 'border-emerald-500',
  error: 'border-red-500',
  warning: 'border-amber-500',
  info: 'border-blue-500',
}

const progressClass: Record<AgentToastType, string> = {
  success: 'bg-emerald-500',
  error: 'bg-red-500',
  warning: 'bg-amber-500',
  info: 'bg-blue-500',
}
</script>

<template>
  <Teleport to="body">
    <div
      class="pointer-events-none fixed right-4 top-4 z-[10000] w-[calc(100%-2rem)] max-w-md space-y-3 sm:min-w-[320px] sm:w-auto"
      aria-live="polite"
      aria-relevant="additions removals"
    >
      <TransitionGroup name="agent-toast">
        <article
          v-for="toast in toasts"
          :key="toast.id"
          :data-toast-id="toast.id"
          :data-toast-type="toast.type"
          :class="[
            'pointer-events-auto overflow-hidden rounded-lg border-l-4 bg-white shadow-lg dark:bg-dark-800',
            borderClass[toast.type],
          ]"
          role="status"
        >
          <div class="flex items-start gap-3 p-4">
            <Icon
              :name="iconName[toast.type]"
              size="md"
              :class="['mt-0.5 shrink-0', iconClass[toast.type]]"
              aria-hidden="true"
            />
            <div class="min-w-0 flex-1">
              <p v-if="toast.title" class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ toast.title }}
              </p>
              <p :class="['break-words text-sm leading-relaxed', toast.title ? 'mt-1 text-gray-600 dark:text-gray-300' : 'text-gray-900 dark:text-white']">
                {{ toast.message }}
              </p>
            </div>
            <button
              type="button"
              class="-m-1 shrink-0 rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:text-gray-500 dark:hover:bg-dark-700 dark:hover:text-gray-300"
              :aria-label="`关闭通知：${toast.title || toast.message}`"
              @click="removeToast(toast.id)"
            >
              <Icon name="x" size="sm" aria-hidden="true" />
            </button>
          </div>
          <div v-if="toast.duration && toast.duration > 0" class="h-1 bg-gray-100 dark:bg-dark-700" aria-hidden="true">
            <div
              :class="['agent-toast-progress h-full', progressClass[toast.type]]"
              :style="{ animationDuration: `${toast.duration}ms` }"
            />
          </div>
        </article>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.agent-toast-enter-active { transition: opacity 0.3s ease-out, transform 0.3s ease-out; }
.agent-toast-leave-active { transition: opacity 0.2s ease-in, transform 0.2s ease-in; }
.agent-toast-enter-from,
.agent-toast-leave-to { opacity: 0; transform: translateX(100%); }

.agent-toast-progress {
  width: 100%;
  animation: agent-toast-progress-shrink linear forwards;
}

@keyframes agent-toast-progress-shrink {
  from { width: 100%; }
  to { width: 0%; }
}

@media (prefers-reduced-motion: reduce) {
  .agent-toast-enter-active,
  .agent-toast-leave-active { transition: opacity 0.01ms linear; }
  .agent-toast-enter-from,
  .agent-toast-leave-to { transform: none; }
  .agent-toast-progress { animation: none; }
}
</style>
