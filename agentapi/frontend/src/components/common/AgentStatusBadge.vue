<template>
  <span class="inline-flex items-center gap-1.5" :data-status="normalizedStatus">
    <span class="inline-block h-2 w-2 shrink-0 rounded-full" :class="variantClass"></span>
    <span class="text-sm text-gray-700 dark:text-gray-300">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
}>()

const normalizedStatus = computed(() => props.status.trim().toLowerCase())

const variantClass = computed(() => {
  switch (normalizedStatus.value) {
    case 'active':
    case 'success':
    case 'completed':
    case 'complete':
    case 'done':
    case 'succeeded':
    case 'confirmed':
    case 'paid':
    case 'ready':
    case 'released':
    case 'used':
    case 'available':
    case 'healthy':
    case 'ok':
    case 'operational':
    case 'verified':
    case 'measured':
    case 'bound':
      return 'bg-green-500'
    case 'disabled':
    case 'inactive':
    case 'archived':
    case 'warning':
    case 'degraded':
    case 'pending':
    case 'queued':
    case 'waiting':
    case 'prepared':
    case 'processing':
    case 'in_progress':
    case 'provisioning':
    case 'partial':
    case 'refund_requested':
    case 'refund_pending':
    case 'refunding':
    case 'partially_refunded':
    case 'suspended':
    case 'unobserved':
      return 'bg-yellow-500'
    case 'error':
    case 'danger':
    case 'failed':
    case 'refund_failed':
    case 'mismatch':
    case 'cancelled':
    case 'canceled':
    case 'expired':
    case 'reversed':
    case 'revoked':
    case 'unavailable':
    case 'rejected':
      return 'bg-red-500'
    default:
      return 'bg-gray-400'
  }
})
</script>
