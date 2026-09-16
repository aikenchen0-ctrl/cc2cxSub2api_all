import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const componentSource = readFileSync(
  resolve(process.cwd(), 'src/components/common/VersionBadge.vue'),
  'utf8'
)
const systemAPISource = readFileSync(
  resolve(process.cwd(), 'src/api/admin/system.ts'),
  'utf8'
)

describe('VersionBadge read-only update notice', () => {
  it('keeps version checking and the update notice', () => {
    expect(componentSource).toContain('appStore.fetchVersion')
    expect(componentSource).toContain("t('version.updateAvailable')")
    expect(componentSource).toContain('latestVersion')
  })

  it('does not expose update or rollback actions', () => {
    expect(componentSource).not.toContain('handleUpdate')
    expect(componentSource).not.toContain('handleRollback')
    expect(componentSource).not.toContain('toggleRollbackPanel')
    expect(systemAPISource).not.toContain("'/admin/system/update'")
    expect(systemAPISource).not.toContain("'/admin/system/rollback'")
    expect(systemAPISource).not.toContain("'/admin/system/rollback-versions'")
  })
})
