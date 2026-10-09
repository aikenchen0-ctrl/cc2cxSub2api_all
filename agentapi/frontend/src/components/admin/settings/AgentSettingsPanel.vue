<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import { routeLocationKey, routerKey } from 'vue-router'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import AgentPaymentSettings from '@/views/admin/settings/AgentPaymentSettings.vue'
import AgentInput from '@/components/common/AgentInput.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  name: string
  siteName: string
  siteLogo: string
  docUrl: string
  contactInfo: string
  apiBaseUrl: string
  siteSubtitle: string
  compactHomeEnabled: boolean
  homeContent: string
  domain?: string
  saving?: boolean
}>()

const emit = defineEmits<{
  'update:name': [value: string]
  'update:siteName': [value: string]
  'update:siteLogo': [value: string]
  'update:docUrl': [value: string]
  'update:contactInfo': [value: string]
  'update:apiBaseUrl': [value: string]
  'update:siteSubtitle': [value: string]
  'update:compactHomeEnabled': [value: boolean]
  'update:homeContent': [value: string]
  save: []
  uploadLogo: [file: File]
}>()

const route = inject(routeLocationKey, { query: {} } as unknown as RouteLocationNormalizedLoaded)
const router = inject(routerKey, null)
type SettingsTab = 'branding' | 'payment'
const activeTab = ref<SettingsTab>('branding')
watch(() => route.query?.tab, value => {
  activeTab.value = value === 'payment' ? value : 'branding'
}, { immediate: true })
function selectTab(tab: SettingsTab) {
  activeTab.value = tab
  if (router) void router.replace({ query: { ...route.query, tab } })
}
const tabs = [
  { key: 'branding' as const, label: '站点设置', icon: 'home' as const },

  { key: 'payment' as const, label: '支付配置', icon: 'creditCard' as const },
]
const logoFailed = ref(false)
const logoInput = ref<HTMLInputElement | null>(null)
const agentName = computed({ get: () => props.name, set: value => emit('update:name', value) })
const siteName = computed({ get: () => props.siteName, set: value => emit('update:siteName', value) })
const siteLogo = computed({ get: () => props.siteLogo, set: value => emit('update:siteLogo', value) })
const docUrl = computed({ get: () => props.docUrl, set: value => emit('update:docUrl', value) })
const contactInfo = computed({ get: () => props.contactInfo, set: value => emit('update:contactInfo', value) })
const apiBaseUrl = computed({ get: () => props.apiBaseUrl, set: value => emit('update:apiBaseUrl', value) })
const siteSubtitle = computed({ get: () => props.siteSubtitle, set: value => emit('update:siteSubtitle', value) })
const compactHomeEnabled = computed({ get: () => props.compactHomeEnabled, set: value => emit('update:compactHomeEnabled', value) })
const homeContent = computed({ get: () => props.homeContent, set: value => emit('update:homeContent', value) })
const previewName = computed(() => agentName.value.trim() || siteName.value.trim() || 'AgentAPI')
const previewInitial = computed(() => previewName.value.slice(0, 1).toUpperCase())

watch(() => props.siteLogo, () => { logoFailed.value = false })

function chooseLogo(event: Event): void {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  emit('uploadLogo', file)
  if (logoInput.value) logoInput.value.value = ''
}
</script>

<template>
  <section class="space-y-6" aria-label="站点系统设置">
    <div class="settings-tabs-shell">
      <nav class="settings-tabs-scroll" role="tablist" aria-label="系统设置">
        <div class="settings-tabs">
          <button
            v-for="tab in tabs"
            :id="`settings-tab-${tab.key}`"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            :class="['settings-tab', activeTab === tab.key && 'settings-tab-active']"
            @click="selectTab(tab.key)"
          >
            <span class="settings-tab-icon"><Icon :name="tab.icon" size="sm" /></span>
            <span class="settings-tab-label">{{ tab.label }}</span>
          </button>
        </div>
      </nav>
    </div>

    <div v-if="activeTab === 'branding'" class="space-y-6">
      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">品牌信息</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">自定义本站的名称和 Logo。修改只应用于当前站点。</p>
        </div>

        <div class="grid gap-6 p-6 xl:grid-cols-[minmax(0,1fr)_18rem]">
          <form class="space-y-5" @submit.prevent="emit('save')">
            <div class="grid gap-5 md:grid-cols-2">
              <div>
                <label for="agent-name" class="input-label">站点名称</label>
                <AgentInput id="agent-name" v-model="agentName" class="mt-2" maxlength="100" type="text" placeholder="用于站长后台识别" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">用于站点管理页面和本站记录。</p>
              </div>
              <div>
                <label for="site-name" class="input-label">站点名称</label>
                <AgentInput id="site-name" v-model="siteName" class="mt-2" maxlength="160" type="text" placeholder="显示在首页、侧栏和浏览器标题中" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">保存后会立即应用到本站公开页面和登录界面。</p>
              </div>
            </div>

            <div>
              <label for="site-logo-file" class="input-label">上传 Logo</label>
              <div class="mt-2 flex flex-wrap items-center gap-3">
                <input id="site-logo-file" ref="logoInput" class="block text-sm text-gray-600 file:mr-3 file:rounded-lg file:border-0 file:bg-primary-50 file:px-3 file:py-2 file:text-sm file:font-medium file:text-primary-700" type="file" accept="image/png,image/jpeg,image/webp,image/gif" :disabled="saving" @change="chooseLogo">
              </div>
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">支持 PNG、JPEG、WEBP、GIF，最大 1 MB。上传后立即应用到本站。</p>
              <label for="site-logo" class="input-label mt-4 block">或填写 Logo 网址</label>
              <AgentInput id="site-logo" v-model="siteLogo" class="mt-2 font-mono text-sm" maxlength="2048" type="text" placeholder="/logo.svg 或 https://…" />
            </div>

            <div>
              <label for="doc-url" class="input-label">文档网址</label>
              <AgentInput id="doc-url" v-model="docUrl" class="mt-2 font-mono text-sm" maxlength="2048" type="url" placeholder="https://docs.example.com" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">留空时不显示文档入口；保存后仅应用于本站首页和顶部栏。</p>
            </div>

            <div>
              <label for="contact-info" class="input-label">客服联系方式</label>
              <AgentInput id="contact-info" v-model="contactInfo" class="mt-2" maxlength="300" type="text" placeholder="例如：support@example.com 或企业微信号" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">留空时不显示；保存后仅作为本站用户菜单中的纯文本展示。</p>
            </div>

            <div>
              <label for="api-base-url" class="input-label">API 调用地址</label>
              <AgentInput id="api-base-url" v-model="apiBaseUrl" class="mt-2 font-mono text-sm" maxlength="2048" type="url" placeholder="https://api.example.com/v1" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">用户在 OpenAI 兼容客户端中填写的 Base URL。该地址会显示在 API 密钥面板并支持一键复制。</p>
            </div>

            <div class="space-y-5 border-t border-gray-100 pt-5 dark:border-dark-700">
              <h3 class="font-semibold text-gray-900 dark:text-white">首页设置</h3>
              <div>
                <label for="site-subtitle" class="input-label">站点副标题</label>
                <textarea id="site-subtitle" v-model="siteSubtitle" rows="3" maxlength="500" class="input mt-2 w-full" placeholder="AI API Gateway Platform"></textarea>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">支持多行文字，显示在粒子首页和简洁首页的站点名称下方。</p>
              </div>
              <label for="compact-home-enabled" class="flex items-center justify-between gap-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
                <span><span class="block text-sm font-medium text-gray-900 dark:text-white">简洁首页</span><span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">仅展示站点品牌和常用入口；关闭后显示粒子首页。</span></span>
                <input id="compact-home-enabled" v-model="compactHomeEnabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              </label>
              <div>
                <label for="home-content" class="input-label">自定义首页内容</label>
                <textarea id="home-content" v-model="homeContent" rows="8" maxlength="100000" class="input mt-2 w-full font-mono text-sm" placeholder="https://example.com 或完整 HTML 内容"></textarea>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">填写后优先全屏展示；清空后使用上方选择的首页模式。支持 HTTP/HTTPS 网页或 HTML，外部网页需允许嵌入。HTML 在隔离框架中显示，可包含样式，不运行脚本；返回本站的链接可使用 target="_top"。</p>
              </div>
            </div>

            <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-5 dark:border-dark-700">
              <p class="text-xs text-gray-500 dark:text-dark-400">这里配置向用户展示的本站 API 调用地址。</p>
              <button class="btn btn-primary" type="button" :disabled="saving" @click="emit('save')">
                <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="['mr-2', saving ? 'animate-spin' : '']" />
                {{ saving ? '正在保存…' : '保存品牌信息' }}
              </button>
            </div>
          </form>

          <aside class="rounded-xl border border-gray-200 bg-gray-50 p-5 dark:border-dark-600 dark:bg-dark-700/60">
            <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">品牌预览</p>
            <div class="mt-4 rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
              <div class="flex items-center gap-3">
                <img v-if="siteLogo && !logoFailed" :src="siteLogo" alt="站点 Logo 预览" class="h-10 w-10 rounded-lg object-contain" @error="logoFailed = true">
                <div v-else class="flex h-10 w-10 items-center justify-center rounded-lg bg-primary-600 font-semibold text-white">{{ previewInitial }}</div>
                <div class="min-w-0">
                  <p class="truncate font-semibold text-gray-900 dark:text-white">{{ previewName }}</p>
                  <p class="truncate text-xs text-gray-500 dark:text-dark-400">{{ domain || '站点域名待配置' }}</p>
                </div>
              </div>
              <div class="mt-4 h-2 rounded-full bg-gray-100 dark:bg-dark-600"><div class="h-2 w-2/3 rounded-full bg-primary-500"></div></div>
              <div class="mt-3 grid grid-cols-3 gap-2"><span v-for="index in 3" :key="index" class="h-12 rounded-lg bg-gray-100 dark:bg-dark-700"></span></div>
            </div>
            <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-dark-400">预览用于确认品牌外观，保存后会应用到本站页面。</p>
          </aside>
        </div>
      </div>
    </div>


    <AgentPaymentSettings v-else-if="activeTab === 'payment'" />
  </section>
</template>

<style scoped>
.settings-tabs-shell {
  @apply sticky z-20 rounded-2xl border border-gray-200/80 bg-white/90 p-1.5 backdrop-blur-xl dark:border-dark-700/80 dark:bg-dark-900/80;
  top: 4.75rem;
  box-shadow: 0 12px 28px rgb(15 23 42 / 0.07), 0 1px 0 rgb(255 255 255 / 0.9) inset;
}
.settings-tabs-scroll {
  @apply overflow-x-auto;
  scrollbar-width: none;
}
.settings-tabs-scroll::-webkit-scrollbar { display: none; }
.settings-tabs { @apply flex min-w-max items-center gap-1 md:min-w-full; }
.settings-tab {
  @apply relative isolate flex h-10 min-w-[6.75rem] shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-transparent px-3 text-sm font-medium text-gray-600 outline-none transition-colors dark:text-dark-300 md:min-w-0 md:flex-1;
}
.settings-tab::before {
  content: "";
  @apply absolute inset-0 -z-10 rounded-xl opacity-0 transition-opacity;
  background: linear-gradient(135deg, rgb(240 253 250 / 0.95), rgb(204 251 241 / 0.75));
}
.settings-tab:hover::before,
.settings-tab:focus-visible::before { opacity: 1; }
.settings-tab:focus-visible { @apply ring-2 ring-primary-500/40 ring-offset-2 ring-offset-white dark:ring-offset-dark-900; }
.settings-tab-active {
  @apply border-primary-200/80 bg-white text-primary-700 shadow-sm dark:border-primary-400/30 dark:bg-dark-800 dark:text-primary-200;
}
.settings-tab-active::before { opacity: 0; }
.settings-tab-active::after {
  position: absolute;
  right: 0.75rem;
  bottom: 0.25rem;
  left: 0.75rem;
  height: 2px;
  border-radius: 9999px;
  content: "";
  background: linear-gradient(90deg, #14b8a6, #0d9488);
}
.settings-tab-icon { @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors dark:text-dark-400; }
.settings-tab:hover .settings-tab-icon { @apply text-primary-600 dark:text-primary-300; }
.settings-tab-active .settings-tab-icon { @apply bg-primary-50 text-primary-600 dark:bg-primary-400/10 dark:text-primary-300; }
.settings-tab-label { @apply min-w-0 truncate leading-none; }
</style>

<style>
.dark .settings-tabs-shell {
  box-shadow: 0 16px 36px rgb(0 0 0 / 0.28), 0 1px 0 rgb(255 255 255 / 0.06) inset;
}
.dark .settings-tab::before {
  background: linear-gradient(135deg, rgb(19 78 74 / 0.45), rgb(15 23 42 / 0.8));
}
</style>
