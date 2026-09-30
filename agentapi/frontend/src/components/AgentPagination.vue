<template>
  <div
    class="border-t border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-900 sm:px-6"
    data-testid="agent-pagination"
  >
    <div class="flex items-center justify-between sm:hidden">
      <button
        type="button"
        class="relative inline-flex items-center rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
        :disabled="page <= 1"
        aria-label="上一页"
        @click="goToPage(page - 1)"
      >
        上一页
      </button>
      <span class="text-sm text-gray-700 dark:text-gray-300" aria-live="polite">
        第 {{ page }} / {{ totalPages }} 页
      </span>
      <button
        type="button"
        class="relative ml-3 inline-flex items-center rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
        :disabled="page >= totalPages"
        aria-label="下一页"
        @click="goToPage(page + 1)"
      >
        下一页
      </button>
    </div>

    <div class="hidden items-center justify-between gap-4 sm:flex">
      <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
        <p class="text-sm text-gray-700 dark:text-gray-300" aria-live="polite">
          显示
          <span class="font-medium">{{ fromItem }}</span>
          至
          <span class="font-medium">{{ toItem }}</span>
          项，共
          <span class="font-medium">{{ total }}</span>
          {{ itemLabel }}
        </p>

        <label
          v-if="showPageSizeSelector"
          class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
        >
          每页
          <AgentSelect
            :model-value="pageSize"
            :options="pageSizeSelectOptions"
            class="w-24"
            aria-label="每页显示条数"
            @change="changePageSize"
          />
          条
        </label>

        <form
          v-if="showJump"
          class="flex items-center gap-2"
          aria-label="跳转页码"
          @submit.prevent="submitJump"
        >
          <label :for="jumpInputId" class="text-sm text-gray-700 dark:text-gray-300">跳至</label>
          <input
            :id="jumpInputId"
            v-model="jumpPage"
            type="number"
            inputmode="numeric"
            min="1"
            :max="totalPages"
            class="w-20 rounded-md border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-700 outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100"
            placeholder="页码"
          />
          <button
            type="submit"
            class="rounded-md px-2.5 py-1.5 text-sm font-medium text-primary-600 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-300 dark:hover:bg-primary-900/20"
            :disabled="totalPages <= 1"
          >
            跳转
          </button>
        </form>
      </div>

      <nav
        class="relative z-0 inline-flex shrink-0 -space-x-px rounded-md shadow-sm"
        aria-label="分页导航"
      >
        <button
          type="button"
          class="relative inline-flex items-center rounded-l-md border border-gray-300 bg-white px-2 py-2 text-sm font-medium text-gray-500 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:bg-dark-700"
          :disabled="page <= 1"
          aria-label="上一页"
          @click="goToPage(page - 1)"
        >
          <Icon name="chevronLeft" size="md" aria-hidden="true" />
        </button>

        <button
          v-for="(pageNumber, index) in visiblePages"
          :key="`${pageNumber}-${index}`"
          type="button"
          class="relative inline-flex min-w-10 items-center justify-center border px-3 py-2 text-sm font-medium"
          :class="[
            pageNumber === page
              ? 'z-10 border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700',
            typeof pageNumber !== 'number' && 'cursor-default',
          ]"
          :disabled="typeof pageNumber !== 'number' || pageNumber === page"
          :aria-label="typeof pageNumber === 'number' ? `前往第 ${pageNumber} 页` : undefined"
          :aria-current="pageNumber === page ? 'page' : undefined"
          @click="typeof pageNumber === 'number' && goToPage(pageNumber)"
        >
          {{ pageNumber }}
        </button>

        <button
          type="button"
          class="relative inline-flex items-center rounded-r-md border border-gray-300 bg-white px-2 py-2 text-sm font-medium text-gray-500 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:bg-dark-700"
          :disabled="page >= totalPages"
          aria-label="下一页"
          @click="goToPage(page + 1)"
        >
          <Icon name="chevronRight" size="md" aria-hidden="true" />
        </button>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import AgentSelect from '@/components/common/AgentSelect.vue'
import Icon from '@/components/icons/Icon.vue'

// Adapted from Sub2API's common Pagination component. AgentAPI keeps Chinese
// labels locally until the complete proxy UI has a real i18n surface.
const props = withDefaults(defineProps<{
  total: number
  page: number
  pageSize: number
  itemLabel?: string
  pageSizeOptions?: number[]
  showPageSizeSelector?: boolean
  showJump?: boolean
}>(), {
  itemLabel: '条记录',
  pageSizeOptions: () => [10, 20, 25, 50, 100, 200],
  showPageSizeSelector: true,
  showJump: false,
})

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()

const jumpPage = ref('')
const jumpInputId = `agent-pagination-jump-${useId()}`

const safePageSize = computed(() => Number.isInteger(props.pageSize) && props.pageSize > 0 ? props.pageSize : 1)
const safeTotal = computed(() => Number.isFinite(props.total) && props.total > 0 ? Math.floor(props.total) : 0)
const totalPages = computed(() => Math.max(1, Math.ceil(safeTotal.value / safePageSize.value)))
const currentPage = computed(() => Math.min(Math.max(Math.trunc(props.page) || 1, 1), totalPages.value))
const fromItem = computed(() => safeTotal.value === 0 ? 0 : (currentPage.value - 1) * safePageSize.value + 1)
const toItem = computed(() => Math.min(currentPage.value * safePageSize.value, safeTotal.value))

const normalizedPageSizeOptions = computed(() => Array.from(new Set([
  ...props.pageSizeOptions.filter((size) => Number.isInteger(size) && size > 0),
  safePageSize.value,
])).sort((left, right) => left - right))
const pageSizeSelectOptions = computed(() => normalizedPageSizeOptions.value.map(size => ({ value: size, label: String(size) })))

const visiblePages = computed<(number | string)[]>(() => {
  const total = totalPages.value
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1)

  const pages: (number | string)[] = [1]
  const start = Math.max(2, currentPage.value - 2)
  const end = Math.min(total - 1, currentPage.value + 2)
  if (start > 2) pages.push('…')
  for (let pageNumber = start; pageNumber <= end; pageNumber += 1) pages.push(pageNumber)
  if (end < total - 1) pages.push('…')
  pages.push(total)
  return pages
})

function goToPage(nextPage: number): void {
  if (!Number.isInteger(nextPage)) return
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === currentPage.value) return
  emit('update:page', nextPage)
}

function changePageSize(raw: string | number | boolean | null): void {
  const nextPageSize = Number.parseInt(String(raw ?? ''), 10)
  if (!normalizedPageSizeOptions.value.includes(nextPageSize) || nextPageSize === safePageSize.value) return
  emit('update:pageSize', nextPageSize)
}

function submitJump(): void {
  const value = String(jumpPage.value).trim()
  if (!value) return
  const requestedPage = Number.parseInt(value, 10)
  if (Number.isNaN(requestedPage)) return
  jumpPage.value = ''
  goToPage(Math.min(Math.max(requestedPage, 1), totalPages.value))
}

watch(() => [props.total, props.pageSize], () => {
  if (props.page > totalPages.value) emit('update:page', totalPages.value)
})
</script>
