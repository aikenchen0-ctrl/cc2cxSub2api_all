export const AGENT_ACCOUNT_FACTS_CHANGED = 'agentapi:account-facts-changed'

export interface AgentAccountFactsChangedDetail {
  balanceCents?: number
  refreshBalance?: boolean
  refreshSubscriptions?: boolean
}

function safeDetail(detail: AgentAccountFactsChangedDetail): AgentAccountFactsChangedDetail {
  const next: AgentAccountFactsChangedDetail = {
    refreshBalance: detail.refreshBalance !== false,
    refreshSubscriptions: detail.refreshSubscriptions !== false,
  }
  if (Number.isFinite(detail.balanceCents)) next.balanceCents = Math.round(Number(detail.balanceCents))
  return next
}

export function notifyAgentAccountFactsChanged(detail: AgentAccountFactsChangedDetail = {}): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<AgentAccountFactsChangedDetail>(AGENT_ACCOUNT_FACTS_CHANGED, {
    detail: safeDetail(detail),
  }))
}

export function onAgentAccountFactsChanged(
  listener: (detail: AgentAccountFactsChangedDetail) => void,
): () => void {
  if (typeof window === 'undefined') return () => undefined
  const handler = (event: Event) => {
    const detail = event instanceof CustomEvent ? event.detail : {}
    listener(safeDetail(detail && typeof detail === 'object' ? detail : {}))
  }
  window.addEventListener(AGENT_ACCOUNT_FACTS_CHANGED, handler)
  return () => window.removeEventListener(AGENT_ACCOUNT_FACTS_CHANGED, handler)
}
