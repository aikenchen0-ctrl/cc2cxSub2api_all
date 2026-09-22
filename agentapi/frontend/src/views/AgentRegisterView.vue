<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { errorMessage } from '@/agent/client'

const router = useRouter()
const session = useAgentSession()
const email = ref('')
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')

async function submit(): Promise<void> {
  error.value = ''
  if (password.value.length < 6) { error.value = 'Password must contain at least 6 characters.'; return }
  if (password.value !== confirmPassword.value) { error.value = 'Passwords do not match.'; return }
  try {
    await session.register(email.value.trim(), password.value, username.value.trim() || undefined)
    await router.replace('/dashboard')
  } catch (err) {
    error.value = errorMessage(err, 'Registration failed.')
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-[calc(100vh-5rem)] max-w-md items-center px-4 py-10">
    <section class="w-full rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
      <h1 class="text-2xl font-semibold">Create an account</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">The account is created on the main Sub2API site and mapped to this AgentAPI instance.</p>
      <p v-if="error" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <form class="mt-6 space-y-4" @submit.prevent="submit">
        <label class="block text-sm font-medium">Email<input v-model="email" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="email" autocomplete="email" required></label>
        <label class="block text-sm font-medium">Display name <span class="font-normal text-gray-400">(optional)</span><input v-model="username" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="text" maxlength="100" autocomplete="nickname"></label>
        <label class="block text-sm font-medium">Password<input v-model="password" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="new-password" required></label>
        <label class="block text-sm font-medium">Confirm password<input v-model="confirmPassword" class="mt-1 w-full rounded-lg border px-3 py-2 dark:border-dark-600 dark:bg-dark-950" type="password" autocomplete="new-password" required></label>
        <button class="w-full rounded-lg bg-primary-600 px-4 py-2.5 font-medium text-white disabled:opacity-50" type="submit" :disabled="session.busy">{{ session.busy ? 'Creating…' : 'Create account' }}</button>
      </form>
      <p class="mt-6 text-center text-sm text-gray-500">Already registered? <RouterLink class="font-medium text-primary-600" to="/login">Sign in</RouterLink></p>
    </section>
  </main>
</template>

