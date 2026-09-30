<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AgentInput from '@/components/common/AgentInput.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  name: string
  siteName: string
  siteLogo: string
  docUrl: string
  contactInfo: string
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
  'update:siteSubtitle': [value: string]
  'update:compactHomeEnabled': [value: boolean]
  'update:homeContent': [value: string]
  save: []
}>()

const activeTab = ref<'branding' | 'boundary'>('branding')
const logoFailed = ref(false)
const agentName = computed({ get: () => props.name, set: value => emit('update:name', value) })
const siteName = computed({ get: () => props.siteName, set: value => emit('update:siteName', value) })
const siteLogo = computed({ get: () => props.siteLogo, set: value => emit('update:siteLogo', value) })
const docUrl = computed({ get: () => props.docUrl, set: value => emit('update:docUrl', value) })
const contactInfo = computed({ get: () => props.contactInfo, set: value => emit('update:contactInfo', value) })
const siteSubtitle = computed({ get: () => props.siteSubtitle, set: value => emit('update:siteSubtitle', value) })
const compactHomeEnabled = computed({ get: () => props.compactHomeEnabled, set: value => emit('update:compactHomeEnabled', value) })
const homeContent = computed({ get: () => props.homeContent, set: value => emit('update:homeContent', value) })
const previewName = computed(() => agentName.value.trim() || siteName.value.trim() || 'AgentAPI')
const previewInitial = computed(() => previewName.value.slice(0, 1).toUpperCase())

watch(() => props.siteLogo, () => { logoFailed.value = false })
</script>

<template>
  <section class="grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]" aria-label="代理站系统设置">
    <nav class="h-fit rounded-xl border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-800" aria-label="设置分类">
      <button
        type="button"
        :class="['flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm font-medium transition-colors', activeTab === 'branding' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700']"
        @click="activeTab = 'branding'"
      >
        <Icon name="home" size="sm" />站点设置
      </button>
      <button
        type="button"
        :class="['mt-1 flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm font-medium transition-colors', activeTab === 'boundary' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700']"
        @click="activeTab = 'boundary'"
      >
        <Icon name="shield" size="sm" />数据边界
      </button>
    </nav>

    <div v-if="activeTab === 'branding'" class="space-y-6">
      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">品牌信息</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">自定义本代理站的名称和 Logo。修改只保存在当前代理站租户内。</p>
        </div>

        <div class="grid gap-6 p-6 xl:grid-cols-[minmax(0,1fr)_18rem]">
          <form class="space-y-5" @submit.prevent="emit('save')">
            <div class="grid gap-5 md:grid-cols-2">
              <div>
                <label for="agent-name" class="input-label">代理站名称</label>
                <AgentInput id="agent-name" v-model="agentName" class="mt-2" maxlength="100" type="text" placeholder="用于站长后台识别" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">用于代理站管理页面和审计记录，不会修改主站名称。</p>
              </div>
              <div>
                <label for="site-name" class="input-label">站点名称</label>
                <AgentInput id="site-name" v-model="siteName" class="mt-2" maxlength="160" type="text" placeholder="显示在首页、侧栏和浏览器标题中" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">保存后会立即应用到本站公开页面和登录界面。</p>
              </div>
            </div>

            <div>
              <label for="site-logo" class="input-label">Logo 图片网址或站内路径</label>
              <AgentInput id="site-logo" v-model="siteLogo" class="mt-2 font-mono text-sm" maxlength="2048" type="text" placeholder="/logo.svg 或 https://…" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">建议使用正方形 SVG/PNG；留空时显示站点名称首字。</p>
            </div>

            <div>
              <label for="doc-url" class="input-label">文档网址</label>
              <AgentInput id="doc-url" v-model="docUrl" class="mt-2 font-mono text-sm" maxlength="2048" type="url" placeholder="https://docs.example.com" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">留空时不显示文档入口；保存后仅应用于当前代理站首页和顶部栏。</p>
            </div>

            <div>
              <label for="contact-info" class="input-label">客服联系方式</label>
              <AgentInput id="contact-info" v-model="contactInfo" class="mt-2" maxlength="300" type="text" placeholder="例如：support@example.com 或企业微信号" />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">留空时不显示；保存后仅作为当前代理站用户菜单中的纯文本展示。</p>
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
              <p class="text-xs text-gray-500 dark:text-dark-400">计费身份、代理站域名和主站上游地址由系统配置，不能在此更改。</p>
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
                  <p class="truncate text-xs text-gray-500 dark:text-dark-400">{{ domain || '代理站域名待配置' }}</p>
                </div>
              </div>
              <div class="mt-4 h-2 rounded-full bg-gray-100 dark:bg-dark-600"><div class="h-2 w-2/3 rounded-full bg-primary-500"></div></div>
              <div class="mt-3 grid grid-cols-3 gap-2"><span v-for="index in 3" :key="index" class="h-12 rounded-lg bg-gray-100 dark:bg-dark-700"></span></div>
            </div>
            <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-dark-400">预览只展示品牌外观。主页、菜单栏与内容面板继续复用 Sub2API 的界面结构。</p>
          </aside>
        </div>
      </div>
    </div>

    <div v-else class="space-y-6">
      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">数据与权限边界</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">代理站复用主站能力，但代理站管理员只能管理当前租户的数据。</p>
        </div>
        <div class="grid gap-4 p-6 md:grid-cols-2">
          <div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/60 dark:bg-emerald-950/20">
            <div class="flex items-center gap-2 font-medium text-emerald-800 dark:text-emerald-200"><Icon name="checkCircle" size="sm" />本站可管理</div>
            <ul class="mt-3 space-y-2 text-sm text-emerald-800/90 dark:text-emerald-200/90">
              <li>本站品牌、Logo、文档入口、客服信息与模型允许列表</li>
              <li>本站用户归属与本地访问状态</li>
              <li>本站用户的代理 API Key 与用量视图</li>
            </ul>
          </div>
          <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/60 dark:bg-amber-950/20">
            <div class="flex items-center gap-2 font-medium text-amber-800 dark:text-amber-200"><Icon name="lock" size="sm" />主站保持隔离</div>
            <ul class="mt-3 space-y-2 text-sm text-amber-800/90 dark:text-amber-200/90">
              <li>不能修改主站全局设置或其他代理站配置</li>
              <li>不能读取主站管理员 Key、JWT 或内部凭证</li>
              <li>用户余额与实际计费记录始终以主站为准</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
