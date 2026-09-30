<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentTaskHistoryItem, type ImageTaskResponse } from '@/agent/api'
import { errorMessage, statusLabel } from '@/agent/locale'
import { imageResultsFrom, imageTaskID, imageTaskStatus, type AgentImageResult } from '@/agent/imageTasks'
import { modelCapability } from '@/agent/modelCatalog'
import AgentPagination from '@/components/AgentPagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentStatusBadge from '@/components/common/AgentStatusBadge.vue'
import AgentTextArea from '@/components/common/AgentTextArea.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'

type SubmissionState = 'waiting' | 'submitting' | 'submitted' | 'failed'
type BatchSubmission = {
  prompt: string
  state: SubmissionState
  taskID: string
  status: string
  error: string
}

const imageModels = ref<string[]>([])
const model = ref('')
const promptText = ref('')
const concurrency = ref(2)
const createOpen = ref(false)
const modelsLoading = ref(true)
const historyLoading = ref(true)
const submitting = ref(false)
const error = ref('')
const historyError = ref('')
const history = ref<AgentTaskHistoryItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const submissions = ref<BatchSubmission[]>([])
const detailOpen = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const detailTask = ref<AgentTaskHistoryItem | null>(null)
const detailResponse = ref<ImageTaskResponse | null>(null)
const detailImages = ref<AgentImageResult[]>([])
let historySequence = 0

const prompts = computed(() => promptText.value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean))
const imageModelOptions = computed(() => imageModels.value.map(value => ({ value, label: value })))
const concurrencyOptions = [1, 2, 3].map(value => ({ value, label: `${value} 个` }))
const promptLimitExceeded = computed(() => prompts.value.length > 20)
const canSubmit = computed(() => !submitting.value && Boolean(model.value) && prompts.value.length > 0 && !promptLimitExceeded.value)
const submittedCount = computed(() => submissions.value.filter((item) => item.state === 'submitted').length)
const failedCount = computed(() => submissions.value.filter((item) => item.state === 'failed').length)

const columns = [
  { key: 'task_id', label: '任务' },
  { key: 'model', label: '模型' },
  { key: 'status', label: '任务状态' },
  { key: 'settlement_status', label: '结算状态' },
  { key: 'actual_cents', label: '费用' },
  { key: 'updated_at', label: '更新时间' },
  { key: 'actions', label: '操作' },
]

onMounted(() => {
  void Promise.all([loadModels(), loadHistory()])
})

async function loadModels(): Promise<void> {
  modelsLoading.value = true
  try {
    imageModels.value = (await agentAPI.model.list()).filter((name) => modelCapability(name) === 'image')
    if (!imageModels.value.includes(model.value)) model.value = imageModels.value[0] || ''
  } catch (err) {
    error.value = errorMessage(err, '加载图片模型失败，请稍后刷新重试。')
  } finally {
    modelsLoading.value = false
  }
}

async function loadHistory(): Promise<void> {
  const sequence = ++historySequence
  historyLoading.value = true
  historyError.value = ''
  try {
    const result = await agentAPI.getTasks(page.value, pageSize.value, 'image')
    if (sequence !== historySequence) return
    history.value = result.items
    total.value = result.total
    const lastPage = Math.max(1, Math.ceil(total.value / pageSize.value))
    if (page.value > lastPage) {
      page.value = lastPage
      await loadHistory()
    }
  } catch (err) {
    if (sequence === historySequence) historyError.value = errorMessage(err, '加载图片任务失败，请稍后刷新重试。')
  } finally {
    if (sequence === historySequence) historyLoading.value = false
  }
}

function openCreate(): void {
  error.value = ''
  submissions.value = []
  createOpen.value = true
}

async function submitBatch(): Promise<void> {
  if (!canSubmit.value) return
  error.value = ''
  submitting.value = true
  submissions.value = prompts.value.map((prompt) => ({ prompt, state: 'waiting', taskID: '', status: '', error: '' }))
  let cursor = 0
  const worker = async () => {
    while (cursor < submissions.value.length) {
      const index = cursor
      cursor += 1
      const item = submissions.value[index]
      item.state = 'submitting'
      try {
        // Each prompt is submitted exactly once. A failed request remains
        // visible for deliberate user action instead of being billed twice by
        // an automatic retry loop.
        const response = await agentAPI.model.generateImageAsync(model.value, item.prompt)
        const taskID = imageTaskID(response)
        if (!taskID) throw new Error('图片服务未返回任务编号')
        item.taskID = taskID
        item.status = imageTaskStatus(response) || 'queued'
        item.state = 'submitted'
      } catch (err) {
        item.state = 'failed'
        item.error = errorMessage(err, '提交失败，请检查后单独重新创建。')
      }
    }
  }
  try {
    await Promise.all(Array.from({ length: Math.min(concurrency.value, submissions.value.length) }, () => worker()))
    promptText.value = failedCount.value > 0
      ? submissions.value.filter((item) => item.state === 'failed').map((item) => item.prompt).join('\n')
      : ''
    await loadHistory()
  } finally {
    submitting.value = false
  }
}

async function openTask(task: AgentTaskHistoryItem): Promise<void> {
  detailTask.value = task
  detailResponse.value = null
  detailImages.value = []
  detailError.value = ''
  detailOpen.value = true
  detailLoading.value = true
  try {
    detailResponse.value = await agentAPI.model.getImageTask(task.task_id)
    detailImages.value = imageResultsFrom(detailResponse.value)
    await loadHistory()
  } catch (err) {
    detailError.value = errorMessage(err, '查询图片任务失败，请稍后重试。')
  } finally {
    detailLoading.value = false
  }
}

async function changePage(nextPage: number): Promise<void> {
  if (nextPage === page.value || nextPage < 1) return
  page.value = nextPage
  await loadHistory()
}

async function changePageSize(nextPageSize: number): Promise<void> {
  if (nextPageSize === pageSize.value) return
  pageSize.value = nextPageSize
  page.value = 1
  await loadHistory()
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN')
}
</script>

<template>
  <TablePageLayout>
    <template #actions>
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <p class="text-sm font-medium text-primary-600 dark:text-primary-400">图片生成</p>
          <h2 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">批量图片任务</h2>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">沿用主站的批量工作流；每行提示词创建一个真实异步任务，任务、费用和结果仅属于当前代理站账号。</p>
        </div>
        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary" :disabled="historyLoading" @click="loadHistory"><Icon name="refresh" class="mr-2" />刷新</button>
          <button type="button" class="btn btn-primary" :disabled="modelsLoading || imageModels.length === 0" @click="openCreate"><Icon name="plus" class="mr-2" />新建批量任务</button>
        </div>
      </div>
      <p v-if="error" role="alert" class="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <p v-if="!modelsLoading && imageModels.length === 0" class="mt-4 rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">当前代理站尚未启用图片模型，请联系本站管理员配置模型权限。</p>
    </template>

    <template #filters>
      <div class="grid gap-4 sm:grid-cols-3">
        <div class="card p-4"><p class="text-xs text-gray-500 dark:text-dark-400">图片任务</p><p class="mt-1 text-2xl font-semibold">{{ total }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500 dark:text-dark-400">可用图片模型</p><p class="mt-1 text-2xl font-semibold">{{ imageModels.length }}</p></div>
        <div class="card p-4"><p class="text-xs text-gray-500 dark:text-dark-400">最近批次</p><p class="mt-1 text-sm font-semibold">{{ submissions.length ? `${submittedCount} 已提交 / ${failedCount} 失败` : '尚未在本次会话提交' }}</p></div>
      </div>
      <p v-if="historyError" role="alert" class="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ historyError }}</p>
    </template>

    <template #table>
      <DataTable :columns="columns" :data="history" :loading="historyLoading" row-key="task_id">
        <template #cell-task_id="{ row }"><div class="max-w-72"><p class="truncate font-mono text-xs" :title="row.task_id">{{ row.task_id }}</p><p class="mt-1 truncate font-mono text-[11px] text-gray-400" :title="row.request_id">{{ row.request_id }}</p></div></template>
        <template #cell-model="{ row }"><span class="font-mono text-xs">{{ row.model || '—' }}</span></template>
        <template #cell-status="{ row }"><AgentStatusBadge :status="row.status" :label="statusLabel(row.status)" /></template>
        <template #cell-settlement_status="{ row }"><AgentStatusBadge :status="row.settlement_status || 'not_recorded'" :label="statusLabel(row.settlement_status || 'not_recorded')" /></template>
        <template #cell-actual_cents="{ row }"><span class="tabular-nums">{{ (row.actual_cents / 100).toFixed(2) }}</span><span class="ml-1 text-xs text-gray-400">/ {{ (row.reserved_cents / 100).toFixed(2) }}</span></template>
        <template #cell-updated_at="{ row }"><span class="text-xs text-gray-500">{{ formatTime(row.updated_at) }}</span></template>
        <template #cell-actions="{ row }"><button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" @click.stop="openTask(row)">查询结果</button></template>
        <template #empty><EmptyState title="暂无图片任务" description="创建批量任务后，每一项都会作为可恢复的服务端任务显示在这里。" action-text="新建批量任务" @action="openCreate" /></template>
      </DataTable>
    </template>

    <template #pagination>
      <AgentPagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" item-label="个图片任务" @update:page="changePage" @update:page-size="changePageSize" />
    </template>
  </TablePageLayout>

  <BaseDialog :show="createOpen" title="新建批量图片任务" width="wide" @close="!submitting && (createOpen = false)">
    <div class="space-y-5">
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block text-sm font-medium">图片模型<AgentSelect id="batch-image-model" v-model="model" :options="imageModelOptions" class="mt-2 w-full" aria-label="图片模型" :disabled="submitting" /></label>
        <label class="block text-sm font-medium">并发提交<AgentSelect id="batch-image-concurrency" v-model="concurrency" :options="concurrencyOptions" class="mt-2 w-full" aria-label="并发提交" :disabled="submitting" /></label>
      </div>
      <label class="block text-sm font-medium">提示词（每行一个，最多 20 个）<AgentTextArea v-model="promptText" id="batch-image-prompts" class="mt-2 min-h-48" placeholder="一只在窗边读书的橘猫&#10;雨后的未来城市街道" :disabled="submitting" /></label>
      <div class="flex items-center justify-between text-xs"><span :class="promptLimitExceeded ? 'text-red-600' : 'text-gray-500'">已识别 {{ prompts.length }} 条提示词</span><span class="text-gray-400">失败项不会自动重试</span></div>
      <div v-if="submissions.length" class="max-h-64 overflow-y-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <div v-for="(item, index) in submissions" :key="`${index}-${item.prompt}`" class="flex items-start gap-3 border-b border-gray-100 px-4 py-3 last:border-b-0 dark:border-dark-800">
          <span class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-100 text-xs dark:bg-dark-700">{{ index + 1 }}</span>
          <div class="min-w-0 flex-1"><p class="truncate text-sm" :title="item.prompt">{{ item.prompt }}</p><p v-if="item.taskID" class="mt-1 truncate font-mono text-[11px] text-gray-400">{{ item.taskID }}</p><p v-if="item.error" class="mt-1 text-xs text-red-600">{{ item.error }}</p></div>
          <span class="text-xs" :class="item.state === 'failed' ? 'text-red-600' : item.state === 'submitted' ? 'text-emerald-600' : 'text-gray-500'">{{ item.state === 'waiting' ? '等待' : item.state === 'submitting' ? '提交中' : item.state === 'submitted' ? '已提交' : '失败' }}</span>
        </div>
      </div>
    </div>
    <template #footer><button type="button" class="btn btn-secondary" :disabled="submitting" @click="createOpen = false">关闭</button><button type="button" class="btn btn-primary" :disabled="!canSubmit" @click="submitBatch">{{ submitting ? '正在提交…' : `提交 ${prompts.length || ''} 个任务` }}</button></template>
  </BaseDialog>

  <BaseDialog :show="detailOpen" title="图片任务结果" width="wide" @close="detailOpen = false">
    <div class="space-y-4">
      <div v-if="detailTask" class="rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800"><p class="font-mono text-xs">{{ detailTask.task_id }}</p><div class="mt-2 flex flex-wrap items-center gap-2"><span class="text-gray-500">{{ detailTask.model || '未记录模型' }}</span><AgentStatusBadge :status="imageTaskStatus(detailResponse) || detailTask.status" :label="statusLabel(imageTaskStatus(detailResponse) || detailTask.status)" /></div></div>
      <div v-if="detailLoading" class="py-12 text-center text-sm text-gray-500">正在查询任务…</div>
      <p v-else-if="detailError" role="alert" class="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ detailError }}</p>
      <div v-else-if="detailImages.length" class="grid gap-4 sm:grid-cols-2"><figure v-for="image in detailImages" :key="image.url" class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700"><img :src="image.url" alt="生成结果" class="aspect-square w-full object-contain bg-gray-50 dark:bg-dark-950"><figcaption v-if="image.revisedPrompt" class="p-3 text-xs text-gray-500">{{ image.revisedPrompt }}</figcaption></figure></div>
      <EmptyState v-else title="暂时没有可显示的图片" description="任务可能仍在处理，关闭后稍后再次查询即可，不需要重新创建任务。" />
    </div>
  </BaseDialog>
</template>
