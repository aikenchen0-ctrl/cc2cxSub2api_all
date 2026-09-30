<template>
  <div ref="containerRef" class="relative">
    <button
      :id="id"
      ref="triggerRef"
      type="button"
      :disabled="disabled"
      :aria-expanded="isOpen"
      aria-haspopup="listbox"
      :aria-label="ariaLabel || '选择选项'"
      :aria-describedby="ariaDescribedby"
      :class="[
        'select-trigger',
        isOpen && 'select-trigger-open',
        error && 'select-trigger-error',
        disabled && 'select-trigger-disabled',
      ]"
      @click="toggle"
      @keydown.down.prevent="openFromKeyboard"
      @keydown.up.prevent="openFromKeyboard"
    >
      <span class="select-value">
        <slot name="selected" :option="selectedOption">{{ selectedLabel }}</slot>
      </span>
      <span
        v-if="clearable && hasValue && !disabled"
        class="select-clear"
        role="button"
        tabindex="-1"
        aria-label="清除选择"
        @click.stop="clearSelection"
        @mousedown.stop
      >
        <Icon name="x" size="sm" />
      </span>
      <span class="select-icon">
        <Icon name="chevronDown" size="md" :class="['transition-transform duration-200', isOpen && 'rotate-180']" />
      </span>
    </button>

    <Teleport to="body">
      <Transition name="select-dropdown">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="select-dropdown-portal"
          :class="instanceId"
          :style="dropdownStyle"
          role="listbox"
          :aria-label="ariaLabel || '选择选项'"
          @click.stop
          @mousedown.stop
          @keydown="onDropdownKeyDown"
        >
          <div v-if="isSearchable" class="select-search">
            <Icon name="search" size="sm" class="text-gray-400" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="search"
              :placeholder="searchPlaceholderText"
              :aria-label="searchPlaceholderText"
              class="select-search-input"
              @click.stop
            />
          </div>

          <div ref="optionsListRef" class="select-options">
            <div
              v-for="(option, index) in filteredOptions"
              :key="`${typeof getOptionValue(option)}:${String(getOptionValue(option) ?? '')}`"
              role="option"
              :aria-selected="isSelected(option)"
              :aria-disabled="isOptionDisabled(option)"
              :class="[
                'select-option',
                isSelected(option) && 'select-option-selected',
                isOptionDisabled(option) && 'select-option-disabled',
                focusedIndex === index && !isOptionDisabled(option) && 'select-option-focused',
              ]"
              @click.stop="!isOptionDisabled(option) && selectOption(option)"
              @mouseenter="!isOptionDisabled(option) && (focusedIndex = index)"
            >
              <slot name="option" :option="option" :selected="isSelected(option)">
                <Icon v-if="option._creatable" name="search" size="sm" class="shrink-0 text-gray-400" />
                <span class="select-option-label" :class="option._creatable && 'italic text-gray-500'">{{ getOptionLabel(option) }}</span>
                <Icon v-if="isSelected(option)" name="check" size="sm" class="shrink-0 text-primary-500" :stroke-width="2" />
              </slot>
            </div>
            <div v-if="filteredOptions.length === 0" class="select-empty">
              {{ loading ? t('common.loading') : emptyTextDisplay }}
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from '@/agent/componentI18n'
import Icon from '@/components/icons/Icon.vue'

export interface AgentSelectOption {
  value: string | number | boolean | null
  label: string
  disabled?: boolean
  [key: string]: unknown
}

const props = withDefaults(defineProps<{
  modelValue: string | number | boolean | null | undefined
  options: AgentSelectOption[] | Array<Record<string, unknown>>
  placeholder?: string
  disabled?: boolean
  error?: boolean
  searchable?: boolean | 'auto'
  searchPlaceholder?: string
  emptyText?: string
  valueKey?: string
  labelKey?: string
  creatable?: boolean
  creatablePrefix?: string
  clearable?: boolean
  id?: string
  ariaLabel?: string
  ariaDescribedby?: string
  remote?: boolean
  loading?: boolean
}>(), {
  disabled: false,
  error: false,
  searchable: 'auto',
  valueKey: 'value',
  labelKey: 'label',
  creatable: false,
  creatablePrefix: '',
  clearable: false,
  remote: false,
  loading: false,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: string | number | boolean | null): void
  (event: 'change', value: string | number | boolean | null, option: AgentSelectOption | null): void
  (event: 'search', query: string): void
}>()

const { t } = useI18n()
const instanceId = `agent-select-${Math.random().toString(36).slice(2, 9)}`
const isOpen = ref(false)
const searchQuery = ref('')
const focusedIndex = ref(-1)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const optionsListRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<'bottom' | 'top'>('bottom')
const triggerRect = ref<DOMRect | null>(null)
let remoteSearchTimer: ReturnType<typeof setTimeout> | undefined

const placeholderText = computed(() => props.placeholder || t('common.selectOption'))
const searchPlaceholderText = computed(() => props.searchPlaceholder || t('common.searchPlaceholder'))
const emptyTextDisplay = computed(() => props.emptyText || t('common.noOptionsFound'))
const isSearchable = computed(() => props.remote || props.searchable === true || (props.searchable === 'auto' && props.options.length > 5))
const hasValue = computed(() => props.modelValue !== null && props.modelValue !== undefined && props.modelValue !== '')

function getOptionValue(option: Record<string, unknown>): string | number | boolean | null | undefined {
  return option[props.valueKey] as string | number | boolean | null | undefined
}

function getOptionLabel(option: Record<string, unknown>): string {
  return String(option[props.labelKey] ?? '')
}

function isOptionDisabled(option: Record<string, unknown>): boolean {
  return Boolean(option.disabled)
}

const selectedOption = computed(() => props.options.find(option => getOptionValue(option) === props.modelValue) || null)
const selectedLabel = computed(() => {
  if (selectedOption.value) return getOptionLabel(selectedOption.value)
  if (props.creatable && hasValue.value) return String(props.modelValue)
  return placeholderText.value
})

const filteredOptions = computed(() => {
  let options = props.options as Array<Record<string, unknown>>
  const query = searchQuery.value.trim().toLowerCase()
  if (query && !props.remote) {
    options = options.filter(option => {
      if (getOptionLabel(option).toLowerCase().includes(query)) return true
      return String(option.description || '').toLowerCase().includes(query)
    })
    if (props.creatable) {
      const prefix = props.creatablePrefix || t('common.search')
      options = [{ [props.valueKey]: searchQuery.value.trim(), [props.labelKey]: `${prefix} “${searchQuery.value.trim()}”`, _creatable: true }, ...options]
    }
  }
  return options
})

const dropdownStyle = computed<Record<string, string>>(() => {
  if (!triggerRect.value) return {}
  const viewportRight = Math.max(8, window.innerWidth - 8)
  const left = Math.min(Math.max(8, triggerRect.value.left), viewportRight)
  const availableWidth = Math.max(0, viewportRight - left)
  const minWidth = Math.min(Math.max(200, triggerRect.value.width), availableWidth)
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    minWidth: `${minWidth}px`,
    maxWidth: `${availableWidth}px`,
    zIndex: '100000020',
  }
  if (dropdownPosition.value === 'top') style.bottom = `${window.innerHeight - triggerRect.value.top + 4}px`
  else style.top = `${triggerRect.value.bottom + 4}px`
  return style
})

function isSelected(option: Record<string, unknown>): boolean {
  return getOptionValue(option) === props.modelValue
}

function firstEnabledIndex(start: number, direction: 1 | -1): number {
  const options = filteredOptions.value
  if (!options.length) return -1
  for (let offset = 0; offset < options.length; offset += 1) {
    const index = (start + direction * offset + options.length * 2) % options.length
    if (!isOptionDisabled(options[index])) return index
  }
  return -1
}

function updateTriggerRect(): void {
  if (containerRef.value) triggerRect.value = containerRef.value.getBoundingClientRect()
}

function calculateDropdownPosition(): void {
  updateTriggerRect()
  nextTick(() => {
    if (!dropdownRef.value || !triggerRect.value) return
    const height = dropdownRef.value.offsetHeight || 240
    dropdownPosition.value = window.innerHeight - triggerRect.value.bottom < height && triggerRect.value.top > height ? 'top' : 'bottom'
  })
}

function toggle(): void {
  if (!props.disabled) isOpen.value = !isOpen.value
}

function openFromKeyboard(): void {
  if (!props.disabled) isOpen.value = true
}

function selectOption(option: Record<string, unknown>): void {
  const value = getOptionValue(option) ?? null
  emit('update:modelValue', value)
  emit('change', value, option as AgentSelectOption)
  isOpen.value = false
  triggerRef.value?.focus()
}

function clearSelection(): void {
  emit('update:modelValue', null)
  emit('change', null, null)
}

function scrollToFocused(): void {
  nextTick(() => {
    const list = optionsListRef.value
    const item = list?.children[focusedIndex.value] as HTMLElement | undefined
    if (!list || !item) return
    if (item.offsetTop < list.scrollTop) list.scrollTop = item.offsetTop
    else if (item.offsetTop + item.offsetHeight > list.scrollTop + list.offsetHeight) list.scrollTop = item.offsetTop + item.offsetHeight - list.offsetHeight
  })
}

function onDropdownKeyDown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const direction = event.key === 'ArrowDown' ? 1 : -1
    focusedIndex.value = firstEnabledIndex(focusedIndex.value + direction, direction)
    scrollToFocused()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const option = filteredOptions.value[focusedIndex.value]
    if (option && !isOptionDisabled(option)) selectOption(option)
  } else if (event.key === 'Escape') {
    event.preventDefault()
    isOpen.value = false
    triggerRef.value?.focus()
  } else if (event.key === 'Tab') {
    isOpen.value = false
  }
}

function closeFromOutside(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (!containerRef.value?.contains(target) && !target.closest(`.${instanceId}`)) isOpen.value = false
}

watch(isOpen, open => {
  if (open) {
    calculateDropdownPosition()
    const selectedIndex = filteredOptions.value.findIndex(isSelected)
    focusedIndex.value = firstEnabledIndex(selectedIndex >= 0 ? selectedIndex : 0, 1)
    if (isSearchable.value) nextTick(() => searchInputRef.value?.focus())
    window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
    window.addEventListener('resize', calculateDropdownPosition)
  } else {
    searchQuery.value = ''
    focusedIndex.value = -1
    if (remoteSearchTimer) clearTimeout(remoteSearchTimer)
    remoteSearchTimer = undefined
    window.removeEventListener('scroll', updateTriggerRect, { capture: true })
    window.removeEventListener('resize', calculateDropdownPosition)
  }
})

watch(searchQuery, query => {
  if (!props.remote || !isOpen.value) return
  if (remoteSearchTimer) clearTimeout(remoteSearchTimer)
  remoteSearchTimer = setTimeout(() => {
    remoteSearchTimer = undefined
    emit('search', query.trim())
  }, 300)
})

onMounted(() => document.addEventListener('click', closeFromOutside))
onBeforeUnmount(() => {
  document.removeEventListener('click', closeFromOutside)
  window.removeEventListener('scroll', updateTriggerRect, { capture: true })
  window.removeEventListener('resize', calculateDropdownPosition)
  if (remoteSearchTimer) clearTimeout(remoteSearchTimer)
})
</script>

<style scoped>
.select-trigger { @apply flex w-full cursor-pointer items-center justify-between gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-900 transition-all duration-200 hover:border-gray-300 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:hover:border-dark-500; }
.select-trigger-open { @apply border-primary-500 ring-2 ring-primary-500/30; }
.select-trigger-error { @apply border-red-500 focus:border-red-500 focus:ring-red-500/30; }
.select-trigger-disabled { @apply cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900; }
.select-value { @apply min-w-0 flex-1 truncate text-left; }
.select-icon, .select-clear { @apply shrink-0 text-gray-400 dark:text-dark-400; }
.select-clear { @apply flex cursor-pointer items-center justify-center rounded hover:text-gray-600 dark:hover:text-gray-200; }
</style>

<style>
.select-dropdown-portal { @apply w-max min-w-[200px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg shadow-black/10 dark:border-dark-700 dark:bg-dark-800 dark:shadow-black/30; pointer-events: auto !important; }
.select-dropdown-portal .select-search { @apply flex items-center gap-2 border-b border-gray-100 px-3 py-2 dark:border-dark-700; }
.select-dropdown-portal .select-search-input { @apply min-w-0 flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400; }
.select-dropdown-portal .select-options { @apply max-h-80 overflow-y-auto py-1 outline-none; }
.select-dropdown-portal .select-option { @apply flex cursor-pointer items-center justify-between gap-2 px-4 py-2.5 text-sm text-gray-700 transition-colors duration-150 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700; pointer-events: auto !important; }
.select-dropdown-portal .select-option-selected { @apply bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300; }
.select-dropdown-portal .select-option-focused { @apply bg-gray-100 dark:bg-dark-700; }
.select-dropdown-portal .select-option-disabled { @apply cursor-not-allowed opacity-40; }
.select-dropdown-portal .select-option-label { @apply min-w-0 flex-1 truncate text-left; }
.select-dropdown-portal .select-empty { @apply px-4 py-8 text-center text-sm text-gray-500 dark:text-dark-400; }
.select-dropdown-enter-active, .select-dropdown-leave-active { transition: opacity .2s ease, transform .2s ease; }
.select-dropdown-enter-from, .select-dropdown-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
