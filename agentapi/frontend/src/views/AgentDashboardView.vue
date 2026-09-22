<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { agentAPI, type AgentContextResponse, type AgentUserView } from '@/agent/api'

const context = ref<AgentContextResponse | null>(null)
const wallet = ref<AgentUserView | null>(null)
const loading = ref(true)
const error = ref('')

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [nextContext, nextWallet] = await Promise.all([agentAPI.getContext(), agentAPI.getWallet()])
    context.value = nextContext
    wallet.value = nextWallet.user
  } catch (err) {
    error.value = (err as { message?: string }).message || 'Unable to load the AgentAPI dashboard.'
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
        <p class="text-sm text-slate-500">{{ context?.agent.site_name || 'AgentAPI' }}</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">Dashboard</h1>
        <p class="mt-1 text-sm text-slate-500">Your AgentAPI balance is a local sub-balance. Model calls are charged to the agent owner on the main site.</p>
      </div>
      <div class="flex gap-2">
        <RouterLink to="/recharge" class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white">Recharge</RouterLink>
        <button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">Refresh</button>
      </div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="loading" class="text-sm text-slate-500">Loading…</p>

    <section v-if="context && wallet" class="grid gap-4 md:grid-cols-3">
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Your available balance</p>
        <p class="mt-2 text-2xl font-semibold">{{ money(wallet.balance_cents) }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Agent</p>
        <p class="mt-2 text-lg font-semibold">{{ context.agent.name }}</p>
        <p class="text-sm text-slate-500">{{ context.agent.domain || 'domain pending' }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Billing status</p>
        <p class="mt-2 text-lg font-semibold">{{ context.agent.billing_mode === 'owner_upstream' ? 'Owner upstream' : context.agent.billing_mode }}</p>
        <p class="text-sm text-slate-500">No commission or independent upstream account.</p>
      </div>
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold">How billing works</h2>
      <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">
        Your account is a real account on the main Sub2API. AgentAPI keeps the mapping and your sub-balance, while the main site remains the authoritative owner balance and usage ledger. If an upstream charge cannot be confirmed immediately, the request stays pending instead of being silently refunded.
      </p>
    </section>
  </main>
</template>
