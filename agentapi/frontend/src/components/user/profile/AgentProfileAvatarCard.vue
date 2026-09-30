<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { agentAPI, type AgentProfile } from '@/agent/api'
import { errorMessage } from '@/agent/client'

const props = defineProps<{
  profile: AgentProfile
}>()

const emit = defineEmits<{
  updated: [profile: AgentProfile]
}>()

const targetAvatarUploadBytes = 20 * 1024
const avatarScaleSteps = [1, 0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarQualitySteps = [0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarDraft = ref('')
const saving = ref(false)
const error = ref('')
const notice = ref('')

const displayName = computed(() => props.profile.username?.trim() || props.profile.email?.trim() || '用户')
const avatarInitial = computed(() => Array.from(displayName.value)[0]?.toUpperCase() || 'U')
const avatarPreviewUrl = computed(() => avatarDraft.value.trim() || props.profile.avatar_url?.trim() || '')

function readableError(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message.trim()) return err.message
  return errorMessage(err, fallback)
}

watch(
  () => props.profile.avatar_url,
  () => {
    avatarDraft.value = ''
  },
)

function normalizeUploadedAvatar(value: string): string | null {
  const normalized = value.trim()
  if (!normalized || !/^data:image\/(?:png|jpe?g|gif|webp);base64,/i.test(normalized)) {
    error.value = '请选择 PNG、JPEG、WebP 或 GIF 图片文件。'
    return null
  }
  return normalized
}

function readFileAsDataURL(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error ?? new Error('读取头像文件失败。'))
    reader.readAsDataURL(file)
  })
}

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('无法读取所选图片。'))
    image.src = dataURL
  })
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error('压缩头像失败。'))
        return
      }
      resolve(blob)
    }, type, quality)
  })
}

async function compressAvatarFile(file: File): Promise<File> {
  const image = await loadImage(await readFileAsDataURL(file))
  const canvas = document.createElement('canvas')
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器无法压缩头像。')

  for (const scale of avatarScaleSteps) {
    const width = Math.max(1, Math.round(image.naturalWidth * scale))
    const height = Math.max(1, Math.round(image.naturalHeight * scale))
    canvas.width = width
    canvas.height = height
    context.clearRect(0, 0, width, height)
    context.drawImage(image, 0, 0, width, height)
    for (const quality of avatarQualitySteps) {
      const blob = await canvasToBlob(canvas, 'image/webp', quality)
      if (blob.size <= targetAvatarUploadBytes) {
        const name = file.name.replace(/\.[^.]+$/, '') || 'avatar'
        return new File([blob], `${name}.webp`, { type: 'image/webp' })
      }
    }
  }
  throw new Error('无法将头像压缩到 20KB 以内，请选择更简单或更小的图片。')
}

async function prepareAvatarUpload(file: File): Promise<File> {
  if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(file.type.toLowerCase())) {
    throw new Error('请选择 PNG、JPEG、WebP 或 GIF 图片文件。')
  }
  if (file.type.toLowerCase() === 'image/gif') {
    if (file.size > targetAvatarUploadBytes) throw new Error('GIF 头像必须小于或等于 20KB。')
    return file
  }
  if (file.size <= targetAvatarUploadBytes) return file
  return compressAvatarFile(file)
}

async function handleFileChange(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) input.value = ''
  if (!file) return
  error.value = ''
  notice.value = ''
  try {
    const normalized = normalizeUploadedAvatar(await readFileAsDataURL(await prepareAvatarUpload(file)))
    if (normalized) {
      avatarDraft.value = normalized
      notice.value = '头像已准备好，保存后同步到主站账号。'
    }
  } catch (err) {
    error.value = readableError(err, '处理头像失败，请重新选择图片。')
  }
}

async function saveAvatar(): Promise<void> {
  if (saving.value) return
  const normalized = normalizeUploadedAvatar(avatarDraft.value)
  if (!normalized) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const updated = await agentAPI.profile.avatar.update(normalized)
    avatarDraft.value = ''
    emit('updated', updated)
    notice.value = '头像已更新。'
  } catch (err) {
    error.value = readableError(err, '保存头像失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}

async function deleteAvatar(): Promise<void> {
  if (saving.value) return
  if (!avatarDraft.value.trim() && !props.profile.avatar_url?.trim()) {
    error.value = '当前没有可删除的头像。'
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const updated = await agentAPI.profile.avatar.update('')
    avatarDraft.value = ''
    emit('updated', updated)
    notice.value = '头像已删除。'
  } catch (err) {
    error.value = readableError(err, '删除头像失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section data-testid="profile-avatar-card" class="space-y-4 border-b border-gray-200 pb-5 dark:border-dark-700">
    <div class="flex flex-col gap-5 sm:flex-row sm:items-start">
      <div class="flex h-20 w-20 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20">
        <img
          v-if="avatarPreviewUrl"
          data-testid="profile-avatar-preview"
          :src="avatarPreviewUrl"
          :alt="displayName"
          class="h-full w-full object-cover"
        >
        <span v-else>{{ avatarInitial }}</span>
      </div>

      <div class="min-w-0 flex-1 space-y-3">
        <div>
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">个人头像</h4>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">上传后会尽量压缩到 20KB，并同步保存到 Sub2API 主站账号。</p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <label class="btn btn-secondary btn-sm cursor-pointer">
            <input
              data-testid="profile-avatar-file-input"
              type="file"
              accept="image/png,image/jpeg,image/gif,image/webp"
              class="hidden"
              @change="handleFileChange"
            >
            选择图片
          </label>
          <button data-testid="profile-avatar-save" type="button" class="btn btn-primary btn-sm" :disabled="saving || !avatarDraft" @click="saveAvatar">
            {{ saving ? '处理中…' : '保存头像' }}
          </button>
          <button data-testid="profile-avatar-delete" type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="deleteAvatar">
            删除头像
          </button>
        </div>
      </div>
    </div>
    <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200">{{ error }}</p>
    <p v-if="notice" role="status" class="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200">{{ notice }}</p>
  </section>
</template>
