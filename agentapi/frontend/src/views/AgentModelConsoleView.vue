<script setup lang="ts">
import { computed, ref } from 'vue'
import { agentAPI, type ChatCompletionResponse } from '@/agent/api'
import { errorMessage } from '@/agent/client'

type Message = { role: 'user' | 'assistant'; content: string }

const models = ['gpt-5.5', 'gpt-5.4-mini', 'gpt-5.6-' + 'luna', 'deepseek-chat']
const model = ref(models[0])
const prompt = ref('')
const messages = ref<Message[]>([])
const loading = ref(false)
const error = ref('')

const canSubmit = computed(() => prompt.value.trim().length > 0 && !loading.value)

function responseText(response: ChatCompletionResponse): string {
  const choice = response.choices?.[0]
  if (choice?.message?.content) return choice.message.content
  if (choice?.text) return choice.text
  if (response.output_text) return response.output_text
  const output = response.output?.flatMap((item) => item.content || []).map((item) => item.text || '').filter(Boolean).join('\n')
  if (output) return output
  return JSON.stringify(response, null, 2)
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return
  const content = prompt.value.trim()
  prompt.value = ''
  error.value = ''
  messages.value.push({ role: 'user', content })
  loading.value = true
  try {
    // Pass a snapshot to the request. The response is appended to the
    // conversation immediately afterwards; handing the reactive array to an
    // adapter would otherwise mutate the request payload after it was sent.
    const response = await agentAPI.model.chat(model.value, messages.value.map((message) => ({ ...message })))
    messages.value.push({ role: 'assistant', content: responseText(response) })
  } catch (err) {
    error.value = errorMessage(err, 'The model request failed.')
    prompt.value = content
  } finally {
    loading.value = false
  }
}

function clearConversation(): void {
  messages.value = []
  error.value = ''
}
</script>

<template>
  <main class="mx-auto flex max-w-5xl flex-col gap-6 p-6">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">Model console</h1>
        <p class="mt-1 text-sm text-slate-500">Requests use your AgentAPI session; no main-site key is placed in the browser.</p>
      </div>
      <div class="flex items-center gap-2">
        <label class="text-sm text-slate-500" for="agent-model">Model</label>
        <select id="agent-model" v-model="model" class="rounded-lg border px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900">
          <option v-for="item in models" :key="item" :value="item">{{ item }}</option>
        </select>
        <button class="rounded-lg border px-3 py-2 text-sm" type="button" @click="clearConversation">Clear</button>
      </div>
    </header>

    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>

    <section class="min-h-[28rem] rounded-2xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div v-if="messages.length === 0" class="flex h-[25rem] items-center justify-center text-center text-sm text-slate-500">
        <div><p class="text-lg font-medium text-slate-700 dark:text-slate-200">Try a model request</p><p class="mt-2">The selected public model is sent to the main Sub2API gateway through AgentAPI.</p></div>
      </div>
      <div v-else class="space-y-5">
        <article v-for="(message, index) in messages" :key="`${message.role}-${index}`" :class="message.role === 'user' ? 'ml-auto max-w-3xl bg-blue-50 dark:bg-blue-950/30' : 'mr-auto max-w-4xl bg-slate-50 dark:bg-slate-800'" class="rounded-xl p-4">
          <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{{ message.role }}</p>
          <p class="whitespace-pre-wrap text-sm leading-7 text-slate-800 dark:text-slate-100">{{ message.content }}</p>
        </article>
        <p v-if="loading" class="text-sm text-slate-500">Waiting for the upstream response…</p>
      </div>
    </section>

    <form class="rounded-2xl border bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-900" @submit.prevent="submit">
      <label class="sr-only" for="agent-prompt">Message</label>
      <textarea id="agent-prompt" v-model="prompt" class="min-h-28 w-full resize-y rounded-xl border px-4 py-3 text-sm outline-none focus:border-blue-500 dark:border-slate-700 dark:bg-slate-950" :disabled="loading" placeholder="Ask the model something…" @keydown.ctrl.enter.prevent="submit"></textarea>
      <div class="mt-3 flex items-center justify-between gap-3"><p class="text-xs text-slate-500">Ctrl + Enter to send · Charges follow the configured AgentAPI wallet policy.</p><button class="rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" type="submit" :disabled="!canSubmit">{{ loading ? 'Sending…' : 'Send' }}</button></div>
    </form>
  </main>
</template>
