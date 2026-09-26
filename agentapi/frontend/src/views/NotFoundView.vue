<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <main class="mx-auto flex min-h-[calc(100vh-5rem)] max-w-xl flex-col items-center justify-center px-6 text-center">
    <p class="text-7xl font-bold text-primary-600">404</p>
    <h1 class="mt-4 text-2xl font-semibold">页面不存在</h1>
    <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">找不到你要访问的 AgentAPI 页面。</p>
    <button class="mt-6 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white" type="button" @click="router.replace('/home')">返回首页</button>
  </main>
</template>
