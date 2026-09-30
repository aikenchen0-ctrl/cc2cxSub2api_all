<script setup lang="ts">
import { useAgentNavigationLoadingState } from '@/composables/useAgentNavigationLoading'

const { isLoading } = useAgentNavigationLoadingState()
</script>

<template>
  <Transition name="progress-fade">
    <div
      v-show="isLoading"
      class="navigation-progress"
      role="progressbar"
      aria-label="页面加载中"
      aria-valuetext="正在加载下一页面"
      :aria-hidden="isLoading ? 'false' : 'true'"
      :data-loading="isLoading ? 'true' : 'false'"
    >
      <div class="navigation-progress-bar" />
    </div>
  </Transition>
</template>

<style scoped>
.navigation-progress {
  position: fixed;
  inset: 0 0 auto;
  z-index: 9999;
  height: 3px;
  overflow: hidden;
  background: transparent;
}

.navigation-progress-bar {
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, transparent 0%, #60a5fa 20%, #2563eb 50%, #60a5fa 80%, transparent 100%);
  animation: agent-progress-slide 1.5s ease-in-out infinite;
}

:global(.dark) .navigation-progress-bar {
  background: linear-gradient(90deg, transparent 0%, #2563eb 20%, #60a5fa 50%, #2563eb 80%, transparent 100%);
}

@keyframes agent-progress-slide {
  from { transform: translateX(-100%); }
  to { transform: translateX(100%); }
}

.progress-fade-enter-active { transition: opacity 0.15s ease-out; }
.progress-fade-leave-active { transition: opacity 0.3s ease-out; }
.progress-fade-enter-from,
.progress-fade-leave-to { opacity: 0; }

@media (prefers-reduced-motion: reduce) {
  .navigation-progress-bar {
    animation: agent-progress-pulse 2s ease-in-out infinite;
  }

  @keyframes agent-progress-pulse {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 1; }
  }
}
</style>
