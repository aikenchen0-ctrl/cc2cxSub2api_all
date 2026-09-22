<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentUsageView as UsageItem } from '@/agent/api'

const items = ref<UsageItem[]>([])
const loading = ref(true)
const error = ref('')

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = (await agentAPI.getUsage()).items
  } catch (err) {
    error.value = (err as { message?: string }).message || 'Unable to load usage records.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">Usage</h1>
        <p class="mt-1 text-sm text-slate-500">Requests are shown with their local reservation and upstream settlement status.</p>
      </div>
      <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">Refresh</button>
    </header>
    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <div class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div v-if="loading" class="p-5 text-sm text-slate-500">Loading…</div>
      <div v-else-if="items.length === 0" class="p-5 text-sm text-slate-500">No usage records yet.</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">Request</th><th class="px-5 py-3">Reserved</th><th class="px-5 py-3">Upstream charge</th><th class="px-5 py-3">Status</th><th class="px-5 py-3">Created</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.request_id" class="border-t dark:border-slate-700">
              <td class="max-w-xs truncate px-5 py-4 font-mono text-xs">{{ item.request_id }}</td>
              <td class="px-5 py-4">{{ money(item.reserved_cents) }}</td>
              <td class="px-5 py-4">{{ item.actual_cents ? money(item.actual_cents) : 'pending' }}</td>
              <td class="px-5 py-4"><span :class="item.settlement_status === 'confirmed' ? 'text-emerald-600' : item.settlement_status === 'pending' ? 'text-amber-600' : 'text-slate-500'">{{ item.settlement_status }}</span></td>
              <td class="px-5 py-4 text-slate-500">{{ new Date(item.created_at).toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </main>
</template>
