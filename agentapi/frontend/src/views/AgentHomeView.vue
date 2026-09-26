<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { agentAPI, type AgentContextResponse } from '@/agent/api'

const context = ref<AgentContextResponse | null>(null)
onMounted(async () => {
  try {
    context.value = await agentAPI.getContext()
  } catch {
    // Branding remains usable even if the optional context request is offline.
  }
})
</script>

<template>
  <main class="mx-auto flex min-h-screen max-w-5xl flex-col justify-center gap-8 p-6">
    <section class="rounded-2xl border bg-white p-8 shadow-sm dark:border-slate-700 dark:bg-slate-900">
      <p class="text-sm font-medium text-blue-600">{{ context?.agent.site_name || 'AgentAPI' }}</p>
      <h1 class="mt-3 text-4xl font-semibold text-slate-900 dark:text-white">连接 Sub2API 主站的轻量模型代理服务</h1>
      <p class="mt-4 max-w-2xl leading-7 text-slate-600 dark:text-slate-300">用户通过此代理站在主站注册。代理站仅保存本地关联信息和子余额；模型请求由已配置的代理站主账户在主站侧计费。</p>
      <div class="mt-8 flex flex-wrap gap-3">
        <router-link class="rounded-lg bg-blue-600 px-5 py-3 text-sm font-medium text-white" to="/login">登录</router-link>
        <router-link class="rounded-lg border px-5 py-3 text-sm font-medium" to="/register">注册账号</router-link>
      </div>
    </section>
    <p class="text-center text-sm text-slate-500">不收取佣金，不复制上游账户，管理员凭据也不会暴露在浏览器中。</p>
  </main>
</template>
