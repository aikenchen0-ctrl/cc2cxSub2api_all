export interface AgentRoutePresentation {
  title: string
  description: string
}

const routeDescriptions: Record<string, string> = {
  '/dashboard': '查看当前账户的余额、用量趋势和本站可用能力。',
  '/keys': '创建和管理仅属于当前用户的 API 密钥。',
  '/usage': '查看当前用户的请求、令牌和费用记录。',
  '/purchase': '通过本站支付通道为当前账户充值。',
  '/orders': '查看当前用户的订单、支付状态与入账结果。',
  '/payment/qrcode': '使用订单二维码继续完成支付。',
  '/payment/stripe': '继续完成 Stripe 支付。',
  '/payment/airwallex': '继续完成 Airwallex 支付。',
  '/payment/result': '核对支付状态和最终入账结果。',
  '/auth/oauth/binding/callback': '安全完成本站账户的外部身份绑定。',
  '/admin/dashboard': '查看本站的用户、用量和运营概况。',
  '/admin/users': '查看通过本站注册并归属到当前站点的用户。',
  '/admin/usage': '聚合当前站点用户的用量和结算记录。',
  '/admin/announcements': '维护只面向本站用户的公告内容。',
  '/admin/settings': '维护本站的品牌、文档和站点设置。',
}

export function resolveAgentRoutePresentation(path: string, fallbackTitle = '控制台'): AgentRoutePresentation {
  return {
    title: fallbackTitle,
    description: routeDescriptions[path] || '使用本站提供的用户控制台能力。',
  }
}

export const agentPanelRouteDescriptions = Object.freeze({ ...routeDescriptions })
