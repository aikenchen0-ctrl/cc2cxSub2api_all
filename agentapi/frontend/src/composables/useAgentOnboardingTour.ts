import { nextTick } from 'vue'
import { driver, type DriveStep, type Driver } from 'driver.js'
import 'driver.js/dist/driver.css'
import '@/styles/agent-onboarding.css'
import {
  clearAgentOnboardingComplete,
  isAgentOnboardingComplete,
  markAgentOnboardingComplete,
  type AgentOnboardingRole,
} from '@/agent/onboarding'

interface AgentOnboardingTour {
  startTour: (force?: boolean) => Promise<boolean>
  replayTour: () => void
  disposeTour: () => void
}

export function agentOnboardingSteps(role: AgentOnboardingRole): DriveStep[] {
  const steps: DriveStep[] = [
    {
      popover: {
        title: '欢迎使用站点控制台',
        description: '这里集中展示账户余额、用量和本站可用能力。',
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '[data-tour="sidebar-dashboard"]',
      popover: {
        title: '仪表盘',
        description: '查看当前账户的余额、用量与可用能力。',
        side: 'right',
        align: 'start',
      },
    },
    {
      element: '[data-tour="sidebar-api-keys"]',
      popover: {
        title: 'API 密钥',
        description: '创建 API Key，并使用本站提供的调用地址接入模型服务。',
        side: 'right',
        align: 'start',
      },
    },
    {
      element: '[data-tour="sidebar-usage"]',
      popover: {
        title: '使用记录',
        description: '查看当前用户的请求、令牌和费用记录，不会读取其他站点的用户数据。',
        side: 'right',
        align: 'start',
      },
    },
  ]

  if (role === 'admin') {
    steps.push({
      element: '[data-tour="sidebar-admin-dashboard"]',
      popover: {
        title: '站点管理',
        description: '管理本站用户、品牌展示和站点配置。',
        side: 'right',
        align: 'start',
      },
    })
  }

  steps.push({
    element: '[data-tour="header-user-menu"]',
    popover: {
      title: '随时重新查看',
      description: '以后可以从用户菜单选择“重新查看新手引导”。',
      side: 'bottom',
      align: 'end',
    },
  })

  return steps
}

function existingSteps(role: AgentOnboardingRole): DriveStep[] {
  return agentOnboardingSteps(role).filter((step) => {
    if (typeof step.element !== 'string') return true
    return document.querySelector(step.element) !== null
  })
}

export function useAgentOnboardingTour(role: AgentOnboardingRole): AgentOnboardingTour {
  let instance: Driver | null = null
  let suppressCompletion = false

  async function startTour(force = false): Promise<boolean> {
    if (typeof document === 'undefined') return false
    if (instance?.isActive()) return false
    if (!force && isAgentOnboardingComplete(role)) return false

    await nextTick()
    const steps = existingSteps(role)
    if (steps.length < 2) return false

    suppressCompletion = false
    instance = driver({
      steps,
      animate: true,
      smoothScroll: true,
      allowClose: true,
      allowScroll: true,
      overlayClickBehavior: 'close',
      stagePadding: 6,
      stageRadius: 10,
      showProgress: true,
      progressText: '{{current}} / {{total}}',
      nextBtnText: '下一步',
      prevBtnText: '上一步',
      doneBtnText: '完成',
      popoverClass: 'agentapi-onboarding-popover',
      onDestroyed: () => {
        if (!suppressCompletion) markAgentOnboardingComplete(role)
        instance = null
      },
    })
    instance.drive()
    return true
  }

  function replayTour(): void {
    if (instance?.isActive()) return
    clearAgentOnboardingComplete(role)
    void startTour(true)
  }

  function disposeTour(): void {
    if (!instance) return
    suppressCompletion = true
    instance.destroy()
    instance = null
  }

  return { startTour, replayTour, disposeTour }
}
