<template>
  <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-900 sm:px-6">
    <p class="text-sm text-gray-600 dark:text-gray-300" aria-live="polite">
      <span class="hidden sm:inline">显示 {{ fromItem }}–{{ toItem }}，共 {{ total }} {{ itemLabel }}</span>
      <span class="sm:hidden">第 {{ page }} / {{ totalPages }} 页</span>
    </p>

    <div class="flex flex-wrap items-center justify-end gap-2">
      <label class="hidden items-center gap-2 text-sm text-gray-600 dark:text-gray-300 sm:flex">
       每页显示
        <select
          :value="pageSize"
          class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100"
          aria-label="每页显示条数"
          @change="changePageSize(($event.target as HTMLSelectElement).value)"
        >
          <option v-for="size in pageSizeOptions" :key="size" :value="size">{{ size }}</option>
        </select>
      </label>

      <nav class="inline-flex items-center -space-x-px rounded-lg shadow-sm" aria-label="分页导航">
        <button
          type="button"
          class="rounded-l-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          :disabled="page <= 1"
          aria-label="上一页"
          @click="emit('update:page', page - 1)"
        >上一页</button>
        <button
          v-for="(number, index) in visiblePages"
          :key="`${number}-${index}`"
          type="button"
          class="min-w-10 border-y border-gray-300 px-3 py-2 text-sm dark:border-dark-600"
          :class="number === page ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700'"
          :disabled="typeof number !== 'number' || number === page"
          :aria-label="typeof number === 'number' ? `第 ${number} 页` : undefined"
          :aria-current="number === page ? 'page' : undefined"
          @click="typeof number === 'number' && emit('update:page', number)"
        >{{ number }}</button>
        <button
          type="button"
          class="rounded-r-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          :disabled="page >= totalPages"
          aria-label="下一页"
          @click="emit('update:page', page + 1)"
        >下一页</button>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

// Adapted from Sub2API's common Pagination component, keeping its page-window
// behavior while removing the host app's i18n, Select, and persistence coupling.
const props = withDefaults(defineProps<{
  total: number
  page: number
  pageSize: number
  itemLabel?: string
  pageSizeOptions?: number[]
}>(), {
  itemLabel: '条记录',
  pageSizeOptions: () => [10, 25, 50, 100, 200],
})

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const fromItem = computed(() => props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1)
const toItem = computed(() => Math.min(props.page * props.pageSize, props.total))

const visiblePages = computed<(number | string)[]>(() => {
  const total = totalPages.value
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1)

  const pages: (number | string)[] = [1]
  const start = Math.max(2, props.page - 2)
  const end = Math.min(total - 1, props.page + 2)
  if (start > 2) pages.push('…')
  for (let number = start; number <= end; number += 1) pages.push(number)
  if (end < total - 1) pages.push('…')
  pages.push(total)
  return pages
})

function changePageSize(raw: string): void {
  const pageSize = Number.parseInt(raw, 10)
  if (props.pageSizeOptions.includes(pageSize) && pageSize !== props.pageSize) {
    emit('update:pageSize', pageSize)
  }
}
</script>
