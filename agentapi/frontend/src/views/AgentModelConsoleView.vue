<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { agentAPI, type AgentTaskHistoryItem, type ChatCompletionResponse, type ImageGenerationResponse, type ImageTaskResponse, type VideoTaskResponse } from '@/agent/api'
import { errorMessage } from '@/agent/client'
import { statusLabel, taskTypeLabel } from '@/agent/locale'
import AgentPagination from '@/components/AgentPagination.vue'

type ConsoleMode = 'text' | 'image' | 'video'
type Message = { role: 'user' | 'assistant'; content: string }
type ImageResult = { url: string; revisedPrompt?: string }

const models = ref<string[]>([])
const mode = ref<ConsoleMode>('text')
const model = ref('')
const prompt = ref('')
const messages = ref<Message[]>([])
const loading = ref(false)
const modelsLoading = ref(true)
const error = ref('')
const modelsError = ref('')
const imageFile = ref<File | null>(null)
const imageInputKey = ref(0)
const imageResults = ref<ImageResult[]>([])
const asyncImage = ref(false)
const imageTaskID = ref('')
const imageStatus = ref('')
const imageResponse = ref<ImageTaskResponse | null>(null)
const videoTaskID = ref('')
const videoStatus = ref('')
const videoURL = ref('')
const videoResponse = ref<VideoTaskResponse | null>(null)
const mediaNotice = ref('')
const taskHistory = ref<AgentTaskHistoryItem[]>([])
const taskHistoryTotal = ref(0)
const taskHistoryPage = ref(1)
const taskHistoryPageSize = ref(25)
const taskHistoryLoading = ref(true)
const taskHistoryError = ref('')

const modelsByMode = computed(() => models.value.filter((name) => modelMode(name) === mode.value))
const canSubmit = computed(() => prompt.value.trim().length > 0 && Boolean(model.value) && !loading.value && !modelsLoading.value)

const VIDEO_POLL_INTERVAL_MS = 3_000
const MAX_VIDEO_POLL_ATTEMPTS = 120
let disposed = false
let pollTimer: ReturnType<typeof setTimeout> | undefined
let resolvePollTimer: (() => void) | undefined
let taskHistorySequence = 0

function modelMode(name: string): ConsoleMode {
  const normalized = name.trim().toLowerCase()
  if (normalized.startsWith('grok-imagine-video-') || normalized.startsWith('seedance-') || normalized.startsWith('kling-')) return 'video'
  if (normalized.startsWith('gpt-image-') || normalized === 'gemini-3.1-flash-image' || normalized.startsWith('grok-imagine-image') || normalized === 'grok-imagine') return 'image'
  return 'text'
}

watch(mode, () => {
  model.value = modelsByMode.value[0] || ''
  error.value = ''
})

async function loadModels(): Promise<void> {
  modelsLoading.value = true
  modelsError.value = ''
  try {
    models.value = await agentAPI.model.list()
    if (!models.value.includes(model.value)) model.value = modelsByMode.value[0] || ''
    if (models.value.length === 0) modelsError.value = '当前代理站尚未启用任何模型。'
  } catch (err) {
    modelsError.value = errorMessage(err, '加载可用模型失败，请稍后刷新重试。')
  } finally {
    modelsLoading.value = false
  }
}

onMounted(() => {
  void loadModels()
  void loadTaskHistory()
})
onBeforeUnmount(() => {
  disposed = true
  taskHistorySequence += 1
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = undefined
    resolvePollTimer?.()
    resolvePollTimer = undefined
  }
})

async function loadTaskHistory(): Promise<void> {
  const sequence = ++taskHistorySequence
  taskHistoryLoading.value = true
  taskHistoryError.value = ''
  try {
    const result = await agentAPI.getTasks(taskHistoryPage.value, taskHistoryPageSize.value)
    if (sequence !== taskHistorySequence) return
    taskHistory.value = result.items
    taskHistoryTotal.value = result.total
    const lastPage = Math.max(1, Math.ceil(result.total / taskHistoryPageSize.value))
    if (taskHistoryPage.value > lastPage) {
      taskHistoryPage.value = lastPage
      await loadTaskHistory()
      return
    }
  } catch (err) {
    if (sequence === taskHistorySequence) {
      taskHistoryError.value = errorMessage(err, '加载最近任务记录失败，请稍后刷新重试。')
    }
  } finally {
    if (sequence === taskHistorySequence) taskHistoryLoading.value = false
  }
}

async function changeTaskHistoryPage(nextPage: number): Promise<void> {
  if (nextPage < 1 || nextPage > Math.max(1, Math.ceil(taskHistoryTotal.value / taskHistoryPageSize.value)) || nextPage === taskHistoryPage.value) return
  taskHistoryPage.value = nextPage
  await loadTaskHistory()
}

async function changeTaskHistoryPageSize(nextPageSize: number): Promise<void> {
  if (nextPageSize === taskHistoryPageSize.value) return
  taskHistoryPageSize.value = nextPageSize
  taskHistoryPage.value = 1
  await loadTaskHistory()
}

async function resumeTask(task: AgentTaskHistoryItem): Promise<void> {
  if (loading.value) return
  error.value = ''
  mediaNotice.value = ''
  mode.value = task.task_type
  await nextTick()
  if (task.model && modelsByMode.value.includes(task.model)) model.value = task.model
  if (task.task_type === 'image') {
    imageTaskID.value = task.task_id
    imageStatus.value = task.status
    imageResults.value = []
    imageResponse.value = null
    await checkImageStatus()
  } else {
    videoTaskID.value = task.task_id
    videoStatus.value = task.status
    videoURL.value = ''
    videoResponse.value = null
    await checkVideoStatus()
  }
}

function responseText(response: ChatCompletionResponse): string {
  const choice = response.choices?.[0]
  if (choice?.message?.content) return choice.message.content
  if (choice?.text) return choice.text
  if (response.output_text) return response.output_text
  const output = response.output?.flatMap((item) => item.content || []).map((item) => item.text || '').filter(Boolean).join('\n')
  if (output) return output
  return JSON.stringify(response, null, 2)
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function safeMediaURL(value: unknown, kind: 'image' | 'video'): string {
  if (typeof value !== 'string') return ''
  const url = value.trim()
  if (/^https?:\/\//i.test(url)) return url
  if (kind === 'image' && /^data:image\/(png|jpeg|jpg|webp|gif);base64,/i.test(url)) return url
  if (kind === 'video' && /^data:video\/(mp4|webm|quicktime);base64,/i.test(url)) return url
  return ''
}

function imageResultsFrom(response: ImageGenerationResponse | ImageTaskResponse): ImageResult[] {
  const root = response as Record<string, unknown>
  const responseData = root.result ?? root.data
  const nested = asRecord(responseData)
  const items = Array.isArray(responseData) ? responseData : Array.isArray(nested?.data) ? nested.data : []
  return items.flatMap((rawItem) => {
    const item = asRecord(rawItem)
    if (!item) return []
    let url = safeMediaURL(item.url, 'image')
    if (!url && typeof item.b64_json === 'string' && item.b64_json.trim()) {
      const format = String(item.output_format || '').toLowerCase()
      const mimeCandidate = typeof item.mime_type === 'string' ? item.mime_type.toLowerCase() : ''
      const mime = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(mimeCandidate)
        ? mimeCandidate
        : format === 'jpeg' || format === 'jpg' ? 'image/jpeg' : format === 'webp' ? 'image/webp' : 'image/png'
      url = `data:${mime};base64,${item.b64_json}`
    }
    return url ? [{ url, revisedPrompt: typeof item.revised_prompt === 'string' ? item.revised_prompt : undefined }] : []
  })
}

function taskField(response: VideoTaskResponse | ImageTaskResponse | null, ...keys: string[]): string {
  if (!response) return ''
  const root = response as Record<string, unknown>
  const nested = asRecord(root.data)
  for (const key of keys) {
    const value = root[key] ?? nested?.[key]
    if (typeof value === 'string' || typeof value === 'number') {
      const text = String(value).trim()
      if (text) return text
    }
  }
  return ''
}

function readVideoStatus(response: VideoTaskResponse | null): string {
  return taskField(response, 'status', 'state').toLowerCase()
}

function videoResultURL(response: VideoTaskResponse | null): string {
  if (!response) return ''
  const root = response as Record<string, unknown>
  const nested = asRecord(root.data)
  const rootVideo = asRecord(root.video)
  const nestedVideo = asRecord(nested?.video)
  return safeMediaURL(rootVideo?.url ?? nestedVideo?.url ?? root.url ?? nested?.url ?? root.video_url ?? nested?.video_url ?? root.download_url ?? nested?.download_url, 'video')
}

function videoIsTerminal(status: string): boolean {
  return ['done', 'completed', 'complete', 'succeeded', 'success', 'failed', 'error', 'cancelled', 'canceled', 'rejected', 'expired'].includes(status)
}

async function waitForNextVideoPoll(): Promise<void> {
  await new Promise<void>((resolve) => {
    resolvePollTimer = resolve
    pollTimer = setTimeout(() => {
      pollTimer = undefined
      resolvePollTimer = undefined
      resolve()
    }, VIDEO_POLL_INTERVAL_MS)
  })
}

async function pollVideoTask(taskID: string, initialResponse?: VideoTaskResponse): Promise<void> {
  videoTaskID.value = taskID
  videoResponse.value = initialResponse || null
  videoURL.value = videoResultURL(videoResponse.value)
  mediaNotice.value = ''
  const initialResponseIsTerminal = initialResponse ? (
    videoIsTerminal(readVideoStatus(initialResponse)) || Boolean(videoResultURL(initialResponse))
  ) : false

  for (let attempt = 0; attempt < MAX_VIDEO_POLL_ATTEMPTS && !disposed; attempt += 1) {
    if (attempt > 0) {
      await waitForNextVideoPoll()
      if (disposed) return
    }
    // A terminal create response is useful for immediate rendering, but still
    // pass it through the AgentAPI task route once so the server can refresh
    // the persisted task status and verify its original settlement.
    if (attempt > 0 || !videoResponse.value || (attempt === 0 && initialResponseIsTerminal)) {
      videoResponse.value = await agentAPI.model.getVideo(taskID)
    }
    videoStatus.value = readVideoStatus(videoResponse.value) || (videoURL.value ? 'done' : 'pending')
    videoURL.value = videoResultURL(videoResponse.value)
    if (videoURL.value || videoIsTerminal(videoStatus.value)) {
      if (videoStatus.value === 'failed' || videoStatus.value === 'error' || videoStatus.value === 'rejected' || videoStatus.value === 'expired') {
        mediaNotice.value = '视频任务未能成功完成。'
      } else if (!videoURL.value) {
        mediaNotice.value = '视频任务已结束，但未返回可播放的视频。'
      }
      return
    }
  }
  if (!disposed) mediaNotice.value = '任务仍在处理中，可以再次查询状态，无需重新创建任务。'
}

async function pollImageTask(taskID: string, initialResponse?: ImageTaskResponse): Promise<void> {
  imageTaskID.value = taskID
  imageResponse.value = initialResponse || null
  mediaNotice.value = ''
  const initialResponseIsTerminal = initialResponse
    ? imageIsTerminal(taskField(initialResponse, 'status', 'state').toLowerCase())
    : false

  for (let attempt = 0; attempt < MAX_VIDEO_POLL_ATTEMPTS && !disposed; attempt += 1) {
    if (attempt > 0) {
      await waitForNextVideoPoll()
      if (disposed) return
    }
    // Let the backend confirm terminal create responses through the original
    // task's scoped polling route before the UI stops checking it.
    if (attempt > 0 || !imageResponse.value || (attempt === 0 && initialResponseIsTerminal)) {
      imageResponse.value = await agentAPI.model.getImageTask(taskID)
    }
    const status = taskField(imageResponse.value, 'status', 'state').toLowerCase()
    imageStatus.value = status
    if (imageIsTerminal(status)) {
      imageResults.value = imageResultsFrom(imageResponse.value)
      if (imageResults.value.length === 0) mediaNotice.value = '图片任务已结束，但未返回可显示的图片。'
      if (['failed', 'error', 'cancelled', 'canceled', 'rejected', 'expired'].includes(status)) {
        mediaNotice.value = '图片任务未能成功完成。'
      }
      return
    }
  }
  if (!disposed) mediaNotice.value = '图片任务仍在处理中，可以再次查询状态，无需重新创建任务。'
}

function imageIsTerminal(status: string): boolean {
  return ['done', 'completed', 'complete', 'succeeded', 'success', 'failed', 'error', 'cancelled', 'canceled', 'rejected', 'expired'].includes(status)
}

async function submitText(): Promise<void> {
  if (!canSubmit.value) return
  const content = prompt.value.trim()
  prompt.value = ''
  error.value = ''
  messages.value.push({ role: 'user', content })
  loading.value = true
  try {
    // Pass a snapshot to the request so later conversation updates cannot mutate the sent payload.
    const response = await agentAPI.model.chat(model.value, messages.value.map((message) => ({ ...message })))
    messages.value.push({ role: 'assistant', content: responseText(response) })
  } catch (err) {
    error.value = errorMessage(err, '模型请求失败，请稍后重试。')
    prompt.value = content
  } finally {
    loading.value = false
  }
}

function chooseImage(event: Event): void {
  const selected = (event.target as HTMLInputElement).files?.[0] || null
  if (selected && selected.type && !selected.type.startsWith('image/')) {
    error.value = '请选择图片文件。'
    imageFile.value = null
    return
  }
  if (selected && selected.size > 20 * 1024 * 1024) {
    error.value = '参考图片必须小于 20 MB。'
    imageFile.value = null
    return
  }
  imageFile.value = selected
  error.value = ''
}

async function submitImage(): Promise<void> {
  if (!canSubmit.value) return
  error.value = ''
  mediaNotice.value = ''
  imageResults.value = []
  imageTaskID.value = ''
  imageStatus.value = ''
  imageResponse.value = null
  loading.value = true
  try {
    if (asyncImage.value) {
      const created = imageFile.value
        ? await agentAPI.model.editImageAsync(model.value, prompt.value.trim(), imageFile.value)
        : await agentAPI.model.generateImageAsync(model.value, prompt.value.trim())
      const taskID = taskField(created, 'id', 'task_id')
      if (!taskID) throw new Error('图片服务已接受请求，但未返回任务编号。')
      await loadTaskHistory()
      await pollImageTask(taskID, created)
      await loadTaskHistory()
      return
    }
    const response = imageFile.value
      ? await agentAPI.model.editImage(model.value, prompt.value.trim(), imageFile.value)
      : await agentAPI.model.generateImage(model.value, prompt.value.trim())
    imageResults.value = imageResultsFrom(response)
    if (imageResults.value.length === 0) mediaNotice.value = '图片服务已处理请求，但未返回可显示的图片。'
  } catch (err) {
    error.value = errorMessage(err, '图片请求失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function checkImageStatus(): Promise<void> {
  if (!imageTaskID.value || loading.value) return
  error.value = ''
  loading.value = true
  try {
    await pollImageTask(imageTaskID.value)
    await loadTaskHistory()
  } catch (err) {
    error.value = errorMessage(err, '加载图片任务状态失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function submitVideo(): Promise<void> {
  if (!canSubmit.value) return
  error.value = ''
  mediaNotice.value = ''
  videoTaskID.value = ''
  videoStatus.value = 'submitting'
  videoURL.value = ''
  videoResponse.value = null
  loading.value = true
  try {
    const created = await agentAPI.model.createVideo(model.value, prompt.value.trim())
    const taskID = taskField(created, 'id', 'request_id', 'task_id', 'video_id')
    if (!taskID) throw new Error('视频服务已接受请求，但未返回任务编号。')
    await loadTaskHistory()
    await pollVideoTask(taskID, created)
    await loadTaskHistory()
  } catch (err) {
    error.value = errorMessage(err, '视频请求失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function checkVideoStatus(): Promise<void> {
  if (!videoTaskID.value || loading.value) return
  error.value = ''
  loading.value = true
  try {
    await pollVideoTask(videoTaskID.value)
    await loadTaskHistory()
  } catch (err) {
    error.value = errorMessage(err, '加载视频状态失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  if (mode.value === 'image') return submitImage()
  if (mode.value === 'video') return submitVideo()
  return submitText()
}

function clearConversation(): void {
  messages.value = []
  imageResults.value = []
  imageTaskID.value = ''
  imageStatus.value = ''
  imageResponse.value = null
  videoTaskID.value = ''
  videoStatus.value = ''
  videoURL.value = ''
  videoResponse.value = null
  mediaNotice.value = ''
  imageFile.value = null
  imageInputKey.value += 1
  error.value = ''
}
</script>

<template>
  <main class="mx-auto flex max-w-5xl flex-col gap-6 p-6">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-sm text-slate-500">AgentAPI</p>
        <h1 class="text-2xl font-semibold text-slate-900 dark:text-white">模型工作台</h1>
        <p class="mt-1 text-sm text-slate-500">请求通过 AgentAPI 会话发送，浏览器不会接触主站密钥。</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <label class="text-sm text-slate-500" for="agent-model">模型</label>
        <select id="agent-model" v-model="model" class="rounded-lg border px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" :disabled="modelsLoading || modelsByMode.length === 0 || loading">
          <option v-for="item in modelsByMode" :key="item" :value="item">{{ item }}</option>
        </select>
        <button class="rounded-lg border px-3 py-2 text-sm disabled:opacity-50" type="button" :disabled="loading" @click="clearConversation">清空</button>
      </div>
    </header>

    <nav aria-label="模型能力" class="flex flex-wrap gap-2">
      <button v-for="item in ([['text', '文本'], ['image', '图片'], ['video', '视频']] as const)" :key="item[0]" class="rounded-lg border px-4 py-2 text-sm" :class="mode === item[0] ? 'border-blue-600 bg-blue-600 text-white' : 'bg-white text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200'" type="button" :aria-pressed="mode === item[0]" :disabled="loading" @click="mode = item[0]">{{ item[1] }}</button>
    </nav>

    <p v-if="modelsLoading" class="rounded-lg bg-slate-50 p-4 text-sm text-slate-600 dark:bg-slate-900 dark:text-slate-300">正在加载已启用的公开模型…</p>
    <p v-else-if="modelsError" class="rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ modelsError }}</p>
    <p v-else-if="modelsByMode.length === 0" class="rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">当前代理站尚未启用{{ taskTypeLabel(mode) }}模型。</p>
    <p v-if="error" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>

    <section v-if="mode === 'text'" class="min-h-[28rem] rounded-2xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div v-if="messages.length === 0" class="flex h-[25rem] items-center justify-center text-center text-sm text-slate-500">
        <div><p class="text-lg font-medium text-slate-700 dark:text-slate-200">试着发送模型请求</p><p class="mt-2">所选公开模型将通过 AgentAPI 发送到 Sub2API 主站网关。</p></div>
      </div>
      <div v-else class="space-y-5">
        <article v-for="(message, index) in messages" :key="`${message.role}-${index}`" :class="message.role === 'user' ? 'ml-auto max-w-3xl bg-blue-50 dark:bg-blue-950/30' : 'mr-auto max-w-4xl bg-slate-50 dark:bg-slate-800'" class="rounded-xl p-4">
          <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{{ statusLabel(message.role) }}</p>
          <p class="whitespace-pre-wrap text-sm leading-7 text-slate-800 dark:text-slate-100">{{ message.content }}</p>
        </article>
        <p v-if="loading" class="text-sm text-slate-500">正在等待上游响应…</p>
      </div>
    </section>

    <section v-else-if="mode === 'image'" class="space-y-5 rounded-2xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div>
        <h2 class="text-lg font-medium text-slate-800 dark:text-slate-100">图片生成与编辑</h2>
        <p class="mt-1 text-sm text-slate-500">添加参考图片以使用图片编辑接口；不添加图片则生成新图片。</p>
      </div>
      <label class="block text-sm font-medium text-slate-700 dark:text-slate-200" for="agent-image-prompt">提示词
        <textarea id="agent-image-prompt" v-model="prompt" class="mt-2 min-h-28 w-full resize-y rounded-xl border px-4 py-3 text-sm outline-none focus:border-blue-500 dark:border-slate-700 dark:bg-slate-950" :disabled="loading || !model" placeholder="描述想生成的图片或需要修改的内容…"></textarea>
      </label>
      <div class="block text-sm text-slate-600 dark:text-slate-300">
        <label class="block" for="agent-image-file">参考图片（选填）</label>
        <div class="mt-2 flex flex-wrap items-center gap-3">
          <input :key="imageInputKey" id="agent-image-file" class="sr-only" accept="image/*" type="file" :disabled="loading || !model" @change="chooseImage">
          <label for="agent-image-file" class="cursor-pointer rounded-lg border px-3 py-2 text-sm dark:border-slate-700" :class="loading || !model ? 'pointer-events-none opacity-50' : ''">选择图片</label>
          <span class="text-xs text-slate-500">{{ imageFile?.name || '尚未选择图片' }}</span>
        </div>
      </div>
      <label class="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-300">
        <input v-model="asyncImage" type="checkbox" :disabled="loading || !model">
        作为后台任务运行并持续查询结果
      </label>
      <div v-if="imageTaskID" class="rounded-xl bg-slate-50 p-4 text-sm dark:bg-slate-950">
        <p>任务 <code class="break-all">{{ imageTaskID }}</code><span v-if="imageStatus"> · {{ statusLabel(imageStatus) }}</span></p>
        <button class="mt-3 rounded-lg border px-3 py-2 text-sm disabled:opacity-50 dark:border-slate-700" type="button" :disabled="loading" @click="checkImageStatus">{{ loading ? '正在查询…' : '再次查询状态' }}</button>
      </div>
      <div v-if="imageResults.length" class="grid gap-4 sm:grid-cols-2">
        <figure v-for="(item, index) in imageResults" :key="`${index}-${item.url.slice(0, 40)}`" class="overflow-hidden rounded-xl border dark:border-slate-700">
          <img class="max-h-[32rem] w-full object-contain bg-slate-50 dark:bg-slate-950" :src="item.url" :alt="item.revisedPrompt || `生成的图片 ${index + 1}`" loading="lazy">
          <figcaption v-if="item.revisedPrompt" class="p-3 text-xs text-slate-500">{{ item.revisedPrompt }}</figcaption>
        </figure>
      </div>
      <p v-if="mediaNotice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-800 dark:bg-blue-950/30 dark:text-blue-200">{{ mediaNotice }}</p>
    </section>

    <section v-else class="space-y-5 rounded-2xl border bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div>
        <h2 class="text-lg font-medium text-slate-800 dark:text-slate-100">视频生成</h2>
        <p class="mt-1 text-sm text-slate-500">视频生成采用异步处理。AgentAPI 会跟踪任务并查询状态，不会重复提交生成请求。</p>
      </div>
      <label class="block text-sm font-medium text-slate-700 dark:text-slate-200" for="agent-video-prompt">提示词
        <textarea id="agent-video-prompt" v-model="prompt" class="mt-2 min-h-28 w-full resize-y rounded-xl border px-4 py-3 text-sm outline-none focus:border-blue-500 dark:border-slate-700 dark:bg-slate-950" :disabled="loading || !model" placeholder="描述想生成的视频…"></textarea>
      </label>
      <div v-if="videoTaskID" class="rounded-xl bg-slate-50 p-4 text-sm dark:bg-slate-950">
        <p>任务 <code class="break-all">{{ videoTaskID }}</code><span v-if="videoStatus"> · {{ statusLabel(videoStatus) }}</span></p>
        <button class="mt-3 rounded-lg border px-3 py-2 text-sm disabled:opacity-50 dark:border-slate-700" type="button" :disabled="loading" @click="checkVideoStatus">{{ loading ? '正在查询…' : '再次查询状态' }}</button>
      </div>
      <video v-if="videoURL" class="max-h-[34rem] w-full rounded-xl bg-black" :src="videoURL" controls playsinline />
      <p v-if="mediaNotice" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-800 dark:bg-blue-950/30 dark:text-blue-200">{{ mediaNotice }}</p>
    </section>

    <form class="rounded-2xl border bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-900" @submit.prevent="submit">
      <template v-if="mode === 'text'">
        <label class="sr-only" for="agent-prompt">消息</label>
        <textarea id="agent-prompt" v-model="prompt" class="min-h-28 w-full resize-y rounded-xl border px-4 py-3 text-sm outline-none focus:border-blue-500 dark:border-slate-700 dark:bg-slate-950" :disabled="loading || !model || modelsLoading" placeholder="向模型提问…" @keydown.ctrl.enter.prevent="submit"></textarea>
      </template>
      <p v-else class="px-1 text-sm text-slate-500">{{ mode === 'image' ? (asyncImage ? '图片任务会持续跟踪，并按原始请求结算。' : imageFile ? '所选图片和提示词将发送到图片编辑接口。' : '提示词将发送到图片生成接口。') : '系统将自动创建任务并查询处理状态。' }}</p>
      <div class="mt-3 flex items-center justify-between gap-3">
        <p class="text-xs text-slate-500">{{ mode === 'text' ? '按 Ctrl + Enter 发送' : '仅可使用已启用的公开模型' }} · 费用按 AgentAPI 钱包计费规则扣除。</p>
        <button class="rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" type="submit" :disabled="!canSubmit">{{ loading ? (mode === 'video' ? '正在查询任务…' : mode === 'image' ? '正在生成…' : '正在发送…') : mode === 'image' ? '生成图片' : mode === 'video' ? '生成视频' : '发送' }}</button>
      </div>
    </form>

    <section class="overflow-hidden rounded-2xl border bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b p-5 dark:border-slate-700">
        <div>
          <h2 class="font-medium text-slate-800 dark:text-slate-100">最近的后台任务</h2>
          <p class="mt-1 text-sm text-slate-500">图片和视频任务编号保存在此代理站。刷新或重新打开工作台后，可以继续查询任务。</p>
        </div>
        <button class="rounded-lg border px-3 py-2 text-sm disabled:opacity-50 dark:border-slate-700" type="button" :disabled="taskHistoryLoading" @click="loadTaskHistory">{{ taskHistoryLoading ? '正在刷新…' : '刷新任务' }}</button>
      </div>
      <p v-if="taskHistoryError" role="alert" class="m-5 rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ taskHistoryError }}</p>
      <p v-else-if="taskHistoryLoading && taskHistory.length === 0" class="p-5 text-sm text-slate-500">正在加载最近任务…</p>
      <p v-else-if="taskHistory.length === 0" class="p-5 text-sm text-slate-500">此账号暂无图片或视频任务记录。</p>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="bg-slate-50 text-slate-500 dark:bg-slate-800"><tr><th class="px-5 py-3">类型</th><th class="px-5 py-3">任务</th><th class="px-5 py-3">模型</th><th class="px-5 py-3">任务状态</th><th class="px-5 py-3">结算状态</th><th class="px-5 py-3">更新时间</th><th class="px-5 py-3"></th></tr></thead>
          <tbody>
            <tr v-for="task in taskHistory" :key="`${task.task_type}-${task.task_id}`" class="border-t dark:border-slate-700">
              <td class="px-5 py-4">{{ taskTypeLabel(task.task_type) }}</td>
              <td class="max-w-64 px-5 py-4"><div class="truncate font-mono text-xs" :title="task.task_id">{{ task.task_id }}</div><div class="mt-1 truncate font-mono text-[11px] text-slate-400" :title="task.request_id">{{ task.request_id }}</div></td>
              <td class="px-5 py-4 font-mono text-xs">{{ task.model || '—' }}</td>
              <td class="px-5 py-4">{{ statusLabel(task.status) }}</td>
              <td class="px-5 py-4">{{ statusLabel(task.settlement_status || 'not_recorded') }}<div v-if="task.reserved_cents > 0 || task.actual_cents > 0" class="mt-1 text-xs text-slate-500">{{ (task.actual_cents / 100).toFixed(2) }} / {{ (task.reserved_cents / 100).toFixed(2) }}</div></td>
              <td class="whitespace-nowrap px-5 py-4 text-xs text-slate-500">{{ new Date(task.updated_at).toLocaleString('zh-CN') }}</td>
              <td class="px-5 py-4 text-right"><button class="rounded-lg border px-3 py-1.5 text-xs disabled:opacity-50 dark:border-slate-700" type="button" :disabled="loading || modelsLoading" @click="resumeTask(task)">{{ loading && ((task.task_type === 'image' && imageTaskID === task.task_id) || (task.task_type === 'video' && videoTaskID === task.task_id)) ? '正在查询…' : '继续查询' }}</button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <AgentPagination
        v-if="taskHistoryTotal > 0"
        :total="taskHistoryTotal"
        :page="taskHistoryPage"
        :page-size="taskHistoryPageSize"
        item-label="个任务"
        @update:page="changeTaskHistoryPage"
        @update:page-size="changeTaskHistoryPageSize"
      />
    </section>
  </main>
</template>
