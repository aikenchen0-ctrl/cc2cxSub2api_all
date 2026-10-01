<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { applyBranding, applyHomeSettings, applyPageTitle } from '@/agent/branding'
import { errorMessage } from '@/agent/locale'
import AgentSettingsPanel from '@/components/admin/settings/AgentSettingsPanel.vue'
import Icon from '@/components/icons/Icon.vue'

const context = ref<AgentContextResponse | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const brandingName = ref('')
const brandingSiteName = ref('')
const brandingLogo = ref('')
const brandingDocURL = ref('')
const brandingContactInfo = ref('')
const brandingAPIBaseURL = ref('')
const brandingSubtitle = ref('')
const brandingCompactHome = ref(false)
const brandingHomeContent = ref('')

function applyContext(nextContext: AgentContextResponse): void {
  context.value = nextContext
  brandingName.value = nextContext.agent.name
  brandingSiteName.value = nextContext.agent.site_name
  brandingLogo.value = nextContext.agent.site_logo || ''
  brandingDocURL.value = nextContext.agent.doc_url || ''
  brandingContactInfo.value = nextContext.agent.contact_info || ''
  brandingAPIBaseURL.value = nextContext.agent.api_base_url || ''
  brandingSubtitle.value = nextContext.agent.site_subtitle || ''
  brandingCompactHome.value = nextContext.agent.compact_home_enabled === true
  brandingHomeContent.value = nextContext.agent.home_content || ''
}

async function loadContext(): Promise<void> {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    applyContext(await agentAPI.getContext())
  } catch (cause) {
    context.value = null
    error.value = errorMessage(cause, '站点信息加载失败，暂时不能安全修改本站设置。')
  } finally {
    loading.value = false
  }
}

async function saveBranding(): Promise<void> {
  if (!context.value || saving.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const updated = await agentAPI.updateBranding({
      name: brandingName.value.trim(),
      site_name: brandingSiteName.value.trim(),
      site_logo: brandingLogo.value.trim(),
      doc_url: brandingDocURL.value.trim(),
      contact_info: brandingContactInfo.value.trim(),
      api_base_url: brandingAPIBaseURL.value.trim(),
      site_subtitle: brandingSubtitle.value.trim(),
      compact_home_enabled: brandingCompactHome.value,
      home_content: brandingHomeContent.value.trim(),
    })
    context.value.agent.name = updated.name
    context.value.agent.site_name = updated.site_name
    context.value.agent.site_logo = updated.site_logo || undefined
    context.value.agent.doc_url = updated.doc_url || undefined
    context.value.agent.contact_info = updated.contact_info || undefined
    context.value.agent.api_base_url = updated.api_base_url || undefined
    context.value.agent.site_subtitle = updated.site_subtitle
    context.value.agent.compact_home_enabled = updated.compact_home_enabled
    context.value.agent.home_content = updated.home_content
    brandingName.value = updated.name
    brandingSiteName.value = updated.site_name
    brandingLogo.value = updated.site_logo || ''
    brandingDocURL.value = updated.doc_url || ''
    brandingContactInfo.value = updated.contact_info || ''
    brandingAPIBaseURL.value = updated.api_base_url || ''
    brandingSubtitle.value = updated.site_subtitle || ''
    brandingCompactHome.value = updated.compact_home_enabled === true
    brandingHomeContent.value = updated.home_content || ''
    applyBranding(updated.site_name, updated.site_logo, updated.doc_url, updated.contact_info)
    applyHomeSettings(updated)
    applyPageTitle('系统设置')
    notice.value = '本站品牌信息已保存，并已应用到当前界面。'
  } catch (cause) {
    error.value = errorMessage(cause, '更新本站品牌信息失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}

onMounted(() => void loadContext())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">本站管理</p>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">系统设置</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">配置本站名称、标题、Logo、文档入口、客服信息和 API 调用地址。</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.domain || context.agent.site_name || context.agent.name }}</span>
    </header>

    <div v-if="loading" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认站点设置边界…</p>
      </div>
    </div>

    <div v-else-if="!context" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载系统设置</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-emerald-50 p-4 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ notice }}</p>

      <AgentSettingsPanel
        v-model:name="brandingName"
        v-model:site-name="brandingSiteName"
        v-model:site-logo="brandingLogo"
        v-model:doc-url="brandingDocURL"
        v-model:contact-info="brandingContactInfo"
        v-model:api-base-url="brandingAPIBaseURL"
        v-model:site-subtitle="brandingSubtitle"
        v-model:compact-home-enabled="brandingCompactHome"
        v-model:home-content="brandingHomeContent"
        :domain="context.agent.domain"
        :saving="saving"
        @save="saveBranding"
      />
    </template>
  </main>
</template>
