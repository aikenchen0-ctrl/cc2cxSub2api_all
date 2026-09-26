import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import enCommon from '@/i18n/locales/en/common'
import zhCommon from '@/i18n/locales/zh/common'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppHeader AgentAPI station entry', () => {
  it('shows a localized icon button only to signed-in users', () => {
    expect(zhCommon.nav.openAgentStation).toBe('一键开分站')
    expect(enCommon.nav.openAgentStation).toBe('Open agent station')
    expect(componentSource).toContain('data-testid="open-agent-station"')
    expect(componentSource).toMatch(/<button\s+v-if="user"[\s\S]*?<Icon name="server" size="md" \/>/)
    expect(componentSource).toContain(':aria-label="t(\'nav.openAgentStation\')"')
    expect(componentSource).toContain(':title="t(\'nav.openAgentStation\')"')
  })

  it('uses the shared SSO start endpoint without putting credentials in the URL', () => {
    expect(componentSource).toContain("buildApiUrl('/auth/integrations/agentapi/start')")
    expect(componentSource).toContain('?next=%2Fdashboard')
    expect(componentSource).toContain('openJuSso(startUrl)')
    expect(componentSource).not.toMatch(/agentapi\/start[^\n]*(apiKey|superKey|token)/i)
  })
})
