<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentAPIKeyView } from '@/agent/api'

const keys = ref<AgentAPIKeyView[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const name = ref('')
const oneTimeKey = ref('')

function messageOf(errorValue: unknown, fallback: string): string {
  if (typeof errorValue === 'object' && errorValue !== null && 'message' in errorValue) {
    const message = (errorValue as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return fallback
}

function formatDate(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const response = await agentAPI.keys.list()
    keys.value = response.items
  } catch (err) {
    error.value = messageOf(err, 'Unable to load AgentAPI keys.')
  } finally {
    loading.value = false
  }
}

async function create(): Promise<void> {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await agentAPI.keys.create(name.value.trim() || 'AgentAPI key')
    oneTimeKey.value = response.key
    keys.value = [response.item, ...keys.value]
    name.value = ''
    notice.value = 'The full key is shown once. Copy it before closing this notice.'
  } catch (err) {
    error.value = messageOf(err, 'Unable to create AgentAPI key.')
  } finally {
    saving.value = false
  }
}

async function revoke(key: AgentAPIKeyView): Promise<void> {
  if (key.status !== 'active' || !window.confirm(`Revoke ${key.name || key.prefix}?`)) return
  error.value = ''
  try {
    await agentAPI.keys.revoke(key.id)
    key.status = 'revoked'
    if (oneTimeKey.value && oneTimeKey.value.startsWith(key.prefix)) oneTimeKey.value = ''
  } catch (err) {
    error.value = messageOf(err, 'Unable to revoke AgentAPI key.')
  }
}

async function copyKey(): Promise<void> {
  if (!oneTimeKey.value) return
  try {
    await navigator.clipboard.writeText(oneTimeKey.value)
    notice.value = 'Copied to clipboard. Treat this key like a password.'
  } catch {
    notice.value = 'Clipboard access was blocked; select and copy the key manually.'
  }
}

onMounted(load)
</script>

<template>
  <main class="mx-auto max-w-5xl space-y-6 p-6">
    <header>
      <p class="text-sm text-slate-500">AgentAPI</p>
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">API keys</h1>
      <p class="mt-1 text-sm text-slate-500">Use an AgentAPI key in the <code>Authorization: Bearer</code> header when calling this agent gateway.</p>
    </header>

    <section class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
      AgentAPI keys are local gateway credentials. The main-site administrator key and main-site JWT are never shown here. A newly created key is stored as a hash and its full value is displayed only once.
    </section>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>

    <section v-if="oneTimeKey" class="space-y-3 rounded-xl border border-blue-200 bg-blue-50 p-5 dark:border-blue-900/60 dark:bg-blue-950/30">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="font-semibold text-blue-900 dark:text-blue-100">New key — copy it now</h2>
        <button class="rounded-lg bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700" type="button" @click="copyKey">Copy key</button>
      </div>
      <code class="block select-all break-all rounded-lg bg-white p-3 text-sm text-slate-900 dark:bg-slate-950 dark:text-slate-100">{{ oneTimeKey }}</code>
    </section>

    <section class="rounded-xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <h2 class="font-semibold text-slate-900 dark:text-white">Create a key</h2>
      <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="create">
        <input v-model="name" class="min-w-0 flex-1 rounded-lg border px-3 py-2 dark:border-slate-700 dark:bg-slate-950" maxlength="100" placeholder="Key name, e.g. Desktop client" type="text">
        <button class="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-white dark:text-slate-900" :disabled="saving" type="submit">{{ saving ? 'Creating…' : 'Create key' }}</button>
      </form>
    </section>

    <section class="overflow-hidden rounded-xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="border-b p-5 dark:border-slate-700"><h2 class="font-semibold text-slate-900 dark:text-white">Your keys</h2></div>
      <div v-if="loading" class="p-5 text-sm text-slate-500">Loading…</div>
      <div v-else-if="keys.length === 0" class="p-5 text-sm text-slate-500">No AgentAPI keys yet.</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">Name</th><th class="px-5 py-3">Prefix</th><th class="px-5 py-3">Created</th><th class="px-5 py-3">Last used</th><th class="px-5 py-3">Status</th><th class="px-5 py-3"></th></tr></thead>
          <tbody>
            <tr v-for="key in keys" :key="key.id" class="border-t dark:border-slate-700">
              <td class="px-5 py-4 font-medium text-slate-900 dark:text-white">{{ key.name || '—' }}</td>
              <td class="px-5 py-4 font-mono text-xs text-slate-500">{{ key.prefix }}…</td>
              <td class="px-5 py-4 text-slate-500">{{ formatDate(key.created_at) }}</td>
              <td class="px-5 py-4 text-slate-500">{{ formatDate(key.last_used_at) }}</td>
              <td class="px-5 py-4"><span :class="key.status === 'active' ? 'text-emerald-600' : 'text-slate-400'">{{ key.status }}</span></td>
              <td class="px-5 py-4 text-right"><button v-if="key.status === 'active'" class="text-sm text-red-600 hover:underline" type="button" @click="revoke(key)">Revoke</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </main>
</template>
