<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'
import { errorMessage } from '@/agent/locale'
import AgentAffiliateRecordsPanel from '@/components/admin/affiliates/AgentAffiliateRecordsPanel.vue'
import Icon from '@/components/icons/Icon.vue'

type RecordType = 'invites' | 'rebates' | 'transfers'

const props = defineProps<{ type: RecordType }>()
const context = ref<AgentContextResponse | null>(null)
const loading = ref(true)
const error = ref('')

const page = computed(() => ({
  invites: {
    eyebrow: '用户增长',
    title: '推广邀请记录',
    description: '查看当前代理站用户之间已被服务端确认的邀请关系和累计返利事实。',
    icon: 'users' as const,
  },
  rebates: {
    eyebrow: '返利账本',
    title: '推广返利记录',
    description: '查看当前代理站邀请关系产生的主站订单、支付金额和权威返利记录。',
    icon: 'gift' as const,
  },
  transfers: {
    eyebrow: '余额划转',
    title: '推广划转记录',
    description: '查看当前代理站用户已经在 Sub2API 主站完成的返利划转及余额快照。',
    icon: 'swap' as const,
  },
})[props.type])

async function loadContext(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    context.value = await agentAPI.getContext()
  } catch (cause) {
    context.value = null
    error.value = errorMessage(cause, '代理站信息加载失败，暂时不能安全查看推广记录。')
  } finally {
    loading.value = false
  }
}

onMounted(() => void loadContext())
</script>

<template>
  <main class="mx-auto max-w-7xl space-y-6 p-6 pb-12">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <div class="flex items-center gap-2 text-sm font-medium text-primary-600 dark:text-primary-400">
          <Icon :name="page.icon" size="sm" />{{ page.eyebrow }}
        </div>
        <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{{ page.title }}</h1>
        <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ page.description }}</p>
      </div>
      <span v-if="context" class="badge badge-gray">{{ context.agent.site_name || context.agent.name }}</span>
    </header>

    <nav class="card flex flex-wrap gap-2 p-2" aria-label="推广记录分类">
      <RouterLink
        v-for="item in [
          { type: 'invites', path: '/admin/affiliates/invites', label: '邀请记录' },
          { type: 'rebates', path: '/admin/affiliates/rebates', label: '返利记录' },
          { type: 'transfers', path: '/admin/affiliates/transfers', label: '划转记录' },
        ]"
        :key="item.type"
        :to="item.path"
        :class="['rounded-lg px-4 py-2 text-sm font-medium transition-colors', props.type === item.type ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700']"
      >{{ item.label }}</RouterLink>
    </nav>

    <div v-if="loading" class="card flex min-h-48 items-center justify-center" role="status">
      <div class="text-center text-sm text-gray-500">
        <Icon name="refresh" size="lg" class="mx-auto animate-spin text-primary-500" />
        <p class="mt-3">正在确认代理站推广数据边界…</p>
      </div>
    </div>

    <div v-else-if="error" role="alert" class="card border-red-200 p-6 dark:border-red-900">
      <p class="font-medium text-red-700 dark:text-red-300">无法加载{{ page.title }}</p>
      <p class="mt-2 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="loadContext"><Icon name="refresh" size="sm" class="mr-2" />重试</button>
    </div>

    <template v-else-if="context">
      <AgentAffiliateRecordsPanel :type="props.type" />

      <section class="card border-dashed p-5">
        <div class="flex gap-3">
          <Icon name="shield" size="lg" class="mt-0.5 shrink-0 text-primary-500" />
          <div>
            <h2 class="font-medium text-gray-900 dark:text-white">数据与操作边界</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">邀请、订单、返利和划转事实均来自 Sub2API 主站，但服务端只聚合当前代理站登记用户，并隐藏跨代理站关系。站长不能赠送返利、修改比例、重置额度、划转用户余额或操作主站全局推广配置。</p>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>
