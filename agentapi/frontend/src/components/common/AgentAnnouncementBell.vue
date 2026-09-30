<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { agentAPI, type AgentAnnouncement } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const items = ref<AgentAnnouncement[]>([])
const loading = ref(false)
const open = ref(false)
const selected = ref<AgentAnnouncement | null>(null)
const error = ref('')
let pollTimer: ReturnType<typeof setInterval> | undefined

const unreadCount = computed(() => items.value.filter(item => !item.read_at).length)

function formatTime(value: string): string {
  if (!value) return ''
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function load(showLoading = false): Promise<void> {
  if (showLoading) loading.value = true
  error.value = ''
  try {
    const result = await agentAPI.announcements.list()
    items.value = result.items
    const popup = result.items.find(item => item.notify_mode === 'popup' && !item.read_at && sessionStorage.getItem(`agentapi_announcement_seen_${item.id}`) !== '1')
    if (popup) {
      sessionStorage.setItem(`agentapi_announcement_seen_${popup.id}`, '1')
      selected.value = popup
    }
  } catch (err) {
    error.value = errorMessage(err, '公告加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function showDetail(item: AgentAnnouncement): Promise<void> {
  open.value = false
  selected.value = item
  if (!item.read_at) {
    try {
      await agentAPI.announcements.markRead(item.id)
      item.read_at = new Date().toISOString()
    } catch (err) {
      error.value = errorMessage(err, '标记公告失败，请稍后重试。')
    }
  }
}

async function acknowledgeSelected(): Promise<void> {
  const item = selected.value
  if (!item) return
  if (!item.read_at) {
    try {
      await agentAPI.announcements.markRead(item.id)
      item.read_at = new Date().toISOString()
    } catch (err) {
      error.value = errorMessage(err, '标记公告失败，请稍后重试。')
      return
    }
  }
  selected.value = null
}

async function markAllRead(): Promise<void> {
  try {
    await agentAPI.announcements.markAllRead()
    const now = new Date().toISOString()
    items.value.forEach(item => { if (!item.read_at) item.read_at = now })
  } catch (err) {
    error.value = errorMessage(err, '标记公告失败，请稍后重试。')
  }
}

onMounted(() => {
  void load()
  pollTimer = setInterval(() => { void load() }, 60_000)
})
onBeforeUnmount(() => { if (pollTimer) clearInterval(pollTimer) })
</script>

<template>
  <div class="relative">
    <button class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-500 transition hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-800" aria-label="站内公告" @click="open = true; load(true)">
      <Icon name="bell" />
      <span v-if="unreadCount" class="absolute right-1 top-1 h-2 w-2 rounded-full bg-red-500 ring-2 ring-white dark:ring-dark-900"></span>
    </button>

    <BaseDialog :show="open" title="站内公告" width="wide" :z-index="100" close-on-click-outside @close="open = false">
      <div class="mb-4 flex items-center justify-between gap-3" aria-label="站内公告列表">
        <div class="flex items-center gap-3">
          <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-primary-600 text-white"><Icon name="bell" /></span>
          <p class="text-sm text-gray-500">{{ unreadCount ? `${unreadCount} 条未读` : '已全部阅读' }}</p>
        </div>
        <button v-if="unreadCount" class="btn btn-secondary text-xs" :disabled="loading" @click="markAllRead">全部已读</button>
      </div>
      <p v-if="error" role="alert" class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-200">{{ error }}</p>
      <div class="max-h-[65vh] overflow-y-auto rounded-xl border border-gray-100 dark:border-dark-700">
        <p v-if="loading" class="p-10 text-center text-sm text-gray-500">正在加载公告…</p>
        <button v-for="item in items" v-else :key="item.id" class="relative flex w-full items-center gap-4 border-b border-gray-100 px-5 py-4 text-left transition last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/50" :class="{ 'bg-primary-50/60 dark:bg-primary-950/20': !item.read_at }" @click="showDetail(item)">
          <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl" :class="item.read_at ? 'bg-gray-100 text-gray-400 dark:bg-dark-700' : 'bg-primary-600 text-white'"><Icon :name="item.read_at ? 'check' : 'bell'" /></span>
          <span class="min-w-0 flex-1"><span class="block truncate text-sm font-medium">{{ item.title }}</span><span class="mt-1 block text-xs text-gray-500">{{ formatTime(item.created_at) }}</span></span>
          <span v-if="!item.read_at" class="h-2 w-2 rounded-full bg-primary-500"></span>
          <Icon name="chevronRight" class="text-gray-400" />
        </button>
        <div v-if="!loading && !items.length" class="p-12 text-center"><Icon name="inbox" class="mx-auto mb-3 text-gray-400" /><p class="text-sm font-medium">暂无公告</p><p class="mt-1 text-xs text-gray-500">本站管理员发布公告后会显示在这里。</p></div>
      </div>
    </BaseDialog>

    <BaseDialog :show="selected !== null" :title="selected?.title || '公告详情'" width="wide" :z-index="110" close-on-click-outside @close="selected = null">
      <template v-if="selected">
        <div class="mb-5 flex items-center gap-2 text-xs text-gray-500"><span class="badge badge-primary">站内公告</span><time>{{ formatTime(selected.created_at) }}</time></div>
        <p v-if="error" role="alert" class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-200">{{ error }}</p>
        <p class="max-h-[60vh] overflow-y-auto whitespace-pre-wrap break-words text-sm leading-7 text-gray-700 dark:text-gray-200">{{ selected.content }}</p>
      </template>
      <template #footer><button class="btn btn-primary" @click="acknowledgeSelected">我知道了</button></template>
    </BaseDialog>
  </div>
</template>
