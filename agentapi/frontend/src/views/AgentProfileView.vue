<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'

const auth = useAgentSession()
const router = useRouter()

async function logout(): Promise<void> {
  await auth.logout()
  await router.replace('/home')
}
</script>

<template>
  <main class="mx-auto max-w-3xl space-y-6 p-6">
    <header><p class="text-sm text-slate-500">AgentAPI</p><h1 class="text-2xl font-semibold text-slate-900 dark:text-white">Profile</h1></header>
    <section class="rounded-xl border bg-white p-6 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <dl class="space-y-4 text-sm"><div><dt class="text-slate-500">Email</dt><dd class="mt-1 font-medium">{{ auth.user?.email || '—' }}</dd></div><div><dt class="text-slate-500">Main user id</dt><dd class="mt-1 font-mono">{{ auth.user?.id || '—' }}</dd></div></dl>
      <button class="mt-8 rounded-lg border border-red-200 px-4 py-2 text-sm text-red-600" type="button" @click="logout">Sign out</button>
    </section>
  </main>
</template>
