<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AgentConfirmDialog from '@/components/AgentConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import AgentTextArea from '@/components/common/AgentTextArea.vue'
import { agentAPI, type AgentContentPage, type AgentContentPageInput } from '@/agent/api'
import { errorMessage } from '@/agent/locale'

const pages = ref<AgentContentPage[]>([])
const loading = ref(true)
const saving = ref(false)
const deleting = ref(false)
const notice = ref('')
const editingID = ref<number | null>(null)
const deleteTarget = ref<AgentContentPage | null>(null)
const form = ref<AgentContentPageInput>({ slug: '', kind: 'custom', title: '', content: '', status: 'draft', sort_order: 0 })
const kindOptions = [
  { value: 'custom', label: '自定义页面' },
  { value: 'legal', label: '公开协议' },
]
const statusOptions = [
  { value: 'draft', label: '草稿' },
  { value: 'active', label: '已发布' },
  { value: 'archived', label: '已归档' },
]

const props = withDefaults(defineProps<{ autoLoad?: boolean }>(), { autoLoad: true })

function reset(): void {
  editingID.value = null
  form.value = { slug: '', kind: 'custom', title: '', content: '', status: 'draft', sort_order: 0 }
}

function edit(item: AgentContentPage): void {
  editingID.value = item.id
  form.value = {
    slug: item.slug, kind: item.kind, title: item.title, content: item.content,
    status: item.status, sort_order: item.sort_order,
  }
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

async function load(): Promise<void> {
  loading.value = true
  notice.value = ''
  try {
    pages.value = (await agentAPI.adminContentPages.list()).items
  } catch (err) {
    notice.value = errorMessage(err, '内容页面加载失败，请稍后重试。')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  saving.value = true
  notice.value = ''
  try {
    if (editingID.value) await agentAPI.adminContentPages.update(editingID.value, form.value)
    else await agentAPI.adminContentPages.create(form.value)
    notice.value = editingID.value ? '页面已更新。' : '页面已创建。'
    reset()
    await load()
  } catch (err) {
    notice.value = errorMessage(err, '保存内容页面失败，请检查页面标识是否重复。')
  } finally {
    saving.value = false
  }
}

function requestRemove(item: AgentContentPage): void {
  if (deleting.value) return
  deleteTarget.value = item
}

async function confirmRemove(): Promise<void> {
  const item = deleteTarget.value
  if (!item || deleting.value) return
  deleting.value = true
  deleteTarget.value = null
  notice.value = ''
  try {
    await agentAPI.adminContentPages.delete(item.id)
    if (editingID.value === item.id) reset()
    await load()
  } catch (err) {
    notice.value = errorMessage(err, '删除内容页面失败。')
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  if (props.autoLoad) void load()
})

defineExpose({ load })
</script>

<template>
  <section class="space-y-6" aria-label="租户内容页面">
    <div class="card overflow-hidden">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">内容页面</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">复用主站协议页和自定义页外观，但内容只存放在当前代理站。</p>
      </div>
      <form class="space-y-5 p-6" @submit.prevent="save">
        <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <label class="text-sm font-medium">页面类型
            <AgentSelect v-model="form.kind" :options="kindOptions" class="mt-2" aria-label="选择页面类型" />
          </label>
          <label class="text-sm font-medium">页面标识
            <AgentInput v-model="form.slug" required pattern="[a-z0-9][a-z0-9_-]{0,63}" maxlength="64" class="mt-2 font-mono" placeholder="about-us" />
          </label>
          <label class="text-sm font-medium">发布状态
            <AgentSelect v-model="form.status" :options="statusOptions" class="mt-2" aria-label="选择发布状态" />
          </label>
          <label class="text-sm font-medium">菜单排序
            <AgentInput v-model.number="form.sort_order" type="number" min="-100000" max="100000" class="mt-2" />
          </label>
        </div>
        <label class="block text-sm font-medium">页面标题
          <AgentInput v-model="form.title" required maxlength="160" class="mt-2" placeholder="页面标题" />
        </label>
        <label class="block text-sm font-medium">Markdown 内容
          <AgentTextArea v-model="form.content" required maxlength="100000" rows="14" class="mt-2 min-h-72 font-mono text-sm" placeholder="# 标题&#10;&#10;页面正文" />
        </label>
        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-5 dark:border-dark-700">
          <p class="text-xs text-gray-500 dark:text-dark-400">协议页发布后可通过 /legal/标识 公开访问；自定义页仅登录用户可见。</p>
          <div class="flex gap-2">
            <button v-if="editingID" type="button" class="btn btn-secondary" @click="reset">取消编辑</button>
            <button type="submit" class="btn btn-primary" :disabled="saving"><Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="['mr-2', saving ? 'animate-spin' : '']" />{{ saving ? '保存中…' : editingID ? '保存修改' : '创建页面' }}</button>
          </div>
        </div>
      </form>
    </div>

    <p v-if="notice" role="status" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-200">{{ notice }}</p>
    <div class="card overflow-hidden">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"><h3 class="font-semibold">现有页面</h3></div>
      <p v-if="loading" class="p-6 text-sm text-gray-500">正在加载…</p>
      <p v-else-if="pages.length === 0" class="p-6 text-sm text-gray-500">尚未创建内容页面。</p>
      <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
        <article v-for="item in pages" :key="item.id" class="flex flex-wrap items-center justify-between gap-4 px-6 py-4">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2"><h4 class="font-medium">{{ item.title }}</h4><span class="badge">{{ item.kind === 'legal' ? '公开协议' : '自定义页' }}</span><span class="badge">{{ item.status }}</span></div>
            <p class="mt-1 truncate font-mono text-xs text-gray-500">{{ item.kind === 'legal' ? `/legal/${item.slug}` : `/custom/${item.slug}` }}</p>
          </div>
          <div class="flex gap-2"><button class="btn btn-secondary btn-sm" :disabled="deleting" @click="edit(item)">编辑</button><button class="btn btn-danger btn-sm" :disabled="deleting" @click="requestRemove(item)">删除</button></div>
        </article>
      </div>
    </div>
    <AgentConfirmDialog
      :open="deleteTarget !== null"
      title="删除内容页面"
      :message="deleteTarget ? `确定删除“${deleteTarget.title}”吗？此操作不可撤销。` : '确定删除此内容页面吗？'"
      confirm-label="删除"
      destructive
      @cancel="deleteTarget = null"
      @confirm="confirmRemove"
    />
  </section>
</template>
