import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('../client', () => ({ apiClient: { get } }))

import { checkUpdates, getVersion } from '@/api/admin/system'

describe('admin system read-only version API', () => {
  beforeEach(() => get.mockReset())

  it('reads the current version', async () => {
    get.mockResolvedValue({ data: { version: '1.0.0' } })
    await expect(getVersion()).resolves.toEqual({ version: '1.0.0' })
    expect(get).toHaveBeenCalledWith('/admin/system/version')
  })

  it('checks latest version without exposing mutation calls', async () => {
    get.mockResolvedValue({ data: { current_version: '1.0.0', latest_version: '1.1.0', has_update: true } })
    await expect(checkUpdates()).resolves.toMatchObject({ has_update: true })
    expect(get).toHaveBeenCalledWith('/admin/system/check-updates', { params: undefined })
  })
})
