<script setup lang="ts">
import AgentAnnouncementsPanel from '@/components/admin/announcements/AgentAnnouncementsPanel.vue'
</script>

<template>
  <main class="agent-page">
    <header><p class="text-sm font-medium text-primary-600 dark:text-primary-400">站点内容</p><h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">公告管理</h1><p class="mt-2 text-sm text-gray-500 dark:text-dark-400">创建、预览和维护向本站用户展示的公告。</p></header>
    <AgentAnnouncementsPanel />
  </main>
</template>
