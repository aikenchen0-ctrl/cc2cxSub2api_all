<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

const route = useRoute()
const router = useRouter()
const session = useAgentSession()
const email = ref('')
const password = ref('')
const totp = ref('')
const tempToken = ref('')
const error = ref('')

async function submit(): Promise<void> {
  error.value = ''
  try {
    if (tempToken.value) {
      await session.login2FA(tempToken.value, totp.value.trim())
    } else {
      const response = await session.login(email.value.trim(), password.value)
      if (response.requires_2fa) {
        tempToken.value = response.temp_token || ''
        return
      }
    }
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (err) {
    error.value = errorMessage(err, 'Sign in failed.')
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-[calc(100vh-5rem)] max-w-md items-center px-4 py-10">
    <section class="w-full rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
      <h1 class="text-2xl font-semibold">{{ tempToken ? 'Two-factor verification' : 'Sign in' }}</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">Your browser receives only an AgentAPI HttpOnly session. Main-site credentials stay server-side.</p>
      <p v-if="error" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <form class="mt-6 space-y-4" @submit.prevent="submit">
        <template v-if="!tempToken">
          <label class="block text-sm font-medium">Email<input v-model="email" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="email" autocomplete="email" required></label>
          <label class="block text-sm font-medium">Password<input v-model="password" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="current-password" required></label>
        </template>
        <label v-else class="block text-sm font-medium">Authenticator code<input v-model="totp" class="mt-1 w-full rounded-lg border px-3 py-2 tracking-[0.4em] dark:border-dark-600 dark:bg-dark-950" inputmode="numeric" autocomplete="one-time-code" minlength="6" maxlength="8" required></label>
        <button class="w-full rounded-lg bg-primary-600 px-4 py-2.5 font-medium text-white disabled:opacity-50" type="submit" :disabled="session.busy">{{ session.busy ? 'Signing in…' : tempToken ? 'Verify' : 'Sign in' }}</button>
      </form>
      <p class="mt-6 text-center text-sm text-gray-500">No account? <RouterLink class="font-medium text-primary-600" to="/register">Create one</RouterLink></p>
    </section>
  </main>
</template>

