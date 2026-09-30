export interface AgentRoutePresentation {
  title: string
  description: string
}

const routeDescriptions: Record<string, string> = {
  '/dashboard': '查看当前账户的主站余额、用量趋势和代理站可用能力。',
  '/model-plaza': '浏览当前代理站开放的文本、图片与视频模型。',
  '/console': '使用当前代理站 API 能力进行兼容主站习惯的模型调试。',
  '/batch-image': '批量创建并跟踪由主站网关执行和计费的图片任务。',
  '/keys': '创建和管理仅属于当前代理站用户的 API 密钥。',
  '/usage': '查看当前用户在主站形成的请求、令牌和费用记录。',
  '/purchase': '通过主站权威订单和支付通道为当前账户充值。',
  '/orders': '查看当前用户的主站订单、支付状态与入账结果。',
  '/affiliate': '查看当前用户的邀请关系、返利余额和主站划转记录。',
  '/profile': '维护当前用户的资料、安全验证和主站身份绑定。',
  '/payment/qrcode': '使用主站订单生成的二维码继续完成支付。',
  '/payment/stripe': '使用主站订单继续完成 Stripe 支付。',
  '/payment/airwallex': '使用主站订单继续完成 Airwallex 支付。',
  '/payment/result': '核对主站返回的支付状态和最终入账结果。',
  '/auth/oauth/binding/callback': '安全完成当前代理站账户的外部身份绑定。',
  '/admin/dashboard': '查看当前代理站的用户、用量和运营概况。',
  '/admin/agent-provisioning': '检查当前代理站实例的开通状态与安全边界。',
  '/admin/ops': '查看当前代理站的运行指标、异常和待处理事项。',
  '/admin/users': '管理归属于当前代理站的用户映射和访问状态。',
  '/admin/promo-codes': '查看当前代理站优惠码能力和资金协议状态。',
  '/admin/subscriptions': '聚合查看当前代理站用户的主站权威订阅事实。',
  '/admin/affiliates/invites': '查看当前代理站用户之间已确认的邀请关系。',
  '/admin/affiliates/rebates': '查看当前代理站邀请关系产生的主站返利记录。',
  '/admin/affiliates/transfers': '查看当前代理站用户已完成的主站返利划转。',
  '/admin/orders/dashboard': '汇总当前代理站用户的主站支付与订单指标。',
  '/admin/orders': '管理当前代理站范围内的订单镜像和租户备注。',
  '/admin/orders/plans': '维护当前代理站展示的套餐覆盖信息。',
  '/admin/channels': '查看当前代理站可见渠道及租户范围配置。',
  '/admin/channels/pricing': '查看主站权威模型价格和当前代理站展示信息。',
  '/admin/satellite-billing': '查看代理站调用如何复用主站余额和计费账本。',
  '/admin/usage': '聚合当前代理站用户的主站权威用量记录。',
  '/admin/audit-logs': '查看当前代理站管理员操作与安全审计记录。',
  '/admin/announcements': '维护只面向当前代理站用户的公告内容。',
  '/admin/content': '维护当前代理站的自定义页面与法律文档。',
  '/admin/backup': '导出和恢复当前代理站的租户配置与业务记录。',
  '/admin/settings': '维护当前代理站的品牌、文档和租户级设置。',
}

export function resolveAgentRoutePresentation(path: string, fallbackTitle = '控制台'): AgentRoutePresentation {
  if (path.startsWith('/custom/')) {
    return {
      title: fallbackTitle,
      description: '查看当前代理站管理员发布的租户专属内容。',
    }
  }
  return {
    title: fallbackTitle,
    description: routeDescriptions[path] || '使用当前代理站提供的用户控制台能力。',
  }
}

export const agentPanelRouteDescriptions = Object.freeze({ ...routeDescriptions })
