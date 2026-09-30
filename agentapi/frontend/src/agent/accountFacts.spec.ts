import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  AGENT_ACCOUNT_FACTS_CHANGED,
  notifyAgentAccountFactsChanged,
  onAgentAccountFactsChanged,
} from './accountFacts'

describe('account facts events', () => {
  afterEach(() => vi.restoreAllMocks())

  it('shares only account refresh hints and normalized authoritative cents', () => {
    const listener = vi.fn()
    const stop = onAgentAccountFactsChanged(listener)

    notifyAgentAccountFactsChanged({ balanceCents: 1250.4, refreshSubscriptions: false })

    expect(listener).toHaveBeenCalledWith({
      balanceCents: 1250,
      refreshBalance: true,
      refreshSubscriptions: false,
    })
    stop()
    notifyAgentAccountFactsChanged({ balanceCents: 999 })
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('normalizes untrusted custom-event details before notifying consumers', () => {
    const listener = vi.fn()
    const stop = onAgentAccountFactsChanged(listener)

    window.dispatchEvent(new CustomEvent(AGENT_ACCOUNT_FACTS_CHANGED, {
      detail: { balanceCents: Number.NaN, refreshBalance: false },
    }))

    expect(listener).toHaveBeenCalledWith({
      refreshBalance: false,
      refreshSubscriptions: true,
    })
    stop()
  })
})
