import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppHeader AgentAPI station entry', () => {
  it('keeps the station entry exclusively in the sidebar', () => {
    expect(componentSource).not.toContain('data-testid="open-agent-station"')
    expect(componentSource).not.toContain('nav.openAgentStation')
    expect(componentSource).not.toContain('/auth/integrations/agentapi/start')
    expect(componentSource).not.toContain('handleOpenAgentStation')
  })
})
