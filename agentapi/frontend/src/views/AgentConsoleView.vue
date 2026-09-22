<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse, type AgentUserView, type AuditEventView, type RechargeOrder, type SettlementView } from '@/agent/api'

const context = ref<AgentContextResponse | null>(null)
const users = ref<AgentUserView[]>([])
const loading = ref(true)
const error = ref('')
const allocation = ref<Record<string, string>>({})
const notice = ref('')
const syncing = ref(false)
const reconciling = ref(false)
const settlements = ref<SettlementView[]>([])
const auditEvents = ref<AuditEventView[]>([])
const rechargeOrders = ref<RechargeOrder[]>([])
const rechargeEnabled = ref(false)
const allocatingOrder = ref('')
const brandingName = ref('')
const brandingSiteName = ref('')
const brandingLogo = ref('')
const brandingSaving = ref(false)

function money(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [agentContext, list, pending, recharge, audit] = await Promise.all([
      agentAPI.getContext(), agentAPI.getUsers(), agentAPI.getSettlements(), agentAPI.getAdminRechargeOrders(), agentAPI.getAuditEvents(),
    ])
    context.value = agentContext
    brandingName.value = agentContext.agent.name
    brandingSiteName.value = agentContext.agent.site_name
    brandingLogo.value = agentContext.agent.site_logo || ''
    users.value = list.items
    settlements.value = pending.items
    rechargeOrders.value = recharge.items
    rechargeEnabled.value = recharge.enabled
    auditEvents.value = audit.items
  } catch (err) {
    error.value = (err as { message?: string }).message || 'Failed to load agent data.'
  } finally {
    loading.value = false
  }
}

async function reconcile(): Promise<void> {
  reconciling.value = true
  notice.value = ''
  try {
    const result = await agentAPI.reconcileSettlements()
    settlements.value = result.items
    notice.value = result.total ? `Reconciled ${result.total} settlement record(s).` : 'No pending settlements were found.'
    await load()
  } catch (err) {
    notice.value = (err as { message?: string }).message || 'Settlement reconciliation failed.'
  } finally {
    reconciling.value = false
  }
}

async function allocate(user: AgentUserView): Promise<void> {
  const amount = Number(allocation.value[user.main_user_id])
  if (!Number.isFinite(amount) || amount <= 0) {
    notice.value = 'Enter a positive amount.'
    return
  }
  try {
    const updated = await agentAPI.allocate(user.main_user_id, amount, 'agent console allocation')
    const index = users.value.findIndex((item) => item.main_user_id === user.main_user_id)
    if (index >= 0) users.value[index] = updated
    notice.value = `Allocated ${amount.toFixed(2)} to ${user.email || user.main_user_id}.`
    allocation.value[user.main_user_id] = ''
    await load()
  } catch (err) {
    notice.value = (err as { message?: string }).message || 'Allocation failed.'
  }
}

async function syncOwnerBalance(): Promise<void> {
  syncing.value = true
  notice.value = ''
  try {
    const agent = await agentAPI.syncAdminWallet()
    if (context.value) context.value.agent = agent
    notice.value = 'Main-site owner balance synchronized.'
    await load()
  } catch (err) {
    notice.value = (err as { message?: string }).message || 'Balance synchronization failed.'
  } finally {
    syncing.value = false
  }
}

function idempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return `recharge-allocate-${crypto.randomUUID()}`
  return `recharge-allocate-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

async function allocateRechargeOrder(order: RechargeOrder): Promise<void> {
  if (order.status !== 'paid_pending_allocation' || allocatingOrder.value) return
  allocatingOrder.value = order.order_no
  notice.value = ''
  try {
    const result = await agentAPI.allocateRechargeOrder(order.order_no, idempotencyKey())
    notice.value = result.allocated
      ? `Recharge order ${order.order_no} was allocated to the mapped user.`
      : `Recharge order ${order.order_no} is still waiting for owner credit.`
    await load()
  } catch (err) {
    notice.value = (err as { message?: string }).message || 'Recharge allocation failed.'
  } finally {
    allocatingOrder.value = ''
  }
}

async function saveBranding(): Promise<void> {
  brandingSaving.value = true
  notice.value = ''
  try {
    const updated = await agentAPI.updateBranding({
      name: brandingName.value.trim(),
      site_name: brandingSiteName.value.trim(),
      site_logo: brandingLogo.value.trim(),
    })
    if (context.value) {
      context.value.agent.name = updated.name
      context.value.agent.site_name = updated.site_name
      context.value.agent.site_logo = updated.site_logo || undefined
    }
    brandingName.value = updated.name
    brandingSiteName.value = updated.site_name
    brandingLogo.value = updated.site_logo || ''
    notice.value = 'Branding saved. New pages will use the updated name and logo.'
  } catch (err) {
    notice.value = (err as { message?: string }).message || 'Branding update failed.'
  } finally {
    brandingSaving.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">Agent Console</h1>
        <p class="mt-1 text-sm text-slate-500">Manage mapped users and allocate prepaid usage credit.</p>
      </div>
      <div class="flex flex-wrap gap-2"><button class="rounded-lg border px-4 py-2 text-sm" type="button" @click="load">Refresh</button><button class="rounded-lg border px-4 py-2 text-sm disabled:opacity-50" type="button" :disabled="reconciling" @click="reconcile">{{ reconciling ? 'Reconciling…' : 'Reconcile pending' }}</button><button class="rounded-lg bg-slate-900 px-4 py-2 text-sm text-white disabled:opacity-50 dark:bg-white dark:text-slate-900" type="button" :disabled="syncing" @click="syncOwnerBalance">{{ syncing ? 'Syncing…' : 'Sync owner balance' }}</button></div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700">{{ notice }}</p>
    <p v-if="loading" class="text-sm text-slate-500">Loading…</p>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="font-semibold">Branding</h2>
          <p class="mt-1 text-sm text-slate-500">Customize the agent display name and logo. Billing identity and domain are not editable here.</p>
        </div>
        <button class="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50" type="button" :disabled="brandingSaving" @click="saveBranding">{{ brandingSaving ? 'Saving…' : 'Save branding' }}</button>
      </div>
      <div class="mt-4 grid gap-4 md:grid-cols-3">
        <label class="text-sm font-medium">Agent name<input v-model="brandingName" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="100" type="text"></label>
        <label class="text-sm font-medium">Site name<input v-model="brandingSiteName" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="160" type="text"></label>
        <label class="text-sm font-medium">Logo URL or path<input v-model="brandingLogo" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="2048" placeholder="/logo.svg or https://…" type="text"></label>
      </div>
    </section>

    <section v-if="context" class="grid gap-4 md:grid-cols-3">
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Agent</p>
        <p class="mt-2 text-lg font-semibold">{{ context.agent.name }}</p>
        <p class="text-sm text-slate-500">{{ context.agent.domain || 'domain pending' }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Available credit</p>
        <p class="mt-2 text-2xl font-semibold">{{ money(context.agent.wallet_available_cents) }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Allocated credit</p>
        <p class="mt-2 text-2xl font-semibold">{{ money(context.agent.wallet_allocated_cents) }}</p>
      </div>
      <div class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
        <p class="text-sm text-slate-500">Main owner balance</p>
        <p class="mt-2 text-2xl font-semibold">{{ money(context.agent.main_balance_cents) }}</p>
        <p class="text-xs text-slate-500">{{ context.agent.billing_status }} · {{ context.agent.main_balance_checked_at || 'not synchronized' }}</p>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">Recharge orders</h2>
        <p class="mt-1 text-sm text-slate-500">Paid orders are allocated only after the owner balance has been synchronized. Payment is {{ rechargeEnabled ? 'enabled' : 'disabled' }} for this instance.</p>
      </div>
      <div v-if="rechargeOrders.length === 0" class="p-5 text-sm text-slate-500">No recharge orders.</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">Order</th><th class="px-5 py-3">User</th><th class="px-5 py-3">Amount</th><th class="px-5 py-3">Status</th><th class="px-5 py-3"></th></tr></thead>
          <tbody>
            <tr v-for="order in rechargeOrders" :key="order.order_no" class="border-t dark:border-slate-700">
              <td class="px-5 py-4 font-mono text-xs">{{ order.order_no }}</td>
              <td class="px-5 py-4 font-mono text-xs">{{ order.main_user_id }}</td>
              <td class="px-5 py-4">{{ money(order.amount_cents) }} {{ order.currency }}</td>
              <td class="px-5 py-4">{{ order.status }}</td>
              <td class="px-5 py-4 text-right"><button v-if="order.status === 'paid_pending_allocation'" class="rounded bg-blue-600 px-3 py-1 text-white disabled:opacity-50" type="button" :disabled="allocatingOrder !== ''" @click="allocateRechargeOrder(order)">{{ allocatingOrder === order.order_no ? 'Allocating…' : 'Allocate' }}</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">Agent users</h2>
        <p class="mt-1 text-sm text-slate-500">Only users mapped to this agent are shown.</p>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800">
            <tr><th class="px-5 py-3">User</th><th class="px-5 py-3">Balance</th><th class="px-5 py-3">Allocate</th></tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.main_user_id" class="border-t dark:border-slate-700">
              <td class="px-5 py-4"><div class="font-medium">{{ user.display_name || user.email || user.main_user_id }}</div><div class="text-xs text-slate-500">{{ user.main_user_id }}</div></td>
              <td class="px-5 py-4">{{ money(user.balance_cents) }}</td>
              <td class="px-5 py-4"><div class="flex gap-2"><input v-model="allocation[user.main_user_id]" class="w-28 rounded border px-2 py-1" min="0" step="0.01" type="number" placeholder="amount"><button class="rounded bg-blue-600 px-3 py-1 text-white disabled:opacity-50" type="button" :disabled="user.status !== 'active'" @click="allocate(user)">Allocate</button></div></td>
            </tr>
            <tr v-if="!loading && users.length === 0"><td class="px-5 py-8 text-center text-slate-500" colspan="3">No mapped users.</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">Pending settlements</h2>
        <p class="mt-1 text-sm text-slate-500">A pending record is never silently refunded; reconcile it against the main-site usage ledger.</p>
      </div>
      <div v-if="settlements.length === 0" class="p-5 text-sm text-slate-500">No pending settlements.</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">Request</th><th class="px-5 py-3">Reserved</th><th class="px-5 py-3">Status</th><th class="px-5 py-3">Reason</th></tr></thead>
          <tbody><tr v-for="item in settlements" :key="item.request_id" class="border-t dark:border-slate-700"><td class="max-w-xs truncate px-5 py-4 font-mono text-xs">{{ item.request_id }}</td><td class="px-5 py-4">{{ money(item.reserved_cents) }}</td><td class="px-5 py-4 text-amber-600">{{ item.status }}</td><td class="max-w-md px-5 py-4 text-slate-500">{{ item.error || 'Awaiting main-site usage' }}</td></tr></tbody>
        </table>
      </div>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700">
        <h2 class="font-semibold">Audit events</h2>
        <p class="mt-1 text-sm text-slate-500">Recent security-sensitive changes for this Agent. Secrets and authorization headers are never stored.</p>
      </div>
      <div v-if="auditEvents.length === 0" class="p-5 text-sm text-slate-500">No audit events.</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">Time</th><th class="px-5 py-3">Operation</th><th class="px-5 py-3">Actor</th><th class="px-5 py-3">Target</th><th class="px-5 py-3">Request</th></tr></thead>
          <tbody><tr v-for="event in auditEvents" :key="event.id" class="border-t dark:border-slate-700"><td class="whitespace-nowrap px-5 py-4">{{ event.created_at }}</td><td class="px-5 py-4 font-medium">{{ event.operation }}</td><td class="px-5 py-4 font-mono text-xs">{{ event.actor_type }}:{{ event.actor_id }}</td><td class="px-5 py-4 font-mono text-xs">{{ event.target_type }}<span v-if="event.target_id">:{{ event.target_id }}</span></td><td class="max-w-xs truncate px-5 py-4 font-mono text-xs">{{ event.request_id }}</td></tr></tbody>
        </table>
      </div>
    </section>
  </main>
</template>
