<template>
  <div v-if="turnstileEnabled && turnstileSiteKey" class="relative min-h-[65px] w-full">
    <div v-if="loading" class="absolute inset-0 flex items-center justify-center rounded-lg border border-gray-200 bg-gray-50 text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-700">正在加载安全验证…</div>
    <div ref="turnstileContainer" class="min-h-[65px] w-full" />
  </div>
  <div v-else-if="tencentEnabled && tencentAppId" class="space-y-2">
    <div ref="tencentContainer" class="flex min-h-[1px] w-full justify-center" />
    <button type="button" class="btn btn-secondary w-full" :disabled="loading" @click="void verifyAction()">
      {{ loading ? '正在加载安全验证…' : '点击完成人机验证' }}
    </button>
  </div>
  <div v-else-if="aliyunEnabled && aliyunSceneId && aliyunPrefix" class="w-full">
    <button :id="aliyunButtonId" type="button" class="btn btn-secondary w-full" :disabled="loading">
      {{ loading ? '正在加载安全验证…' : '点击完成人机验证' }}
    </button>
    <div :id="aliyunElementId" />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

interface TurnstileAPI {
  render(container: HTMLElement, options: Record<string, unknown>): string
  reset(widgetId?: string): void
  remove(widgetId?: string): void
}

interface TencentResult { ret: number; ticket?: string; randstr?: string; errorCode?: number }
interface TencentInstance { show(): void; destroy(): void }
type TencentConstructor = {
  new (appId: string, callback: (result: TencentResult) => void, options?: Record<string, unknown>): TencentInstance
  new (container: HTMLElement, appId: string, callback: (result: TencentResult) => void, options?: Record<string, unknown>): TencentInstance
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
    TencentCaptcha?: TencentConstructor
    TCaptchaGlobal?: boolean
    initAliyunCaptcha?: (options: Record<string, unknown>) => void
    AliyunCaptchaConfig?: { region: string; prefix: string }
  }
}

const props = withDefaults(defineProps<{
  turnstileEnabled?: boolean
  turnstileSiteKey?: string
  tencentEnabled?: boolean
  tencentAppId?: string
  tencentRegion?: string
  aliyunEnabled?: boolean
  aliyunSceneId?: string
  aliyunPrefix?: string
  aliyunRegion?: string
}>(), {
  turnstileEnabled: false, turnstileSiteKey: '', tencentEnabled: false, tencentAppId: '',
  tencentRegion: 'cn', aliyunEnabled: false, aliyunSceneId: '', aliyunPrefix: '', aliyunRegion: 'cn',
})

const emit = defineEmits<{
  verify: [token: string, randstr: string]
  expire: []
  error: []
}>()

const turnstileContainer = ref<HTMLElement | null>(null)
const tencentContainer = ref<HTMLElement | null>(null)
const loading = ref(false)
const uid = Math.random().toString(36).slice(2, 10)
const aliyunButtonId = `agent-aliyun-button-${uid}`
const aliyunElementId = `agent-aliyun-element-${uid}`
let turnstileWidget = ''
let tencentInstance: TencentInstance | null = null
let aliyunReady: Promise<void> | null = null
let aliyunProof = ''
let aliyunResolve: ((value: { token: string; randstr: string } | null) => void) | null = null

function loadScript(src: string, ready: () => boolean): Promise<void> {
  if (ready()) return Promise.resolve()
  return new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${src}"]`)
    const done = (): void => ready() ? resolve() : reject(new Error('captcha SDK unavailable'))
    if (existing) {
      existing.addEventListener('load', done, { once: true })
      existing.addEventListener('error', () => reject(new Error('captcha SDK failed to load')), { once: true })
      return
    }
    const script = document.createElement('script')
    script.src = src
    script.async = true
    script.onload = done
    script.onerror = () => reject(new Error('captcha SDK failed to load'))
    document.head.appendChild(script)
  })
}

async function mountTurnstile(): Promise<void> {
  if (!props.turnstileEnabled || !props.turnstileSiteKey || !turnstileContainer.value) return
  loading.value = true
  try {
    await loadScript('https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit', () => Boolean(window.turnstile))
    turnstileWidget = window.turnstile!.render(turnstileContainer.value, {
      sitekey: props.turnstileSiteKey,
      theme: 'auto', size: 'flexible',
      callback: (token: string) => emit('verify', token, ''),
      'expired-callback': () => emit('expire'),
      'error-callback': () => emit('error'),
    })
  } catch {
    emit('error')
  } finally {
    loading.value = false
  }
}

async function verifyTencent(): Promise<{ token: string; randstr: string } | null> {
  const international = props.tencentRegion === 'intl'
  const src = international
    ? 'https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js'
    : 'https://turing.captcha.qcloud.com/TJCaptcha.js'
  loading.value = true
  try {
    await loadScript(src, () => Boolean(window.TencentCaptcha))
    return await new Promise((resolve, reject) => {
      const callback = (result: TencentResult): void => {
        if (result.ret === 2) { resolve(null); return }
        const token = result.ticket?.trim() || ''
        const randstr = result.randstr?.trim() || ''
        if (!token || !randstr || token.startsWith('trerror_') || result.errorCode !== undefined) {
          reject(new Error('Tencent captcha verification failed'))
          return
        }
        emit('verify', token, randstr)
        resolve({ token, randstr })
      }
      tencentInstance?.destroy()
      tencentInstance = international && tencentContainer.value
        ? new window.TencentCaptcha!(tencentContainer.value, props.tencentAppId, callback, { type: 'popup', userLanguage: 'zh-cn' })
        : new window.TencentCaptcha!(props.tencentAppId, callback, { userLanguage: 'zh-cn' })
      tencentInstance.show()
    })
  } finally {
    loading.value = false
  }
}

async function ensureAliyun(): Promise<void> {
  if (aliyunReady) return aliyunReady
  loading.value = true
  window.AliyunCaptchaConfig = { region: props.aliyunRegion === 'sgp' ? 'sgp' : 'cn', prefix: props.aliyunPrefix }
  aliyunReady = loadScript('https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js', () => Boolean(window.initAliyunCaptcha)).then(() => {
    window.initAliyunCaptcha!({
      SceneId: props.aliyunSceneId, prefix: props.aliyunPrefix, mode: 'popup',
      element: `#${aliyunElementId}`, button: `#${aliyunButtonId}`,
      captchaVerifyCallback: (value: unknown) => {
        const token = String(value || '').trim()
        aliyunProof = token
        if (token) emit('verify', token, '')
        aliyunResolve?.(token ? { token, randstr: '' } : null)
        aliyunResolve = null
        return { captchaResult: Boolean(token) }
      },
      onBizResultCallback: () => undefined, getInstance: () => undefined,
      slideStyle: { width: 360, height: 40 }, language: 'cn',
    })
  }).finally(() => { loading.value = false })
  return aliyunReady
}

async function verifyAction(): Promise<{ token: string; randstr: string } | null> {
  try {
    if (props.tencentEnabled && props.tencentAppId) return await verifyTencent()
    if (props.aliyunEnabled && props.aliyunSceneId && props.aliyunPrefix) {
      if (aliyunProof) return { token: aliyunProof, randstr: '' }
      await ensureAliyun()
      await nextTick()
      return await new Promise((resolve) => {
        aliyunResolve = resolve
        document.getElementById(aliyunButtonId)?.click()
      })
    }
  } catch {
    emit('error')
  }
  return null
}

function reset(): void {
  if (window.turnstile && turnstileWidget) window.turnstile.reset(turnstileWidget)
  tencentInstance?.destroy()
  tencentInstance = null
  aliyunProof = ''
  aliyunResolve?.(null)
  aliyunResolve = null
}

onMounted(() => { void mountTurnstile(); if (props.aliyunEnabled) void ensureAliyun() })
onBeforeUnmount(() => {
  if (window.turnstile && turnstileWidget) window.turnstile.remove(turnstileWidget)
  tencentInstance?.destroy()
  aliyunResolve?.(null)
})
defineExpose({ reset, verifyAction })
</script>
