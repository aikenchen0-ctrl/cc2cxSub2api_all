import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAgentSession } from '@/agent/session'

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/home' },
  { path: '/home', name: 'Home', component: () => import('@/views/AgentHomeView.vue'), meta: { requiresAuth: false, title: '首页' } },
  { path: '/login', name: 'Login', component: () => import('@/views/AgentLoginView.vue'), meta: { requiresAuth: false, title: '登录' } },
  { path: '/register', name: 'Register', component: () => import('@/views/AgentRegisterView.vue'), meta: { requiresAuth: false, title: '注册账号' } },
  { path: '/dashboard', name: 'Dashboard', component: () => import('@/views/AgentDashboardView.vue'), meta: { requiresAuth: true, title: '总览' } },
  { path: '/recharge', name: 'Recharge', component: () => import('@/views/AgentRechargeView.vue'), meta: { requiresAuth: true, title: '充值' } },
  { path: '/console', name: 'Console', component: () => import('@/views/AgentModelConsoleView.vue'), meta: { requiresAuth: true, title: '模型工作台' } },
  { path: '/agent-admin', name: 'AgentAdmin', component: () => import('@/views/AgentConsoleView.vue'), meta: { requiresAuth: true, requiresAgentAdmin: true, title: '代理站管理' } },
  { path: '/keys', name: 'Keys', component: () => import('@/views/user/AgentAPIKeysView.vue'), meta: { requiresAuth: true, title: 'API 密钥' } },
  { path: '/usage', name: 'Usage', component: () => import('@/views/AgentUsageView.vue'), meta: { requiresAuth: true, title: '用量记录' } },
  { path: '/profile', name: 'Profile', component: () => import('@/views/AgentProfileView.vue'), meta: { requiresAuth: true, title: '个人资料' } },
  { path: '/:pathMatch(.*)*', name: 'NotFound', component: () => import('@/views/NotFoundView.vue'), meta: { title: '页面不存在' } },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  const auth = useAgentSession()
  if (!auth.initialized) {
    await auth.checkAuth()
  }
  document.title = `${String(to.meta.title || 'AgentAPI')} · AgentAPI`
  if (to.meta.requiresAuth !== false && !auth.isAuthenticated) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresAgentAdmin === true && !auth.isAgentAdmin) {
    return '/dashboard'
  }
  if ((to.path === '/login' || to.path === '/register') && auth.isAuthenticated) {
    return '/dashboard'
  }
  return true
})

export default router
