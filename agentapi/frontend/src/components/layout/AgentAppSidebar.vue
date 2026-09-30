<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { agentAPI, type AgentContentPage } from '@/agent/api'
import { siteLogo, siteName } from '@/agent/branding'
import { useAgentSession } from '@/agent/session'
import { isDark, toggleTheme } from '@/agent/theme'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ collapsed: boolean; mobileOpen: boolean }>()
// Desktop collapse preference must not hide labels in the mobile drawer.
const isCollapsed = computed(() => props.collapsed && !props.mobileOpen)
const emit = defineEmits<{ 'update:collapsed': [value: boolean]; 'close-mobile': [] }>()

const route = useRoute()
const session = useAgentSession()
const customPages = ref<AgentContentPage[]>([])
const groupExpandOverrides = ref<Record<string, boolean>>({})

interface SidebarItem {
  path: string
  label: string
  icon: NonNullable<InstanceType<typeof Icon>['$props']['name']>
  children?: SidebarItem[]
}

interface SidebarSection {
  label: string
  items: SidebarItem[]
}

const quickApps: SidebarItem[] = [
  { path: '/model-plaza', label: '模型广场', icon: 'grid' },
  { path: '/console', label: '模型工作台', icon: 'chat' },
  { path: '/batch-image', label: '批量图片', icon: 'sparkles' },
]

const personal: SidebarItem[] = [
  { path: '/dashboard', label: '仪表盘', icon: 'chart' },
  { path: '/keys', label: 'API 密钥', icon: 'key' },
  { path: '/usage', label: '使用记录', icon: 'clock' },
  { path: '/purchase', label: '充值', icon: 'creditCard' },
  { path: '/orders', label: '订单记录', icon: 'dollar' },
  { path: '/affiliate', label: '推广返利', icon: 'users' },
  { path: '/profile', label: '个人资料', icon: 'user' },
]

const management: SidebarItem[] = [
  { path: '/admin/dashboard', label: '仪表盘', icon: 'chart' },
  { path: '/admin/agent-provisioning', label: '代理站开通', icon: 'server' },
  { path: '/admin/ops', label: '运营监控', icon: 'chartBar' },
  { path: '/admin/users', label: '用户管理', icon: 'users' },
  { path: '/admin/promo-codes', label: '优惠码管理', icon: 'badge' },
  { path: '/admin/subscriptions', label: '订阅管理', icon: 'creditCard' },
  {
    path: '/admin/affiliates', label: '推广管理', icon: 'users',
    children: [
      { path: '/admin/affiliates/invites', label: '邀请记录', icon: 'users' },
      { path: '/admin/affiliates/rebates', label: '返利记录', icon: 'gift' },
      { path: '/admin/affiliates/transfers', label: '划转记录', icon: 'dollar' },
    ],
  },
  {
    path: '/admin/orders', label: '支付管理', icon: 'dollar',
    children: [
      { path: '/admin/orders/dashboard', label: '支付统计', icon: 'chartBar' },
      { path: '/admin/orders', label: '订单管理', icon: 'dollar' },
      { path: '/admin/orders/plans', label: '套餐管理', icon: 'creditCard' },
    ],
  },
  {
    path: '/admin/channels', label: '渠道管理', icon: 'server',
    children: [
      { path: '/admin/channels', label: '渠道列表', icon: 'server' },
      { path: '/admin/channels/pricing', label: '模型定价', icon: 'dollar' },
    ],
  },
  { path: '/admin/satellite-billing', label: '代理站计费', icon: 'dollar' },
  { path: '/admin/usage', label: '用量同步', icon: 'clock' },
  { path: '/admin/audit-logs', label: '操作日志', icon: 'shield' },
  { path: '/admin/announcements', label: '公告管理', icon: 'bell' },
  { path: '/admin/content', label: '内容页面', icon: 'document' },
  { path: '/admin/backup', label: '备份管理', icon: 'database' },
  { path: '/admin/settings', label: '系统设置', icon: 'cog' },
]

const sections = computed<SidebarSection[]>(() => [
  ...(session.isAgentAdmin ? [{ label: '站点管理', items: management }] : []),
  ...(customPages.value.length ? [{ label: '站点内容', items: customPages.value.map<SidebarItem>(item => ({ path: `/custom/${item.slug}`, label: item.title, icon: 'document' })) }] : []),
  { label: '我的账户', items: personal },
])

function isActive(path: string): boolean {
  if (path === '/admin/orders') return route.path === path
  if (path === '/admin/channels') return route.path === path
  return route.path === path || (path.startsWith('/admin/') && route.path.startsWith(`${path}/`))
}

function isGroupActive(item: SidebarItem): boolean {
  return item.children?.some(child => isActive(child.path)) === true
}

function isGroupExpanded(item: SidebarItem): boolean {
  const override = groupExpandOverrides.value[item.path]
  return override === undefined ? isGroupActive(item) : override
}

function toggleGroup(item: SidebarItem): void {
  if (isCollapsed.value) return
  groupExpandOverrides.value = {
    ...groupExpandOverrides.value,
    [item.path]: !isGroupExpanded(item),
  }
}

function tourName(path: string): string | undefined {
  const names: Record<string, string> = {
    '/model-plaza': 'sidebar-model-plaza',
    '/dashboard': 'sidebar-dashboard',
    '/keys': 'sidebar-api-keys',
    '/usage': 'sidebar-usage',
    '/profile': 'sidebar-profile',
    '/admin/dashboard': 'sidebar-admin-dashboard',
  }
  return names[path]
}

onMounted(async () => {
  try { customPages.value = (await agentAPI.contentPages.list()).items }
  catch { customPages.value = [] }
})
</script>

<template>
  <aside id="agent-sidebar" class="sidebar" :class="[isCollapsed ? 'w-[72px]' : 'w-64', { '-translate-x-full lg:translate-x-0': !mobileOpen }]" aria-label="侧边菜单">
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': isCollapsed }">
      <router-link to="/home" class="sidebar-logo flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-xl shadow-glow transition-opacity hover:opacity-80" :aria-label="siteName" @click="emit('close-mobile')"><img :src="siteLogo" alt="" class="h-full w-full object-contain"></router-link>
      <div class="sidebar-brand" :class="{ 'sidebar-brand-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined"><router-link to="/home" class="block truncate text-lg font-bold text-gray-900 transition-colors hover:text-primary-600 dark:text-white dark:hover:text-primary-400" :tabindex="isCollapsed ? -1 : undefined" @click="emit('close-mobile')">{{ siteName }}</router-link><p class="text-[10px] font-medium uppercase tracking-[0.16em] text-gray-400">Agent Console</p></div>
    </div>

    <nav class="sidebar-nav scrollbar-hide">
      <div class="sidebar-section">
        <div class="sidebar-section-title" :class="{ 'sidebar-section-title-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined"><span class="sidebar-section-title-text" :class="{ 'sidebar-section-title-text-collapsed': isCollapsed }">快捷应用</span></div>
        <router-link v-for="item in quickApps" :key="item.path" :to="item.path" class="sidebar-link mb-1" :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': isCollapsed }" :title="isCollapsed ? item.label : undefined" :aria-label="item.label" :aria-current="isActive(item.path) ? 'page' : undefined" :data-tour="tourName(item.path)" @click="emit('close-mobile')"><Icon :name="item.icon" class="shrink-0" /><span class="sidebar-label" :class="{ 'sidebar-label-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined">{{ item.label }}</span></router-link>
      </div>
      <div v-for="section in sections" :key="section.label" class="sidebar-section">
        <div class="sidebar-section-title" :class="{ 'sidebar-section-title-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined"><span class="sidebar-section-title-text" :class="{ 'sidebar-section-title-text-collapsed': isCollapsed }">{{ section.label }}</span></div>
        <template v-for="item in section.items" :key="item.path">
          <div v-if="item.children?.length" class="mb-1">
            <button
              type="button"
              class="sidebar-link w-full"
              :class="{ 'sidebar-link-active': isGroupActive(item) && (isCollapsed || !isGroupExpanded(item)), 'sidebar-link-collapsed': isCollapsed }"
              :title="isCollapsed ? item.label : undefined"
              :aria-label="item.label"
              :aria-expanded="!isCollapsed && isGroupExpanded(item)"
              :data-testid="`sidebar-group-${item.path.split('/').filter(Boolean).join('-')}`"
              @click="toggleGroup(item)"
            >
              <Icon :name="item.icon" class="shrink-0" />
              <span class="sidebar-label sidebar-label-flex flex-1" :class="{ 'sidebar-label-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined"><span class="min-w-0 truncate text-left">{{ item.label }}</span><Icon name="chevronDown" size="sm" class="shrink-0 transition-transform duration-200" :class="{ 'rotate-180': isGroupExpanded(item) }" /></span>
            </button>
            <div v-if="!isCollapsed && isGroupExpanded(item)" class="mb-1 ml-4 border-l border-gray-200 pl-2 dark:border-dark-600">
              <router-link
                v-for="child in item.children"
                :key="child.path"
                :to="child.path"
                class="sidebar-link mb-0.5 py-1.5 text-sm"
                :class="{ 'sidebar-link-active': isActive(child.path) }"
                :aria-current="isActive(child.path) ? 'page' : undefined"
                @click="emit('close-mobile')"
              >
                <Icon :name="child.icon" size="sm" class="shrink-0" />
                <span class="truncate">{{ child.label }}</span>
              </router-link>
            </div>
          </div>
          <router-link v-else :to="item.path" class="sidebar-link mb-1" :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': isCollapsed }" :title="isCollapsed ? item.label : undefined" :aria-label="item.label" :aria-current="isActive(item.path) ? 'page' : undefined" :data-tour="tourName(item.path)" @click="emit('close-mobile')"><Icon :name="item.icon" class="shrink-0" /><span class="sidebar-label" :class="{ 'sidebar-label-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined">{{ item.label }}</span></router-link>
        </template>
      </div>
    </nav>

    <div class="mt-auto border-t border-gray-100 p-3 dark:border-dark-800">
      <button type="button" class="sidebar-link mb-2 w-full" :class="{ 'sidebar-link-collapsed': isCollapsed }" :title="isCollapsed ? (isDark ? '浅色模式' : '深色模式') : undefined" aria-label="切换主题" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" class="shrink-0" /><span class="sidebar-label" :class="{ 'sidebar-label-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined">{{ isDark ? '浅色模式' : '深色模式' }}</span></button>
      <button type="button" class="sidebar-link hidden w-full lg:flex" :class="{ 'sidebar-link-collapsed': isCollapsed }" :title="isCollapsed ? '展开侧栏' : '收起侧栏'" :aria-label="isCollapsed ? '展开侧栏' : '收起侧栏'" :aria-expanded="!isCollapsed" @click="emit('update:collapsed', !collapsed)"><Icon :name="isCollapsed ? 'chevronRight' : 'chevronLeft'" class="shrink-0" /><span class="sidebar-label" :class="{ 'sidebar-label-collapsed': isCollapsed }" :aria-hidden="isCollapsed ? 'true' : undefined">收起侧栏</span></button>
    </div>
  </aside>
  <button v-if="mobileOpen" type="button" class="fixed inset-0 z-30 bg-black/50 lg:hidden" aria-label="关闭菜单遮罩" @click="emit('close-mobile')"></button>
</template>

<style scoped>
/* Copied from Sub2API AppSidebar: keep labels mounted during collapse. */
.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  white-space: nowrap;
  transition:
    max-width 0.22s ease,
    opacity 0.14s ease,
    transform 0.14s ease;
  max-width: 12rem;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(229 231 235);
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity 0.18s ease;
}

.dark .sidebar-section-title::after {
  background: rgb(55 65 81);
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
  transition-delay: 0.08s;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width 0.2s ease,
    opacity 0.12s ease,
    transform 0.12s ease;
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}
</style>
