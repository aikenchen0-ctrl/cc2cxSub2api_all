import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAgentSession } from '@/agent/session'
import { applyPageTitle } from '@/agent/branding'
import { useAgentNavigationLoadingState } from '@/composables/useAgentNavigationLoading'

const routes: RouteRecordRaw[] = [
  { path: '/admin', redirect: '/admin/dashboard' },
  {
    path: '/admin/dashboard', name: 'AgentAdmin-dashboard',
    component: () => import('@/views/admin/dashboard/AgentAdminDashboardView.vue'),
    meta: { requiresAuth: true, requiresAgentAdmin: true, title: '仪表盘' },
  },
  {
    path: '/admin/users', name: 'AgentAdmin-users',
    component: () => import('@/views/admin/users/AgentAdminUsersView.vue'),
    meta: { requiresAuth: true, requiresAgentAdmin: true, title: '用户管理' },
  },
  {
    path: '/admin/usage', name: 'AgentAdmin-usage',
    component: () => import('@/views/admin/usage/AgentAdminUsageView.vue'),
    meta: { requiresAuth: true, requiresAgentAdmin: true, title: '用量同步' },
  },
  {
    path: '/admin/settings', name: 'AgentAdmin-settings',
    component: () => import('@/views/admin/settings/AgentAdminSettingsView.vue'),
    meta: { requiresAuth: true, requiresAgentAdmin: true, title: '系统设置' },
  },
  {
    path: '/admin/announcements', name: 'AgentAdmin-announcements',
    component: () => import('@/views/admin/announcements/AgentAnnouncementsView.vue'),
    meta: { requiresAuth: true, requiresAgentAdmin: true, title: '公告管理' },
  },
  { path: '/', redirect: '/home' },
  { path: '/setup', redirect: '/admin/dashboard' },
  { path: '/home', name: 'Home', component: () => import('@/views/AgentHomeView.vue'), meta: { requiresAuth: false, title: '首页' } },
  { path: '/downloads', name: 'Downloads', component: () => import('@/views/AgentDownloadsView.vue'), meta: { requiresAuth: false, title: '客户端下载' } },
  { path: '/key-usage', name: 'KeyUsage', component: () => import('@/views/AgentKeyUsageView.vue'), meta: { requiresAuth: false, title: 'Key 用量查询' } },
  { path: '/login', name: 'Login', component: () => import('@/views/AgentLoginView.vue'), meta: { requiresAuth: false, authLayout: true, title: '登录' } },
  { path: '/register', name: 'Register', component: () => import('@/views/AgentRegisterView.vue'), meta: { requiresAuth: false, authLayout: true, title: '注册账号' } },
  { path: '/email-verify', name: 'EmailVerify', component: () => import('@/views/AgentEmailVerifyView.vue'), meta: { requiresAuth: false, authLayout: true, title: '验证邮箱' } },
  { path: '/forgot-password', name: 'ForgotPassword', component: () => import('@/views/AgentForgotPasswordView.vue'), meta: { requiresAuth: false, authLayout: true, title: '找回密码' } },
  { path: '/reset-password', name: 'ResetPassword', component: () => import('@/views/AgentResetPasswordView.vue'), meta: { requiresAuth: false, authLayout: true, title: '重置密码' } },
  ...[
    ['/auth/callback', 'MainOAuthCallback', 'GitHub / Google'],
    ['/auth/linuxdo/callback', 'LinuxDoOAuthCallback', 'LinuxDo'],
    ['/auth/wechat/callback', 'WeChatOAuthCallback', '微信'],
    ['/auth/dingtalk/callback', 'DingTalkOAuthCallback', '钉钉'],
    ['/auth/dingtalk/email-completion', 'DingTalkEmailCompletion', '钉钉'],
    ['/auth/oidc/callback', 'OIDCOAuthCallback', 'OIDC'],
  ].map(([path, name, provider]) => ({
    path,
    name,
    component: () => import('@/views/auth/AgentExternalAuthCallbackView.vue'),
    props: { provider },
    meta: { requiresAuth: false, authLayout: true, title: `${provider} 登录` },
  })),
  { path: '/auth/oauth/callback', redirect: to => ({ path: '/auth/callback', query: to.query, hash: to.hash }) },
  { path: '/dashboard', name: 'Dashboard', component: () => import('@/views/AgentDashboardView.vue'), meta: { requiresAuth: true, title: '总览' } },
  { path: '/purchase', name: 'Purchase', component: () => import('@/views/AgentRechargeView.vue'), meta: { requiresAuth: true, title: '充值' } },
  { path: '/payment/qrcode', name: 'PaymentQRCode', component: () => import('@/views/AgentPaymentQRCodeView.vue'), meta: { requiresAuth: true, title: '扫码支付' } },
  { path: '/payment/stripe', name: 'StripePayment', component: () => import('@/views/AgentStripePaymentView.vue'), meta: { requiresAuth: true, title: 'Stripe 支付' } },
  { path: '/payment/stripe-popup', redirect: to => ({ path: '/payment/stripe', query: to.query }) },
  { path: '/payment/airwallex', name: 'AirwallexPayment', component: () => import('@/views/AgentAirwallexPaymentView.vue'), meta: { requiresAuth: true, title: 'Airwallex 支付' } },
  { path: '/payment/result', name: 'PaymentResult', component: () => import('@/views/AgentPaymentResultView.vue'), meta: { requiresAuth: true, title: '支付结果' } },
  { path: '/auth/wechat/payment/callback', name: 'WechatPaymentCallback', component: () => import('@/views/auth/AgentWechatPaymentCallbackView.vue'), meta: { requiresAuth: false, title: '恢复微信支付' } },
  { path: '/auth/oauth/binding/callback', name: 'OAuthBindingCallback', component: () => import('@/views/auth/AgentOAuthBindingCallbackView.vue'), meta: { requiresAuth: true, title: '账号绑定' } },
  { path: '/recharge', redirect: '/purchase' },
  { path: '/orders', name: 'Orders', component: () => import('@/views/AgentOrdersView.vue'), meta: { requiresAuth: true, title: '订单记录' } },
  { path: '/agent-admin', redirect: '/admin/dashboard' },
  { path: '/keys', name: 'Keys', component: () => import('@/views/user/AgentAPIKeysView.vue'), meta: { requiresAuth: true, title: 'API 密钥' } },
  { path: '/usage', name: 'Usage', component: () => import('@/views/AgentUsageView.vue'), meta: { requiresAuth: true, title: '用量记录' } },
  { path: '/:pathMatch(.*)*', name: 'NotFound', component: () => import('@/views/NotFoundView.vue'), meta: { title: '页面不存在' } },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

const navigationLoading = useAgentNavigationLoadingState()

router.beforeEach(async (to) => {
  navigationLoading.startNavigation()
  const auth = useAgentSession()
  if (!auth.initialized) {
    await auth.checkAuth()
  }
  applyPageTitle(String(to.meta.title || ''))
  if (to.meta.requiresAuth !== false && !auth.isAuthenticated) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresAgentAdmin === true && !auth.isAgentAdmin) {
    return '/dashboard'
  }
  if ((to.path === '/login' || to.path === '/register' || to.path === '/email-verify' || to.path === '/forgot-password' || to.path === '/reset-password') && auth.isAuthenticated) {
    return '/dashboard'
  }
  return true
})

router.afterEach(() => {
  navigationLoading.endNavigation()
})

router.onError(() => {
  navigationLoading.endNavigation()
})

export default router
